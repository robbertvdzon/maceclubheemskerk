package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"cloud.google.com/go/auth/credentials/idtoken"
)

type VerifyFunc func(context.Context, string, string, string) (User, error)

func GoogleVerifier() (VerifyFunc, error) {
	return googleVerifier(&http.Client{Timeout: 10 * time.Second})
}
func googleVerifier(client *http.Client) (VerifyFunc, error) {
	validator, err := idtoken.NewValidator(&idtoken.ValidatorOptions{Client: client})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, token, audience, nonce string) (User, error) {
		if audience == "" {
			return User{}, errors.New("missing audience")
		}
		p, err := validator.Validate(ctx, token, audience)
		if err != nil {
			return User{}, errors.New("invalid Google credential")
		}
		return identity(p, nonce, time.Now())
	}, nil
}
func identity(p *idtoken.Payload, nonce string, now time.Time) (User, error) {
	verified, _ := p.Claims["email_verified"].(bool)
	email, _ := p.Claims["email"].(string)
	name, _ := p.Claims["name"].(string)
	tokenNonce, _ := p.Claims["nonce"].(string)
	if (p.Issuer != "https://accounts.google.com" && p.Issuer != "accounts.google.com") || p.Subject == "" || !verified || email == "" || nonce == "" || tokenNonce != nonce || p.Expires <= now.Unix() || p.IssuedAt > now.Add(time.Minute).Unix() {
		return User{}, errors.New("invalid Google identity")
	}
	return User{ID: p.Subject, Name: name, Email: email}, nil
}
