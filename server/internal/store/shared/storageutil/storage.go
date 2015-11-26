package storageutil

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

// Note: these validation functions are tested at a higher level in store/testsuite/storage.go

// validateBucketOrAppName performs validation of a bucket or app name, which
// just happen to have the same rules applied to them. bucketOrApp should be
// either "bucket" or "app" and is used for the error messages.
func validateBucketOrAppName(bucketOrApp, s string) error {
	if s == "" {
		return fmt.Errorf("%s name may not be an empty string", bucketOrApp)
	}
	for _, r := range s {
		ok := r == '_' || r == '-' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r)
		if !ok {
			return fmt.Errorf("%s name may only contain underscores, dashes, periods, letters and digits", bucketOrApp)
		}
	}
	return nil
}

// ValidateBucketName tells whether or not the bucket name is a valid one. It
// returns an error which should be presented to the user describing what is
// wrong with the name, or nil.
//
// An empty string is considered an error.
func ValidateBucketName(s string) error {
	return validateBucketOrAppName("bucket", s)
}

// ValidateAppName tells whether or not the app name is a valid one. It returns
// an error which should be presented to the user describing what is wrong with
// the name, or nil.
//
// An empty string is considered an error.
func ValidateAppName(s string) error {
	return validateBucketOrAppName("app", s)
}

// ValidateRepoURI tells whether or not the repo URI is a valid one. It returns
// an error which should be presented to the user describing what is wrong with
// the repo URI, or nil.
//
// An empty string is considered an error.
func ValidateRepoURI(s string) error {
	if s == "" {
		return errors.New("repo URI may not be an empty string")
	}
	if strings.TrimSpace(s) != s {
		return errors.New("repo URI may not start or end with whitespace")
	}

	// First parse the string as a URL. For any valid repo URI this should always
	// succeed.
	u, err := url.Parse(s)
	if err != nil {
		return err
	}

	// A valid repo URI never has a scheme. Because of this, net/url.Parse will
	// parse e.g. "bing.com/search?q=dotnet" as:
	//
	//  &url.URL{Scheme:"", Opaque:"", User:(*url.Userinfo)(nil), Host:"", Path:"bing.com/search", RawPath:"", RawQuery:"q=dotnet", Fragment:""}
	//
	if u.Scheme != "" || u.Host != "" { // Note: Host is actually in Path field.
		return errors.New("repo URI may not contain a scheme")
	}
	if u.Opaque != "" {
		return errors.New("repo URI may not contain URL opaque field")
	}
	if u.RawQuery != "" {
		return errors.New("repo URI may not contain URL query parameters")
	}
	if u.Fragment != "" {
		return errors.New("repo URI may not contain URL fragments")
	}
	return nil
}
