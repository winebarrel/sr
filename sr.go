// Package sr runs a command against an IAM Identity Center permission set
// other than the one its profile names.
//
// A profile in ~/.aws/config pins exactly one sso_role_name, so reaching a
// second permission set in the same account has meant editing that file and
// remembering to put it back. sr leaves it alone: the profile still supplies
// the account, the start URL and the region, and only the permission set is
// substituted, for one child process. When it exits there is nothing to undo.
package sr

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
)

// Context is what the command needs from its surroundings.
type Context struct {
	// Exec runs the wrapped command. The command line supplies ExecProcess;
	// tests supply their own.
	Exec func(argv, env []string) error

	// LoadOptions are passed to config.LoadDefaultConfig after the ones sr
	// supplies itself, and so can override them. Nil is the ordinary case.
	LoadOptions []func(*config.LoadOptions) error
}

// Cmd is the command line.
type Cmd struct {
	Profile string            `short:"p" env:"AWS_PROFILE" help:"Profile to take the account, start URL and region from."`
	Role    string            `short:"r" required:"" help:"Permission set to assume, in full or as an alias."`
	Alias   map[string]string `short:"a" env:"SR_ALIAS" mapsep:"," help:"Short names for permission sets, as 'short=PermissionSetName'."`

	Command []string `arg:"" passthrough:"" help:"Command to run."`
}

// Run resolves credentials for the requested permission set and hands the
// command over to them. Nothing is written anywhere: the credentials exist
// only in the environment of the process that replaces this one.
func (cmd *Cmd) Run(cmdCtx *Context) error {
	creds, region, err := credentials(context.Background(), cmd.Profile, resolveRole(cmd.Role, cmd.Alias), cmdCtx.LoadOptions...)

	if err != nil {
		return err
	}

	return cmdCtx.Exec(trimSeparator(cmd.Command), childEnv(os.Environ(), creds, region))
}

// trimSeparator drops the "--" between sr's flags and the command.
//
// Neither the shell nor the parser removes it, so left in place it would be
// taken for the program to run.
func trimSeparator(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		return args[1:]
	}

	return args
}
