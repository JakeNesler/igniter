package main

import (
	"os"
	"testing"
	"time"
)

func TestEnvOr(t *testing.T) {
	const key = "IGNITER_TEST_ENV_OR"

	t.Run("default when unset", func(t *testing.T) {
		old, ok := os.LookupEnv(key)
		os.Unsetenv(key)
		t.Cleanup(func() {
			if ok {
				os.Setenv(key, old)
			} else {
				os.Unsetenv(key)
			}
		})
		if got, want := envOr(key, "default"), "default"; got != want {
			t.Fatalf("envOr = %q, want %q", got, want)
		}
	})

	t.Run("default when empty", func(t *testing.T) {
		t.Setenv(key, "")
		if got, want := envOr(key, "default"), "default"; got != want {
			t.Fatalf("envOr = %q, want %q", got, want)
		}
	})

	t.Run("value when set", func(t *testing.T) {
		t.Setenv(key, "configured")
		if got, want := envOr(key, "default"), "configured"; got != want {
			t.Fatalf("envOr = %q, want %q", got, want)
		}
	})
}

func TestEnvDur(t *testing.T) {
	const key = "IGNITER_TEST_ENV_DUR"
	def := 5 * time.Minute

	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{
			name: "empty falls back to default",
			want: def,
		},
		{
			name:  "invalid falls back to default",
			value: "not-a-duration",
			want:  def,
		},
		{
			name:  "valid duration",
			value: "90s",
			want:  90 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(key, tt.value)
			if got := envDur(key, def); got != tt.want {
				t.Fatalf("envDur = %v, want %v", got, tt.want)
			}
		})
	}
}
