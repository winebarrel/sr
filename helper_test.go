package sr_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/stretchr/testify/require"
	"github.com/winebarrel/sr"
)

const startURL = "https://example.awsapps.com/start"

// testConfig covers both what sr is for -- a profile whose sso_role_name is
// not the permission set being asked for -- and a profile it has to refuse.
const testConfig = `
[profile example]
sso_start_url = ` + startURL + `
sso_region = us-east-1
sso_account_id = 123456789012
sso_role_name = AdministratorAccess
region = ap-northeast-1

[profile no-region]
sso_start_url = ` + startURL + `
sso_region = us-east-1
sso_account_id = 123456789012
sso_role_name = AdministratorAccess

[profile keys]
aws_access_key_id = AKIAEXAMPLE
aws_secret_access_key = secret
`

// expiry is when the stub's credentials run out.
var expiry = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

// identityCenter is a stub of the one call sr makes.
type identityCenter struct {
	// URL is where the stub is listening.
	URL string

	// Request is the last request it received, for tests that care what was
	// asked for rather than what came back.
	Request *http.Request

	// Status, when set, is returned instead of credentials, the way Identity
	// Center answers for a permission set that is not assigned.
	Status int
}

// startIdentityCenter starts a stub that answers GetRoleCredentials.
func startIdentityCenter(t *testing.T) *identityCenter {
	t.Helper()

	center := &identityCenter{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		center.Request = r

		if center.Status != 0 {
			w.Header().Set("X-Amzn-Errortype", "ForbiddenException")
			w.WriteHeader(center.Status)
			json.NewEncoder(w).Encode(map[string]any{"message": "No access"}) //nolint:errcheck

			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"roleCredentials": map[string]any{
				"accessKeyId":     "AKIAEXAMPLE",
				"secretAccessKey": "secret",
				"sessionToken":    "token",
				"expiration":      expiry.UnixMilli(),
			},
		})
	}))

	t.Cleanup(server.Close)
	center.URL = server.URL

	return center
}

// context returns a Context that runs exec and talks to the stub.
func (center *identityCenter) context(t *testing.T, exec func(argv, env []string) error) *sr.Context {
	t.Helper()

	target, err := url.Parse(center.URL)
	require.NoError(t, err)

	// Every AWS request is sent to the stub instead. Replacing the HTTP client
	// is the only way to intercept the Identity Center call: credentials are
	// resolved before endpoints are, so the client that makes it is built
	// before any configured endpoint exists to be picked up.
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req = req.Clone(req.Context())
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = ""

		return http.DefaultTransport.RoundTrip(req)
	})}

	return &sr.Context{
		Exec:        exec,
		LoadOptions: []func(*config.LoadOptions) error{config.WithHTTPClient(client)},
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// isolateAWS points the AWS configuration at a temporary directory holding
// config, so that whatever the machine running the tests has in ~/.aws cannot
// take part. The directory it returns stands in for the home directory, and is
// where a cached SSO token would be looked for.
func isolateAWS(t *testing.T, config string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config")

	require.NoError(t, os.WriteFile(path, []byte(config), 0600))

	t.Setenv("HOME", dir)
	t.Setenv("AWS_CONFIG_FILE", path)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials"))

	// Anything the machine running the tests happens to export would otherwise
	// reach the profile resolution, the credential chain, or the child.
	for _, name := range []string{
		"SR_ALIAS",
		"AWS_PROFILE",
		"AWS_DEFAULT_PROFILE",
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
		"AWS_SECURITY_TOKEN",
		"AWS_CREDENTIAL_EXPIRATION",
		"AWS_REGION",
		"AWS_DEFAULT_REGION",
	} {
		t.Setenv(name, "")
	}

	return dir
}

// writeToken puts body where the AWS CLI would have cached a token for the
// start URL, under the given home directory.
func writeToken(t *testing.T, home, body string) {
	t.Helper()

	path, err := ssocreds.StandardCachedTokenFilepath(startURL)
	require.NoError(t, err)
	require.Equal(t, home, path[:len(home)], "the cache path must be inside the test home directory")

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte(body), 0600))
}

// validToken is a cached token that has not expired.
func validToken() string {
	return `{"accessToken":"the-token","expiresAt":"` + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + `"}`
}

// expiredToken is a cached token from a session that has since ended.
func expiredToken() string {
	return `{"accessToken":"the-token","expiresAt":"2020-01-01T00:00:00Z"}`
}

// envMap turns an environment into a map, keeping the last value set for a
// name the way exec does.
func envMap(env []string) map[string]string {
	out := map[string]string{}

	for _, entry := range env {
		name, value, _ := strings.Cut(entry, "=")
		out[name] = value
	}

	return out
}

// refuseToRun is an Exec for tests where the command must never start.
func refuseToRun(t *testing.T) func(argv, env []string) error {
	t.Helper()

	return func(_, _ []string) error {
		t.Error("the command must not run")

		return nil
	}
}
