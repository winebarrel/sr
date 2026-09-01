package sr_test

import (
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/winebarrel/sr"
)

func TestCmdParse(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		profile string
		alias   map[string]string
		role    string
		command []string
		errMsg  string
	}{
		{
			name:    "role and command",
			args:    []string{"-r", "ReadOnlyAccess", "terraform", "plan"},
			alias:   map[string]string{},
			role:    "ReadOnlyAccess",
			command: []string{"terraform", "plan"},
		},
		{
			name:    "profile flag",
			args:    []string{"-p", "example", "-r", "ReadOnlyAccess", "aws", "s3", "ls"},
			profile: "example",
			alias:   map[string]string{},
			role:    "ReadOnlyAccess",
			command: []string{"aws", "s3", "ls"},
		},
		{
			// Everything from the command onwards belongs to it, including
			// anything that looks like a flag of sr's own.
			name:    "command keeps its own flags",
			args:    []string{"-r", "ReadOnlyAccess", "aws", "s3", "ls", "--profile", "other", "--version"},
			alias:   map[string]string{},
			role:    "ReadOnlyAccess",
			command: []string{"aws", "s3", "ls", "--profile", "other", "--version"},
		},
		{
			// Parsing hands the separator over with the rest; it is Run that
			// has to drop it, which TestCmdRunSeparator covers.
			name:    "explicit separator survives parsing",
			args:    []string{"-r", "ReadOnlyAccess", "--", "terraform", "plan"},
			alias:   map[string]string{},
			role:    "ReadOnlyAccess",
			command: []string{"--", "terraform", "plan"},
		},
		{
			name:    "aliases",
			args:    []string{"-a", "ro=ReadOnlyAccess,admin=AdministratorAccess", "-r", "ro", "terraform", "plan"},
			alias:   map[string]string{"ro": "ReadOnlyAccess", "admin": "AdministratorAccess"},
			role:    "ro",
			command: []string{"terraform", "plan"},
		},
		{
			// There is no permission set to fall back on: taking the one the
			// profile names would run the command as whatever sr exists to
			// avoid.
			name:   "no role",
			args:   []string{"terraform", "plan"},
			errMsg: "missing flags: --role",
		},
		{
			name:   "role without a command",
			args:   []string{"-r", "ReadOnlyAccess"},
			errMsg: `expected "<command> ..."`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			t.Setenv("AWS_PROFILE", "")
			t.Setenv("SR_ALIAS", "")

			var cli struct {
				Version kong.VersionFlag
				sr.Cmd
			}

			parser, err := kong.New(&cli, kong.Name("sr"), kong.Vars{"version": ""})
			require.NoError(t, err)

			_, err = parser.Parse(tt.args)

			if tt.errMsg != "" {
				assert.ErrorContains(err, tt.errMsg)
				return
			}

			require.NoError(t, err)
			assert.Equal(tt.profile, cli.Profile)
			assert.Equal(tt.alias, cli.Alias)
			assert.Equal(tt.role, cli.Role)
			assert.Equal(tt.command, cli.Command)
		})
	}
}

func TestCmdRun(t *testing.T) {
	assert := assert.New(t)

	center := startIdentityCenter(t)
	writeToken(t, isolateAWS(t, testConfig), validToken())

	var argv, env []string

	cmd := &sr.Cmd{
		Profile: "example",
		Role:    "ReadOnlyAccess",
		Command: []string{"terraform", "plan"},
	}

	err := cmd.Run(center.context(t, func(gotArgv, gotEnv []string) error {
		argv, env = gotArgv, gotEnv

		return nil
	}))

	require.NoError(t, err)

	// The permission set asked for is the one that was requested, not the
	// AdministratorAccess the profile names.
	require.NotNil(t, center.Request)
	assert.Equal("/federation/credentials", center.Request.URL.Path)
	assert.Equal("123456789012", center.Request.URL.Query().Get("account_id"))
	assert.Equal("ReadOnlyAccess", center.Request.URL.Query().Get("role_name"))
	assert.Equal("the-token", center.Request.Header.Get("x-amz-sso_bearer_token"))

	assert.Equal([]string{"terraform", "plan"}, argv)

	vars := envMap(env)
	assert.Equal("AKIAEXAMPLE", vars["AWS_ACCESS_KEY_ID"])
	assert.Equal("secret", vars["AWS_SECRET_ACCESS_KEY"])
	assert.Equal("token", vars["AWS_SESSION_TOKEN"])
	assert.Equal(expiry.Format(time.RFC3339), vars["AWS_CREDENTIAL_EXPIRATION"])
	assert.Equal("ap-northeast-1", vars["AWS_REGION"])
	assert.Equal("ap-northeast-1", vars["AWS_DEFAULT_REGION"])
}

// TestCmdRunAlias covers a short name standing in for a permission set. The
// expansion is local, so what reaches Identity Center is the full name.
func TestCmdRunAlias(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		alias    map[string]string
		expected string
	}{
		{
			name:     "alias",
			role:     "ro",
			alias:    map[string]string{"ro": "ReadOnlyAccess"},
			expected: "ReadOnlyAccess",
		},
		{
			// A permission set name is always usable as written, whether or
			// not anyone has given it a short name.
			name:     "full name passes through",
			role:     "ReadOnlyAccess",
			alias:    map[string]string{"ro": "ReadOnlyAccess"},
			expected: "ReadOnlyAccess",
		},
		{
			// Matching is exact. A permission set differing only in case is a
			// different permission set to Identity Center, so guessing at the
			// case here would only produce a confusing failure later.
			name:     "case must match",
			role:     "RO",
			alias:    map[string]string{"ro": "ReadOnlyAccess"},
			expected: "RO",
		},
		{
			// Aliases are read from the environment, so there are often none.
			name:     "no aliases",
			role:     "ReadOnlyAccess",
			expected: "ReadOnlyAccess",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			center := startIdentityCenter(t)
			writeToken(t, isolateAWS(t, testConfig), validToken())

			cmd := &sr.Cmd{
				Profile: "example",
				Role:    tt.role,
				Alias:   tt.alias,
				Command: []string{"terraform", "plan"},
			}

			err := cmd.Run(center.context(t, func(_, _ []string) error { return nil }))

			require.NoError(t, err)
			assert.Equal(t, tt.expected, center.Request.URL.Query().Get("role_name"))
		})
	}
}

// TestCmdRunSeparator covers the "--" anyone used to a wrapper writes out of
// habit. Neither the shell nor the parser removes it, so left alone it would
// be taken for the program to run.
func TestCmdRunSeparator(t *testing.T) {
	center := startIdentityCenter(t)
	writeToken(t, isolateAWS(t, testConfig), validToken())

	var argv []string

	cmd := &sr.Cmd{
		Profile: "example",
		Role:    "ReadOnlyAccess",
		Command: []string{"--", "terraform", "plan"},
	}

	err := cmd.Run(center.context(t, func(gotArgv, _ []string) error {
		argv = gotArgv

		return nil
	}))

	require.NoError(t, err)
	assert.Equal(t, []string{"terraform", "plan"}, argv)
}
