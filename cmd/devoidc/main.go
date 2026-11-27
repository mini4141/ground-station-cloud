// Command devoidc is a tiny local OIDC issuer: JWKS endpoint plus a dev login that mints Cognito-shaped JWTs.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/mini4141/ground-station-cloud/internal/platform"
)

const kid = "dev-key-1"

func loadKey(path string) *rsa.PrivateKey {
	if b, err := os.ReadFile(path); err == nil {
		if blk, _ := pem.Decode(b); blk != nil {
			if k, err := x509.ParsePKCS1PrivateKey(blk.Bytes); err == nil {
				return k
			}
		}
	}
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}), 0o600)
	return k
}

func main() {
	platform.Setup(context.Background(), "devoidc")
	key := loadKey(platform.Env("DEVOIDC_KEY", ".local/devoidc.key"))
	issuer := platform.Env("DEVOIDC_ISSUER", "http://localhost:9000")

	mint := func(operator string) (string, error) {
		now := time.Now()
		t := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"sub": "op-" + operator, "email": operator + "@operators.example.test", "username": operator, "iss": issuer,
			"client_id": "console", "token_use": "access", "iat": now.Unix(), "exp": now.Add(12 * time.Hour).Unix(),
		})
		t.Header["kid"] = kid
		return t.SignedString(key)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"status":"ok"}`)) })
	mux.HandleFunc("GET /.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		e := big.NewInt(int64(key.E)).Bytes()
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": kid,
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(e)}}})
	})
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "jwks_uri": issuer + "/.well-known/jwks.json", "authorization_endpoint": issuer + "/login"})
	})
	// implicit-flow style login, like the Cognito Hosted UI: redirects back with the token in the fragment
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		op := r.URL.Query().Get("operator")
		if op == "" {
			op = "demo"
		}
		tok, err := mint(op)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		redirect := r.URL.Query().Get("redirect_uri")
		if redirect == "" {
			http.Error(w, "redirect_uri required", 400)
			return
		}
		http.Redirect(w, r, redirect+"#access_token="+url.QueryEscape(tok)+"&token_type=Bearer", http.StatusFound)
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Operator string `json:"operator"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Operator == "" {
			in.Operator = "demo"
		}
		tok, err := mint(in.Operator)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": tok, "token_type": "Bearer"})
	})

	addr := platform.Env("DEVOIDC_ADDR", ":9000")
	slog.Info("devoidc listening", "addr", addr, "issuer", issuer)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("devoidc", "err", err)
		os.Exit(1)
	}
}
