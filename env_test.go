package sr_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/winebarrel/sr"
)

// TestCmdRunEnv covers what the wrapped command inherits.
func TestCmdRunEnv(t *testing.T) {
	tests := []struct {
		name     string
		profile  string
		env      map[string]string
		expected map[string]string
		absent   []string
	}{
		{
			name:    "unrelated variables are kept",
			profile: "example",
			env:     map[string]string{"TF_LOG": "DEBUG"},
			expected: map[string]string{
				"TF_LOG":     "DEBUG",
				"AWS_REGION": "ap-northeast-1",
			},
		},
		{
			// Left in place, it would send the child back to the very
			// permission set the invocation exists to replace.
			name:     "the profile is dropped",
			profile:  "example",
			env:      map[string]string{"AWS_PROFILE": "example", "AWS_DEFAULT_PROFILE": "example"},
			absent:   []string{"AWS_PROFILE", "AWS_DEFAULT_PROFILE"},
			expected: map[string]string{"AWS_ACCESS_KEY_ID": "AKIAEXAMPLE"},
		},
		{
			// The deprecated spelling has no new value to take, so it has to
			// be gone rather than merely overwritten: sent with keys it does
			// not belong to, it fails in a way that says nothing about why.
			name:     "a stale session token does not survive",
			profile:  "example",
			env:      map[string]string{"AWS_SECURITY_TOKEN": "stale", "AWS_CREDENTIAL_EXPIRATION": "stale"},
			absent:   []string{"AWS_SECURITY_TOKEN"},
			expected: map[string]string{"AWS_SESSION_TOKEN": "token"},
		},
		{
			// A region in the shell outranks the profile, here as everywhere
			// else, so the command lands where it would have landed on its
			// own.
			name:    "a region in the shell wins",
			profile: "example",
			env:     map[string]string{"AWS_REGION": "eu-central-1"},
			expected: map[string]string{
				"AWS_REGION":         "eu-central-1",
				"AWS_DEFAULT_REGION": "eu-central-1",
			},
		},
		{
			// With nothing to pass on, the child resolves a region for itself
			// exactly as it would have without sr in the way.
			name:    "no region at all",
			profile: "no-region",
			absent:  []string{"AWS_REGION", "AWS_DEFAULT_REGION"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			center := startIdentityCenter(t)
			writeToken(t, isolateAWS(t, testConfig), validToken())

			for name, value := range tt.env {
				t.Setenv(name, value)
			}

			var env []string

			cmd := &sr.Cmd{
				Profile: tt.profile,
				Role:    "ReadOnlyAccess",
				Command: []string{"terraform", "plan"},
			}

			err := cmd.Run(center.context(t, func(_, gotEnv []string) error {
				env = gotEnv

				return nil
			}))

			require.NoError(t, err)

			vars := envMap(env)

			for name, value := range tt.expected {
				assert.Equal(value, vars[name], name)
			}

			for _, name := range tt.absent {
				assert.NotContains(vars, name)
			}
		})
	}
}

// TestCmdRunCredentialExpiration covers the one variable the SDKs do not read.
// It is how the command, or whoever is reading its output, can tell how long
// the session has left.
func TestCmdRunCredentialExpiration(t *testing.T) {
	center := startIdentityCenter(t)
	writeToken(t, isolateAWS(t, testConfig), validToken())

	var env []string

	cmd := &sr.Cmd{Profile: "example", Role: "ReadOnlyAccess", Command: []string{"terraform", "plan"}}

	err := cmd.Run(center.context(t, func(_, gotEnv []string) error {
		env = gotEnv

		return nil
	}))

	require.NoError(t, err)
	assert.Equal(t, expiry.Format(time.RFC3339), envMap(env)["AWS_CREDENTIAL_EXPIRATION"])
}
