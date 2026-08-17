// Package firebase verifies Sign in with Apple / Google ID tokens minted by
// Firebase Auth. The app performs the social flow; we only trust the token.
package firebase

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	fbauth "firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"
)

// Identity is what a verified token tells us about the person.
type Identity struct {
	UID      string
	Email    string
	Name     string
	Picture  string
	Provider string // google.com | apple.com | password | …
}

// Verifier is the port the auth service depends on.
type Verifier interface {
	Verify(ctx context.Context, idToken string) (*Identity, error)
	DeleteUser(ctx context.Context, uid string) error
	Enabled() bool
}

type client struct {
	auth *fbauth.Client
	// False for a project-ID-only client, which can verify but not administer.
	manage bool
}

var _ Verifier = (*client)(nil)

// New builds a verifier. Two ways in, preferred first:
//
//   - a service-account JSON file — full access, including deleting an identity
//     when someone deletes their account.
//   - a bare project ID — verification only, and no secret to store anywhere.
//
// The second works because verifying an ID token needs Google's PUBLIC signing
// certificates, not our credentials: the token is already signed, and all we do
// is check the signature, the audience and the expiry. So social sign-in runs
// in dev without anyone downloading a private key, which is the difference
// between this being configured and being left switched off.
//
// Neither configured returns Disabled{}, so the server still boots.
func New(ctx context.Context, credentialsFile, projectID string) (Verifier, error) {
	var (
		cfg    *firebase.Config
		opts   []option.ClientOption
		manage bool
	)

	switch {
	case credentialsFile != "":
		opts = append(opts, option.WithCredentialsFile(credentialsFile))
		manage = true
	case projectID != "":
		cfg = &firebase.Config{ProjectID: projectID}
		// Without this the SDK hunts for application-default credentials and
		// fails on a machine that has none — which is most of them.
		opts = append(opts, option.WithoutAuthentication())
	default:
		return Disabled{}, nil
	}

	app, err := firebase.NewApp(ctx, cfg, opts...)
	if err != nil {
		return nil, fmt.Errorf("firebase app: %w", err)
	}
	authClient, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("firebase auth: %w", err)
	}
	return &client{auth: authClient, manage: manage}, nil
}

func (c *client) Enabled() bool { return true }

func (c *client) Verify(ctx context.Context, idToken string) (*Identity, error) {
	token, err := c.auth.VerifyIDToken(ctx, idToken)
	if err != nil {
		return nil, fmt.Errorf("verify id token: %w", err)
	}
	id := &Identity{UID: token.UID, Provider: "unknown"}

	if firebaseClaim, ok := token.Claims["firebase"].(map[string]any); ok {
		if p, ok := firebaseClaim["sign_in_provider"].(string); ok {
			id.Provider = p
		}
	}
	if v, ok := token.Claims["email"].(string); ok {
		id.Email = v
	}
	if v, ok := token.Claims["name"].(string); ok {
		id.Name = v
	}
	if v, ok := token.Claims["picture"].(string); ok {
		id.Picture = v
	}
	return id, nil
}

// DeleteUser removes the identity from Firebase when the account is deleted.
//
// Needs a service account: this one writes, so the public certs a verify-only
// client runs on are not enough. The caller treats a failure as a warning, so
// say plainly which half of the setup is missing.
func (c *client) DeleteUser(ctx context.Context, uid string) error {
	if !c.manage {
		return fmt.Errorf(
			"deleting a firebase identity needs FIREBASE_CREDENTIALS_FILE; " +
				"the project-ID-only client can verify tokens but not administer users")
	}
	return c.auth.DeleteUser(ctx, uid)
}

// Disabled stands in when no credentials are configured.
type Disabled struct{}

var _ Verifier = Disabled{}

func (Disabled) Enabled() bool { return false }
func (Disabled) Verify(context.Context, string) (*Identity, error) {
	return nil, fmt.Errorf("firebase auth is not configured")
}
func (Disabled) DeleteUser(context.Context, string) error { return nil }
