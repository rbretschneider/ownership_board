package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// oidcFlowLifetime is how long a sign-in may take at the identity provider.
const oidcFlowLifetime = 10 * time.Minute

// OIDCConfig configures SSO. An empty IssuerURL turns SSO off entirely.
type OIDCConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	ButtonLabel  string
}

// OIDCFlow is the state carried through the browser between start and
// callback, in a signed cookie.
type OIDCFlow struct {
	State    string
	Nonce    string
	Verifier string
	// LinkUserID is set when a signed-in user is linking SSO to their account.
	LinkUserID int64
}

// OIDC runs the Authorization Code flow with PKCE, state and nonce. Endpoints
// come from issuer discovery. The provider is discovered on first use, so the
// board still boots while the identity provider is down.
type OIDC struct {
	config OIDCConfig
	signer signer

	mu       sync.Mutex
	provider *oidc.Provider
}

// NewOIDC builds the SSO client. It signs flow cookies with the same secret as
// access tokens, under a different audience.
func NewOIDC(config OIDCConfig, service *Service) *OIDC {
	return &OIDC{config: config, signer: service.signer}
}

// IsEnabled reports whether SSO is configured.
func (o *OIDC) IsEnabled() bool {
	return o.config.IssuerURL != ""
}

// ButtonLabel is the text of the sign-in button.
func (o *OIDC) ButtonLabel() string {
	if o.config.ButtonLabel == "" {
		return "Sign in with SSO"
	}
	return o.config.ButtonLabel
}

// Start begins a sign-in. It returns the provider URL to redirect to and the
// sealed flow state to keep in a cookie.
func (o *OIDC) Start(ctx context.Context, linkUserID int64) (string, string, error) {
	oauth, _, err := o.clients(ctx)
	if err != nil {
		return "", "", err
	}
	flow, err := newFlow(linkUserID)
	if err != nil {
		return "", "", err
	}
	sealed, _, err := o.signer.sign(audienceOIDCFlow, oidcFlowLifetime, map[string]any{
		"state": flow.State, "nonce": flow.Nonce, "verifier": flow.Verifier, "link": flow.LinkUserID,
	})
	if err != nil {
		return "", "", err
	}
	redirect := oauth.AuthCodeURL(flow.State, oidc.Nonce(flow.Nonce), oauth2.S256ChallengeOption(flow.Verifier))
	return redirect, sealed, nil
}

// Finish completes a sign-in from the callback's code and state and returns
// who the provider says signed in, plus the flow that started it. The
// provider's tokens are used here once and then dropped.
func (o *OIDC) Finish(ctx context.Context, sealedFlow, state, code string) (Identity, OIDCFlow, error) {
	flow, err := o.openFlow(sealedFlow)
	if err != nil || state == "" || state != flow.State {
		return Identity{}, OIDCFlow{}, errors.Join(ErrUnauthorized, errors.New("auth: oidc state mismatch"))
	}
	oauth, verifier, err := o.clients(ctx)
	if err != nil {
		return Identity{}, OIDCFlow{}, err
	}
	token, err := oauth.Exchange(ctx, code, oauth2.VerifierOption(flow.Verifier))
	if err != nil {
		return Identity{}, OIDCFlow{}, fmt.Errorf("auth: oidc code exchange: %w", err)
	}
	identity, err := verifyIdentity(ctx, verifier, token, flow.Nonce)
	return identity, flow, err
}

func (o *OIDC) clients(ctx context.Context) (*oauth2.Config, *oidc.IDTokenVerifier, error) {
	if !o.IsEnabled() {
		return nil, nil, errors.New("auth: SSO is not configured")
	}
	provider, err := o.discover(ctx)
	if err != nil {
		return nil, nil, err
	}
	oauth := &oauth2.Config{
		ClientID: o.config.ClientID, ClientSecret: o.config.ClientSecret, RedirectURL: o.config.RedirectURL,
		Endpoint: provider.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "email", "profile"},
	}
	return oauth, provider.Verifier(&oidc.Config{ClientID: o.config.ClientID}), nil
}

// discover caches the provider after the first successful discovery. A failure
// is not cached, so the next sign-in tries again.
func (o *OIDC) discover(ctx context.Context) (*oidc.Provider, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.provider != nil {
		return o.provider, nil
	}
	provider, err := oidc.NewProvider(ctx, o.config.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("auth: oidc discovery for %s: %w", o.config.IssuerURL, err)
	}
	o.provider = provider
	return provider, nil
}

func (o *OIDC) openFlow(sealed string) (OIDCFlow, error) {
	claims, err := o.signer.verify(audienceOIDCFlow, sealed)
	if err != nil {
		return OIDCFlow{}, err
	}
	return OIDCFlow{
		State: claimString(claims, "state"), Nonce: claimString(claims, "nonce"),
		Verifier: claimString(claims, "verifier"), LinkUserID: claimInt(claims, "link"),
	}, nil
}

func newFlow(linkUserID int64) (OIDCFlow, error) {
	state, err := newSecret()
	if err != nil {
		return OIDCFlow{}, err
	}
	nonce, err := newSecret()
	if err != nil {
		return OIDCFlow{}, err
	}
	return OIDCFlow{State: state, Nonce: nonce, Verifier: oauth2.GenerateVerifier(), LinkUserID: linkUserID}, nil
}

func verifyIdentity(ctx context.Context, verifier *oidc.IDTokenVerifier, token *oauth2.Token, nonce string) (Identity, error) {
	rawIDToken, isPresent := token.Extra("id_token").(string)
	if !isPresent {
		return Identity{}, errors.New("auth: oidc response has no id_token")
	}
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return Identity{}, fmt.Errorf("auth: verify id_token: %w", err)
	}
	if idToken.Nonce != nonce {
		return Identity{}, errors.Join(ErrUnauthorized, errors.New("auth: oidc nonce mismatch"))
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("auth: read id_token claims: %w", err)
	}
	return Identity{
		Issuer: idToken.Issuer, Subject: idToken.Subject, Email: strings.TrimSpace(claims.Email),
		IsEmailVerified: claims.EmailVerified, Name: claims.Name,
	}, nil
}
