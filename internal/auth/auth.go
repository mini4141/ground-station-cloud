// Package auth validates operator JWTs (Cognito or the local dev OIDC issuer; same claims).
package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// Operator is the authenticated principal.
type Operator struct{ ID, Name string }

type ctxKey struct{}

// From returns the operator stored by the middleware.
func From(ctx context.Context) Operator { o, _ := ctx.Value(ctxKey{}).(Operator); return o }

// WithOperator stores an operator in ctx (used by tests).
func WithOperator(ctx context.Context, o Operator) context.Context {
	return context.WithValue(ctx, ctxKey{}, o)
}

// Verifier checks bearer tokens. With TrustGateway the signature was already verified by
// the API Gateway JWT authorizer (Lambdas have no internet to fetch JWKS), so claims are only decoded.
type Verifier struct {
	keys         keyfunc.Keyfunc
	TrustGateway bool
}

// NewJWKS builds a verifier backed by a JWKS URL.
func NewJWKS(url string) (*Verifier, error) {
	k, err := keyfunc.NewDefault([]string{url})
	if err != nil {
		return nil, err
	}
	return &Verifier{keys: k}, nil
}

// Verify extracts the operator from an Authorization header value.
func (v *Verifier) Verify(header string) (Operator, error) {
	raw, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return Operator{}, errors.New("missing bearer token")
	}
	claims := jwt.MapClaims{}
	var err error
	if v.TrustGateway {
		_, _, err = jwt.NewParser().ParseUnverified(raw, claims)
	} else {
		_, err = jwt.ParseWithClaims(raw, claims, v.keys.Keyfunc, jwt.WithValidMethods([]string{"RS256"}))
	}
	if err != nil {
		return Operator{}, err
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return Operator{}, errors.New("token has no sub")
	}
	name, _ := claims["email"].(string)
	if name == "" {
		name, _ = claims["username"].(string)
	}
	if name == "" {
		name = sub
	}
	return Operator{ID: sub, Name: name}, nil
}

// Middleware rejects unauthenticated requests except those where public(r) is true.
func (v *Verifier) Middleware(public func(*http.Request) bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if public(r) {
			next.ServeHTTP(w, r)
			return
		}
		op, err := v.Verify(r.Header.Get("Authorization"))
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next.ServeHTTP(w, r.WithContext(WithOperator(r.Context(), op)))
	})
}
