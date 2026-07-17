package testhelper

import "os"

// SetEnv sets an environment variable for testing purposes.
//
// It returns a cleanup function that restores the previous value.
func SetEnv(key, value string) func() {
	old, existed := os.LookupEnv(key)

	if err := os.Setenv(key, value); err != nil {
		panic(err)
	}

	return func() {
		if !existed {
			if err := os.Unsetenv(key); err != nil {
				panic(err)
			}
			return
		}

		if err := os.Setenv(key, old); err != nil {
			panic(err)
		}
	}
}

// UnsetEnv removes an environment variable for testing purposes.
//
// It returns a cleanup function that restores the previous value if it existed.
func UnsetEnv(key string) func() {
	old, existed := os.LookupEnv(key)

	if err := os.Unsetenv(key); err != nil {
		panic(err)
	}

	return func() {
		if existed {
			if err := os.Setenv(key, old); err != nil {
				panic(err)
			}
		}
	}
}
