package server

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/sourcegraph/sourcegraph/internal/api"
	"github.com/sourcegraph/sourcegraph/internal/gitserver/protocol"
)

// VCSSyncer describes whether and how to sync content from a VCS remote to
// local disk.
type VCSSyncer interface {
	// Type returns the type of the syncer.
	Type() string
	// IsCloneable checks to see if the VCS remote URL is cloneable. Any non-nil
	// error indicates there is a problem.
	IsCloneable(ctx context.Context, remoteURL *url.URL) error
	// CloneCommand returns the command to be executed for cloning from remote.
	CloneCommand(ctx context.Context, remoteURL *url.URL, tmpPath string) (cmd *exec.Cmd, err error)
	// FetchCommand returns the command to be executed for fetching updates from remote.
	FetchCommand(ctx context.Context, remoteURL *url.URL) (cmd *exec.Cmd, configRemoteOpts bool, err error)
	// RemoteShowCommand returns the command to be executed for showing remote.
	RemoteShowCommand(ctx context.Context, remoteURL *url.URL) (cmd *exec.Cmd, err error)
}

// GitRepoSyncer is a syncer for Git repositories.
type GitRepoSyncer struct{}

func (s *GitRepoSyncer) Type() string {
	return "git"
}

// IsCloneable checks to see if the Git remote URL is cloneable.
func (s *GitRepoSyncer) IsCloneable(ctx context.Context, remoteURL *url.URL) error {
	if strings.ToLower(string(protocol.NormalizeRepo(api.RepoName(remoteURL.String())))) == "github.com/sourcegraphtest/alwayscloningtest" {
		return nil
	}
	if testGitRepoExists != nil {
		return testGitRepoExists(ctx, remoteURL)
	}

	args := []string{"ls-remote", remoteURL.String(), "HEAD"}
	ctx, cancel := context.WithTimeout(ctx, shortGitCommandTimeout(args))
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := runWithRemoteOpts(ctx, cmd, nil)
	if err != nil {
		if ctxerr := ctx.Err(); ctxerr != nil {
			err = ctxerr
		}
		if len(out) > 0 {
			err = fmt.Errorf("%s (output follows)\n\n%s", err, out)
		}
		return err
	}
	return nil
}

// CloneCommand returns the command to be executed for cloning a Git repository.
func (s *GitRepoSyncer) CloneCommand(ctx context.Context, remoteURL *url.URL, tmpPath string) (cmd *exec.Cmd, err error) {
	if err := os.MkdirAll(tmpPath, os.ModePerm); err != nil {
		return nil, errors.Wrapf(err, "clone failed to create tmp dir")
	}

	cmd = exec.CommandContext(ctx, "git", "init", "--bare", ".")
	cmd.Dir = tmpPath
	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "clone setup failed")
	}

	cmd, _, err = s.FetchCommand(ctx, remoteURL)
	if err != nil {
		return nil, errors.Wrapf(err, "clone setup failed for FetchCommand")
	}
	cmd.Dir = tmpPath
	return cmd, nil
}

// FetchCommand returns the command to be executed for fetching updates of a Git repository.
func (s *GitRepoSyncer) FetchCommand(ctx context.Context, remoteURL *url.URL) (cmd *exec.Cmd, configRemoteOpts bool, err error) {
	configRemoteOpts = true
	if customCmd := customFetchCmd(ctx, remoteURL); customCmd != nil {
		cmd = customCmd
		configRemoteOpts = false
	} else if useRefspecOverrides() {
		cmd = refspecOverridesFetchCmd(ctx, remoteURL)
	} else {
		cmd = exec.CommandContext(ctx, "git", "fetch",
			"--progress", "--prune", remoteURL.String(),
			// Normal git refs
			"+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*",
			// GitHub pull requests
			"+refs/pull/*:refs/pull/*",
			// GitLab merge requests
			"+refs/merge-requests/*:refs/merge-requests/*",
			// Bitbucket pull requests
			"+refs/pull-requests/*:refs/pull-requests/*",
			// Gerrit changesets
			"+refs/changes/*:refs/changes/*",
			// Possibly deprecated refs for sourcegraph zap experiment?
			"+refs/sourcegraph/*:refs/sourcegraph/*")
	}
	return cmd, configRemoteOpts, nil
}

// RemoteShowCommand returns the command to be executed for showing remote of a Git repository.
func (s *GitRepoSyncer) RemoteShowCommand(ctx context.Context, remoteURL *url.URL) (cmd *exec.Cmd, err error) {
	return exec.CommandContext(ctx, "git", "remote", "show", remoteURL.String()), nil
}

// PerforceDepotSyncer is a syncer for Perforce depots.
type PerforceDepotSyncer struct {
	// MaxChanges indicates to only import at most n changes when possible.
	MaxChanges int
}

func (s *PerforceDepotSyncer) Type() string {
	return "perforce"
}

