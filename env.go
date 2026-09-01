package sr

import (
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// replacedEnv are the variables the child must not simply inherit.
//
// A named profile is what the SDKs reach for once they are done with the
// environment, so leaving one set would point the child back at the permission
// set being replaced. Stale credentials have to go for a different reason: an
// AWS_SESSION_TOKEN from an unrelated session, sent with keys it does not
// belong to, fails in a way that says nothing about why.
var replacedEnv = []string{
	"AWS_PROFILE",
	"AWS_DEFAULT_PROFILE",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"AWS_SECURITY_TOKEN",
	"AWS_CREDENTIAL_EXPIRATION",
	"AWS_REGION",
	"AWS_DEFAULT_REGION",
}

// childEnv returns base with the credentials and region substituted in.
func childEnv(base []string, creds aws.Credentials, region string) []string {
	env := make([]string, 0, len(base)+len(replacedEnv))

	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")

		if !slices.Contains(replacedEnv, name) {
			env = append(env, entry)
		}
	}

	env = append(env,
		"AWS_ACCESS_KEY_ID="+creds.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY="+creds.SecretAccessKey,
		"AWS_SESSION_TOKEN="+creds.SessionToken,
		// Not read by the SDKs, but it is the only way to tell how long the
		// session has left.
		"AWS_CREDENTIAL_EXPIRATION="+creds.Expires.UTC().Format(time.RFC3339),
	)

	// Without one, the child resolves a region for itself as it would have
	// without sr in the way.
	if region != "" {
		env = append(env,
			"AWS_REGION="+region,
			"AWS_DEFAULT_REGION="+region,
		)
	}

	return env
}
