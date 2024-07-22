package command

import (
	"strings"
	"testing"
)

func TestNewCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  *Command
		err   bool
	}{
		{
			name:  "no command provided",
			input: "",
			want:  nil,
			err:   true,
		},
		{
			name:  "help wanted --help",
			input: "--help",
			want: &Command{
				HelpWanted: true,
			},
		},
		{
			name:  "help wanted -h",
			input: "-h",
			want: &Command{
				HelpWanted: true,
			},
		},
		{
			name:  "unrecognized subcommand",
			input: "unknown",
			want:  nil,
			err:   true,
		},
		{
			name:  "start command -- no remote command provided",
			input: "start",
			want:  nil,
			err:   true,
		},
		{
			name:  "start command -- no remote command provided",
			input: "start --",
			want:  nil,
			err:   true,
		},
		{
			name:  "start command -- no remote command divider provided",
			input: "start echo hello",
			want:  nil,
			err:   true,
		},
		{
			name:  "start command -- remote command provided",
			input: "start -- echo hello",
			want: &Command{
				SubCommand:    Start,
				RemoteCommand: "echo",
				RemoteArgs:    []string{"hello"},
			},
		},
		{
			name:  "start command -- remote command provided with flags",
			input: "start --host=localhost -- echo hello",
			want: &Command{
				SubCommand:    Start,
				Host:          "localhost",
				RemoteCommand: "echo",
				RemoteArgs:    []string{"hello"},
			},
		},
		{
			name:  "start command -- unsupported flag",
			input: "start --unknown -- echo hello",
			want:  nil,
			err:   true,
		},
		{
			name:  "stop command -- job id provided",
			input: "stop 123",
			want: &Command{
				SubCommand: Stop,
				JobID:      "123",
			},
		},
		{
			name:  "stop command -- no job id provided",
			input: "stop --host=localhost",
			want:  nil,
			err:   true,
		},
		{
			name:  "status command -- job id provided",
			input: "status 123",
			want: &Command{
				SubCommand: Status,
				JobID:      "123",
			},
		},
		{
			name:  "status command -- no job id provided",
			input: "status --host=localhost",
			want:  nil,
			err:   true,
		},
		{
			name:  "output command -- job id provided",
			input: "output 123",
			want: &Command{
				SubCommand: Output,
				JobID:      "123",
			},
		},
		{
			name:  "output command -- no job id provided",
			input: "output --host=localhost",
			want:  nil,
			err:   true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewCommand(strings.Split(tt.input, " "))
			if tt.err {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatalf("expected command, got nil")
			}
			if got.SubCommand != tt.want.SubCommand {
				t.Fatalf("expected subcommand %v, got %v", tt.want.SubCommand, got.SubCommand)
			}
		})
	}
}
