package sr_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/winebarrel/sr"
)

// TestCmdRunErrors covers what stops the command from running at all.
func TestCmdRunErrors(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		role    string
		token   string
		status  int
		errMsg  string
	}{
		{
			// Nobody has signed in on this machine, or the session was logged
			// out.
			name:    "not signed in",
			profile: "example",
			errMsg:  "run `aws sso login`",
		},
		{
			name:    "expired session",
			profile: "example",
			token:   expiredToken(),
			errMsg:  "run `aws sso login`",
		},
		{
			// There is no permission set to replace, and the override would
			// have gone unnoticed.
			name:    "not an sso profile",
			profile: "keys",
			token:   validToken(),
			errMsg:  "the profile does not use IAM Identity Center",
		},
		{
			name:    "unknown profile",
			profile: "nope",
			token:   validToken(),
			errMsg:  "failed to get shared config profile, nope",
		},
		{
			// Where a mistyped or unassigned permission set lands. Nothing is
			// listed up front, so this is the first sign of it.
			name:    "permission set not assigned",
			profile: "example",
			token:   validToken(),
			status:  403,
			errMsg:  "ForbiddenException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			center := startIdentityCenter(t)
			center.Status = tt.status

			home := isolateAWS(t, testConfig)

			if tt.token != "" {
				writeToken(t, home, tt.token)
			}

			cmd := &sr.Cmd{
				Profile: tt.profile,
				Role:    "ReadOnlyAccess",
				Command: []string{"terraform", "plan"},
			}

			err := cmd.Run(center.context(t, refuseToRun(t)))

			assert.ErrorContains(t, err, tt.errMsg)
		})
	}
}