// decomposePerforceRemoteURL decomposes information back from a clone URL for a
// Perforce depot.
func decomposePerforceRemoteURL(remoteURL *url.URL) (username, password, host, depot string, err error) {
	if remoteURL.Scheme != "perforce" {
		return "", "", "", "", errors.New(`scheme is not "perforce"`)
	}
	password, _ = remoteURL.User.Password()
	return remoteURL.User.Username(), password, remoteURL.Host, remoteURL.Path, nil
}

// p4ping sends one message to the Perforce server to check connectivity.
func p4ping(ctx context.Context, host, username, password string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "p4", "ping", "-c", "1")
	cmd.Env = append(os.Environ(),
		"P4PORT="+host,
		"P4USER="+username,
		"P4PASSWD="+password,
	)

	out, err := runWith(ctx, cmd, false, nil)
	if err != nil {
		if ctxerr := ctx.Err(); ctxerr != nil {
			err = ctxerr
		}
		if len(out) > 0 {
			err = fmt.Errorf("%s (output follows)\n\n%s", err, out)
		}
		return err
	}
	return nil
}

// p4trust blindly accepts fingerprint of the Perforce server.
func p4trust(ctx context.Context, host string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "p4", "trust", "-y", "-f")
	cmd.Env = append(os.Environ(),
		"P4PORT="+host,
	)

	out, err := runWith(ctx, cmd, false, nil)
	if err != nil {
		if ctxerr := ctx.Err(); ctxerr != nil {
			err = ctxerr
		}
		if len(out) > 0 {
			err = fmt.Errorf("%s (output follows)\n\n%s", err, out)
		}
		return err
	}
	return nil
}

// p4pingWithTrust attempts to ping the Perforce server and performs a trust operation when needed.
func p4pingWithTrust(ctx context.Context, host, username, password string) error {
	// Attempt to check connectivity, may be prompted to trust.
	err := p4ping(ctx, host, username, password)
	if err == nil {
		return nil // The ping worked, session still validate for the user
	}

	if strings.Contains(err.Error(), "To allow connection use the 'p4 trust' command.") {
		err := p4trust(ctx, host)
		if err != nil {
			return errors.Wrap(err, "trust")
		}
		return nil
	}

	// Something unexpected happened, bubble up the error
	return err
}

// IsCloneable checks to see if the Perforce remote URL is cloneable.
func (s *PerforceDepotSyncer) IsCloneable(ctx context.Context, remoteURL *url.URL) error {
	username, password, host, _, err := decomposePerforceRemoteURL(remoteURL)
	if err != nil {
		return errors.Wrap(err, "decompose")
	}

	// FIXME: Need to find a way to determine if depot exists instead of a general ping to the Perforce server.
	return p4pingWithTrust(ctx, host, username, password)
}

// CloneCommand returns the command to be executed for cloning a Perforce depot as a Git repository.
func (s *PerforceDepotSyncer) CloneCommand(ctx context.Context, remoteURL *url.URL, tmpPath string) (*exec.Cmd, error) {
	username, password, host, depot, err := decomposePerforceRemoteURL(remoteURL)
	if err != nil {
		return nil, errors.Wrap(err, "decompose")
	}

	err = p4pingWithTrust(ctx, host, username, password)
	if err != nil {
		return nil, errors.Wrap(err, "ping with trust")
	}

	// Example: git p4 clone --bare --max-changes 1000 //Sourcegraph/@all /tmp/clone-584194180/.git
	args := []string{"p4", "clone", "--bare"}
	if s.MaxChanges > 0 {
		args = append(args, "--max-changes", strconv.Itoa(s.MaxChanges))
	}
	args = append(args, depot+"@all", tmpPath)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(),
		"P4PORT="+host,
		"P4USER="+username,
		"P4PASSWD="+password,
	)

	return cmd, nil
}

// FetchCommand returns the command to be executed for fetching updates of a Perforce depot as a Git repository.
func (s *PerforceDepotSyncer) FetchCommand(ctx context.Context, remoteURL *url.URL) (cmd *exec.Cmd, configRemoteOpts bool, err error) {
	username, password, host, _, err := decomposePerforceRemoteURL(remoteURL)
	if err != nil {
		return nil, false, errors.Wrap(err, "decompose")
	}

	err = p4pingWithTrust(ctx, host, username, password)
	if err != nil {
		return nil, false, errors.Wrap(err, "ping with trust")
	}

	// Example: git p4 sync --max-changes 1000
	args := []string{"p4", "sync"}
	if s.MaxChanges > 0 {
		args = append(args, "--max-changes", strconv.Itoa(s.MaxChanges))
	}

	cmd = exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(),
		"P4PORT="+host,
		"P4USER="+username,
		"P4PASSWD="+password,
	)

	return cmd, false, nil
}

// RemoteShowCommand returns the command to be executed for showing Git remote of a Perforce depot.
func (s *PerforceDepotSyncer) RemoteShowCommand(ctx context.Context, remoteURL *url.URL) (cmd *exec.Cmd, err error) {
	// Remote info is encoded as in the current repository
	return exec.CommandContext(ctx, "git", "remote", "show", "./"), nil
}
