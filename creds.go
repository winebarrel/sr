package sr

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
)

// credentials resolves the profile with role substituted for its sso_role_name,
// and returns the credentials along with the region the command should run in.
//
// The profile is loaded the way any other AWS tool loads it, so the account,
// the start URL and the region keep coming from where they always came from.
// WithSSOProviderOptions is applied after ssocreds.New has taken sso_role_name
// from the profile, so it wins -- which leaves the token cache, and the
// refresh an [sso-session] profile is capable of, the SDK's business.
//
// A permission set that is not assigned fails when the credentials are
// retrieved. What the account has is never listed: that would cost a call in
// front of the one that matters and would only move the same failure earlier.
func credentials(ctx context.Context, profile, role string, optFns ...func(*config.LoadOptions) error) (aws.Credentials, string, error) {
	options := []func(*config.LoadOptions) error{
		config.WithSSOProviderOptions(func(o *ssocreds.Options) {
			o.RoleName = role
		}),
	}

	if profile != "" {
		options = append(options, config.WithSharedConfigProfile(profile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, append(options, optFns...)...)

	if err != nil {
		return aws.Credentials{}, "", err
	}

	// A profile that gets its credentials some other way has no permission set
	// to replace, and the override above was quietly ignored. Running the
	// command against whatever that profile does use is the one outcome nobody
	// asked for.
	if !aws.IsCredentialsProvider(cfg.Credentials, (*ssocreds.Provider)(nil)) {
		return aws.Credentials{}, "", errors.New("the profile does not use IAM Identity Center")
	}

	creds, err := cfg.Credentials.Retrieve(ctx)

	if err != nil {
		return aws.Credentials{}, "", withLoginHint(err)
	}

	return creds, cfg.Region, nil
}

// withLoginHint adds the sign-in to an error about the cached token.
//
// sr signs nobody in: that means opening a browser and waiting for someone to
// come back to it, which is not something to do on behalf of a command that
// was asked to run now. Saying so is all it can do. Both a missing token and
// an expired one arrive as InvalidTokenError.
func withLoginHint(err error) error {
	var invalidToken *ssocreds.InvalidTokenError

	if errors.As(err, &invalidToken) {
		return fmt.Errorf("%w: run `aws sso login`", err)
	}

	return err
}
