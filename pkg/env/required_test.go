package env

import (
	"testing"

	"github.com/ghdrope/oci-registry-mirror/internal/testhelper"
)

// TestRequire verifies the behavior of required environment variables.
func TestRequire(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		value       string
		expectError bool
	}{
		{
			name:        "value is set",
			key:         "TEST_KEY",
			value:       "value",
			expectError: false,
		},
		{
			name:        "value is empty",
			key:         "EMPTY_KEY",
			value:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			cleanup := testhelper.SetEnv(tt.key, tt.value)
			defer cleanup()

			val, err := Require(tt.key)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}

				if _, ok := err.(MissingError); !ok {
					t.Fatalf("expected MissingError, got %T", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if val != tt.value {
				t.Errorf("unexpected value: got %q, want %q", val, tt.value)
			}
		})
	}
}

// TestMust verifies that Must returns the value or panics when missing.
func TestMust(t *testing.T) {
	t.Run("valid value", func(t *testing.T) {
		cleanup := testhelper.SetEnv("MUST_KEY", "ok")
		defer cleanup()

		val := Must("MUST_KEY")

		if val != "ok" {
			t.Errorf("unexpected value: got %q, want %q", val, "ok")
		}
	})

	t.Run("panic on missing value", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic, got none")
			}
		}()

		_ = Must("MISSING_KEY")
	})
}
