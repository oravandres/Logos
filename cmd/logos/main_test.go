package main

import (
	"strings"
	"testing"
)

// TestParseMode pins the CLI surface that MiMi's Deployment manifest relies
// on: `args: ["migrate"]` in the initContainer and `args: ["serve"]` in the
// main container must route to their dedicated modes, and a typo in either
// must surface as a loud error (so the pod crash-loops with a diagnostic log
// rather than silently falling back to "migrate + serve" and opening a
// listener inside what was meant to be a run-to-completion init container).
func TestParseMode(t *testing.T) {
	t.Parallel()

	type want struct {
		mode   mode
		errSub string
	}

	tests := []struct {
		name string
		args []string
		want want
	}{
		{
			name: "no subcommand preserves zero-arg backward compat",
			args: []string{"logos"},
			want: want{mode: modeAll},
		},
		{
			name: "empty args slice also picks the default",
			args: []string{},
			want: want{mode: modeAll},
		},
		{
			name: "migrate subcommand",
			args: []string{"logos", "migrate"},
			want: want{mode: modeMigrate},
		},
		{
			name: "serve subcommand",
			args: []string{"logos", "serve"},
			want: want{mode: modeServe},
		},
		{
			name: "help subcommand",
			args: []string{"logos", "help"},
			want: want{mode: modeHelp},
		},
		{
			name: "help via -h",
			args: []string{"logos", "-h"},
			want: want{mode: modeHelp},
		},
		{
			name: "help via --help",
			args: []string{"logos", "--help"},
			want: want{mode: modeHelp},
		},
		{
			name: "unknown subcommand rejected loudly",
			args: []string{"logos", "migrat"},
			want: want{errSub: `unknown subcommand "migrat"`},
		},
		{
			name: "extra positional argument rejected",
			args: []string{"logos", "migrate", "--force"},
			want: want{errSub: "expected at most one subcommand"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseMode(tt.args)

			if tt.want.errSub != "" {
				if err == nil {
					t.Fatalf("parseMode(%v) error = nil, want substring %q", tt.args, tt.want.errSub)
				}
				if !strings.Contains(err.Error(), tt.want.errSub) {
					t.Fatalf("parseMode(%v) error = %v, want substring %q", tt.args, err, tt.want.errSub)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseMode(%v) error = %v, want nil", tt.args, err)
			}
			if got != tt.want.mode {
				t.Fatalf("parseMode(%v) = %d, want %d", tt.args, got, tt.want.mode)
			}
		})
	}
}
