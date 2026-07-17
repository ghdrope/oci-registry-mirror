package env

import (
	"fmt"
	"os"
)

// missingError is required when a required environment variable
// is not set or is empty.
type MissingError struct {
	Key string
}

func (e MissingError) Error() string {
	return fmt.Sprintf("environment variable %q is required but not set", e.Key)
}

// Require validates the value of the environment variable identified by key.
//
// If the variable is not set or is empty, it returns a MissingError.
//
// This helper should be used for mandatory configuration values that the
// application cannot run without.
func Require(key string) (string, error) {
	if v := os.Getenv(key); v != "" {
		return v, nil
	}
	return "", MissingError{Key: key}
}

// Must returns the value of the environment variable identified by key.
//
// It panics if the variable is not set or is empty.
//
// This is intended for use in initialization code where failure should stop
// the application immediately (e.g. during startup).
func Must(key string) string {
	v, err := Require(key)
	if err != nil {
		panic(err)
	}
	return v
}
