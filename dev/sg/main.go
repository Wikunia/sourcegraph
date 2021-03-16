package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"
)

var (
	runFlagSet = flag.NewFlagSet("sg run", flag.ExitOnError)

	runCommand = &ffcli.Command{
		Name:       "run",
		ShortUsage: "sg run <command>",
		ShortHelp:  "Run the given command.",
		FlagSet:    runFlagSet,
		Exec: func(ctx context.Context, args []string) error {
			if len(args) != 1 {
				fmt.Printf("ERROR: too many arguments\n\n")
				return flag.ErrHelp
			}

			cmd, ok := conf.Commands[args[0]]
			if !ok {
				fmt.Printf("ERROR: command %q not found :(\n\n", args[0])
				return flag.ErrHelp
			}

			return run(ctx, cmd)
		},
		UsageFunc: func(c *ffcli.Command) string {
			var out strings.Builder

			fmt.Fprintf(&out, "USAGE\n")
			fmt.Fprintf(&out, "  sg %s <command>\n", c.Name)
			fmt.Fprintf(&out, "\n")
			fmt.Fprintf(&out, "AVAILABLE COMMANDS IN %s\n", *configFlag)

			for name := range conf.Commands {
				fmt.Fprintf(&out, "  %s\n", name)
			}

			return out.String()
		},
	}
)

var (
	runSetFlagSet = flag.NewFlagSet("sg run-set", flag.ExitOnError)

	runSetCommand = &ffcli.Command{
		Name:       "run-set",
		ShortUsage: "sg run-set <commandset>",
		ShortHelp:  "Run the given command set.",
		FlagSet:    runSetFlagSet,
		Exec:       runExec,
		UsageFunc: func(c *ffcli.Command) string {
			var out strings.Builder

			fmt.Fprintf(&out, "USAGE\n")
			fmt.Fprintf(&out, "  sg %s <commandset>\n", c.Name)
			fmt.Fprintf(&out, "\n")
			fmt.Fprintf(&out, "AVAILABLE COMMANDSETS IN %s\n", *configFlag)

			for name := range conf.Commandsets {
				fmt.Fprintf(&out, "  %s\n", name)
			}

			return out.String()
		},
	}

	runExec = func(ctx context.Context, args []string) error {
		if len(args) != 1 {
			fmt.Printf("ERROR: too many arguments\n\n")
			return flag.ErrHelp
		}

		names, ok := conf.Commandsets[args[0]]
		if !ok {
			fmt.Printf("ERROR: commandset %q not found :(\n\n", args[0])
			return flag.ErrHelp
		}

		cmds := make([]Command, 0, len(names))
		for _, name := range names {
			cmd, ok := conf.Commands[name]
			if !ok {
				return fmt.Errorf("command %q not found in commandset %q", name, args[0])
			}

			cmds = append(cmds, cmd)
		}

		return run(ctx, cmds...)
	}
)

var (
	startFlagSet = flag.NewFlagSet("sg start", flag.ExitOnError)

	startCommand = &ffcli.Command{
		Name:       "start",
		ShortUsage: "sg start>",
		ShortHelp:  "Runs the commandset with the name 'start'.",
		FlagSet:    runFlagSet,
		Exec: func(ctx context.Context, args []string) error {
			if len(args) != 0 {
				fmt.Printf("ERROR: this command doesn't take arguments\n\n")
				return flag.ErrHelp
			}

			return runExec(ctx, []string{"default"})
		},
		UsageFunc: func(c *ffcli.Command) string {
			var out strings.Builder

			fmt.Fprintf(&out, "USAGE\n")
			fmt.Fprintln(&out, "  sg start")

			return out.String()
		},
	}
)

var (
	rootFlagSet = flag.NewFlagSet("sg", flag.ExitOnError)
	configFlag  = rootFlagSet.String("config", "sg.config.yaml", "configuration file")
	conf        *Config

	rootCommand = &ffcli.Command{
		ShortUsage:  "sg [flags] <subcommand>",
		FlagSet:     rootFlagSet,
		Subcommands: []*ffcli.Command{runCommand, runSetCommand, startCommand},
	}
)

func main() {
	if err := rootCommand.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	var err error
	conf, err = ParseConfigFile(*configFlag)
	if err != nil {
		os.Exit(1)
	}

	if err := rootCommand.Run(context.Background()); err != nil {
		os.Exit(1)
	}
}
