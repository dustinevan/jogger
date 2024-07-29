package command

import (
	"fmt"
	"strings"
)

type SubCommand int

const (
	Start SubCommand = iota
	Stop
	Status
	Output
)

var subCommandStrings = [...]string{
	"start",
	"stop",
	"status",
	"output",
}

func ParseSubCommand(s string) (SubCommand, error) {
	for i, v := range subCommandStrings {
		if v == s {
			return SubCommand(i), nil
		}
	}
	return 0, fmt.Errorf("unsupported subcommand: %s", s)
}

type Flag int

const (
	Help Flag = iota
	Host
	RemoteCommandDelimiter
)

var (
	flagStrings = [...]string{
		"--help",
		"--host",
		"--",
	}
	flagStringMap = map[string]Flag{
		"--help": Help,
		"-h":     Help,
		"--host": Host,
		"-D":     Host,
		"--":     RemoteCommandDelimiter,
	}
)

// ParseFlag parses a flag from a string. If the flag is a boolean flag, the value will be an empty string.
// --help, -h, --host=localhost:7654, -D=localhost:7654, --
func ParseFlag(s string) (flag Flag, value string, err error) {
	parts := strings.SplitN(s, "=", 2)
	if len(parts) == 1 {
		flag, ok := flagStringMap[s]
		if !ok {
			return 0, "", fmt.Errorf("unsupported flag: %s", s)
		}
		return flag, "", nil
	}
	flag, ok := flagStringMap[parts[0]]
	if !ok {
		return 0, "", fmt.Errorf("unsupported flag: %s", s)
	}
	return flag, parts[1], nil
}

func (f Flag) String() string {
	return flagStrings[f]
}

type Command struct {
	SubCommand    SubCommand
	Host          string
	JobID         string
	RemoteCommand string
	RemoteArgs    []string
	HelpWanted    bool
}

func NewCommand(args []string) (*Command, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no command provided")
	}
	// Help is only valid as the first argument
	c := &Command{}
	if args[0] == "--help" || args[0] == "-h" {
		c.HelpWanted = true
		return c, nil
	}

	// If the first argument is not help, it must be a subcommand
	subCommand, err := ParseSubCommand(args[0])
	if err != nil {
		return nil, err
	}
	c.SubCommand = subCommand

	// Parse the rest of the arguments
	args = args[1:]
	for i := 0; i < len(args); i++ {

		// Check for flags first
		// If we find the remote command divider, everything after is the remote command
		if args[i] == "--" {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("no remote command provided")
			}
			c.RemoteCommand = args[i+1]
			c.RemoteArgs = args[i+2:]
			break
		}
		// If the argument starts with a dash, it's a flag.
		// Note that we've already checked for the remote command divider -- above
		if args[i][0] == '-' {
			flag, value, err := ParseFlag(args[i])
			if err != nil {
				return nil, err
			}
			switch flag {
			case Help:
				c.HelpWanted = true
				// If we find the help flag, stop parsing the rest of the arguments
				// This means the help flag can be used anywhere before the remote command divider or jobID
				return c, nil
			case Host:
				c.Host = value
				continue
			default:
				// This means the flag was parsed successfully but no handler exists for it, a programming error
				// this is a CLI, so we return an error instead of panicking
				return nil, fmt.Errorf("unsupported flag: %s", args[i])
			}

		}
		// The argument is not a flag
		if c.SubCommand != Start {
			c.JobID = args[i]
			break
			// This means start was called without a remote command divider
			// 1. The arg doesn't start with a dash, so it's not a flag.
			// 2. The start subcommand is being used.
		} else {
			return nil, fmt.Errorf("no remote command divider provided: use -- to separate the jog command from the remote command")
		}
	}

	// Check for required fields
	if c.SubCommand == Start {
		if c.RemoteCommand == "" {
			return nil, fmt.Errorf("no remote command provided")
		}
	} else {
		if c.JobID == "" {
			return nil, fmt.Errorf("no job id provided")
		}
	}
	return c, nil
}

func (c *Command) String() string {
	var sb strings.Builder
	sb.WriteString("jog ")
	sb.WriteString(subCommandStrings[c.SubCommand])
	if c.HelpWanted {
		sb.WriteString(" ")
		sb.WriteString(flagStrings[Help])
	}
	if c.Host != "" {
		sb.WriteString(" ")
		sb.WriteString(flagStrings[Host])
		sb.WriteString("=")
		sb.WriteString(c.Host)
	}
	if c.RemoteCommand != "" {
		sb.WriteString(" -- ")
		sb.WriteString(c.RemoteCommand)
		for _, a := range c.RemoteArgs {
			sb.WriteString(" ")
			sb.WriteString(a)
		}
	}
	return sb.String()
}

const Usage = `
NAME 
    jog - a simple job runner

SYNOPSIS
    jog start [-D --host address[:port]] -- [command [argument ...]]
    jog [stop | status | output] [-D --host address[:port]] [job_id]
    jog [-h | --help]

ENVIRONMENT VARIABLES -- The following must be set to securely connect to the host:
    export JOGGER_CA_CERT_FILE=   [Absolute path to the self-signed CA certificate pem file]
    export JOGGER_USER_CERT_FILE= [Absolute path to the user certificate pem file]
    export JOGGER_USER_KEY_FILE=  [Absolute path to the user private key pem file]

JOG COMMANDS
    start           start a job -- double dash -- separates the jog command from the remote command
    stop            stop a job
    status          get the status of a job
    output          stream the output of a job

OPTIONS
    -D --host       address[:port] full details: https://github.com/grpc/grpc/blob/master/doc/naming.md
    -h --help       print this usage information

EXAMPLES
    # Starting a job
    $ jog start --host=localhost:7654 -- echo 'echo the job'
    > started: uuid1
    
    # Setting the JOGGER_HOST environment variable means you don't need to use the --host flag every time
    export JOGGER_HOST=localhost:7654
    
    $ jog start -- echo 'run another one'
    > started: uuid2
    
    $ jog stop uuid2
    > uuid2 already exited with status: completed
    
    $ jog start -- long-running-job arg1 arg2 arg3
    > started: uuid3
    
    $ jog status uuid3
    > status: running
    
    $ jog output uuid1
    > log lines starting from the beginning and steaming until
    this command is terminated or the job moves to a done state.

`
