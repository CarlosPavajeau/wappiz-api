package jwt

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

var testNow = time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)

type testKeyring struct {
	mu      sync.Mutex
	keys    map[string]parsedJWK
	lookups int
}

func (r *testKeyring) lookup(_ context.Context, kid string) (parsedJWK, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lookups++
	jwk, ok := r.keys[kid]
	if !ok {
		return parsedJWK{}, true, errors.New("signing key not found")
	}
	return jwk, true, nil
}

// newTestVerifier builds a DBVerifier whose key lookups are served from an
// in-memory keyring and whose clock is pinned to testNow.
func newTestVerifier(t *testing.T, opts ...VerifierOption) (*DBVerifier, *ecdsa.PrivateKey, *testKeyring) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	keyring := &testKeyring{keys: map[string]parsedJWK{
		"key-1": {key: &privateKey.PublicKey, alg: "ES256"},
	}}

	opts = append([]VerifierOption{WithClock(func() time.Time { return testNow })}, opts...)
	verifier := NewDBVerifier(nil, opts...)
	verifier.lookup = keyring.lookup
	return verifier, privateKey, keyring
}

func signToken(t *testing.T, privateKey *ecdsa.PrivateKey, kid string, claims gojwt.Claims) string {
	t.Helper()
	token := gojwt.NewWithClaims(gojwt.SigningMethodES256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(privateKey)
	require.NoError(t, err)
	return signed
}

func TestDBVerifierVerify(t *testing.T) {
	ctx := context.Background()

	t.Run("valid token with extra claims", func(t *testing.T) {
		verifier, privateKey, _ := newTestVerifier(t, WithIssuer("wappiz"))
		token := signToken(t, privateKey, "key-1", gojwt.MapClaims{
			"iss":   "wappiz",
			"exp":   testNow.Add(time.Hour).Unix(),
			"id":    "user_123",
			"role":  "admin",
			"email": "user@example.com",
		})

		claims, err := verifier.Verify(ctx, token)
		require.NoError(t, err)
		require.Equal(t, "user_123", claims.UserID)
		require.Equal(t, "admin", claims.Role)
	})

	t.Run("expired token", func(t *testing.T) {
		verifier, privateKey, _ := newTestVerifier(t)
		token := signToken(t, privateKey, "key-1", gojwt.MapClaims{
			"exp": testNow.Add(-time.Hour).Unix(),
			"id":  "user_123",
		})

		_, err := verifier.Verify(ctx, token)
		require.ErrorIs(t, err, ErrTokenExpired)
	})

	t.Run("token not yet valid", func(t *testing.T) {
		verifier, privateKey, _ := newTestVerifier(t)
		token := signToken(t, privateKey, "key-1", gojwt.MapClaims{
			"exp": testNow.Add(2 * time.Hour).Unix(),
			"nbf": testNow.Add(time.Hour).Unix(),
			"id":  "user_123",
		})

		_, err := verifier.Verify(ctx, token)
		require.ErrorIs(t, err, ErrTokenNotYetValid)
	})

	t.Run("wrong issuer", func(t *testing.T) {
		verifier, privateKey, _ := newTestVerifier(t, WithIssuer("wappiz"))
		token := signToken(t, privateKey, "key-1", gojwt.MapClaims{
			"iss": "attacker",
			"exp": testNow.Add(time.Hour).Unix(),
			"id":  "user_123",
		})

		_, err := verifier.Verify(ctx, token)
		require.ErrorIs(t, err, ErrInvalidIssuer)
	})

	t.Run("wrong audience", func(t *testing.T) {
		verifier, privateKey, _ := newTestVerifier(t, WithAudience("api"))
		token := signToken(t, privateKey, "key-1", gojwt.MapClaims{
			"aud": "other",
			"exp": testNow.Add(time.Hour).Unix(),
			"id":  "user_123",
		})

		_, err := verifier.Verify(ctx, token)
		require.ErrorIs(t, err, ErrInvalidAudience)
	})

	t.Run("missing exp is rejected", func(t *testing.T) {
		verifier, privateKey, _ := newTestVerifier(t)
		token := signToken(t, privateKey, "key-1", gojwt.MapClaims{
			"id": "user_123",
		})

		_, err := verifier.Verify(ctx, token)
		require.Error(t, err)
	})

	t.Run("explicit validation time overrides clock", func(t *testing.T) {
		verifier, privateKey, _ := newTestVerifier(t)
		token := signToken(t, privateKey, "key-1", gojwt.MapClaims{
			"exp": testNow.Add(time.Hour).Unix(),
			"id":  "user_123",
		})

		_, err := verifier.Verify(ctx, token, testNow.Add(2*time.Hour))
		require.ErrorIs(t, err, ErrTokenExpired)
	})

	t.Run("HMAC token is rejected before key lookup", func(t *testing.T) {
		verifier, _, keyring := newTestVerifier(t)
		token := gojwt.NewWithClaims(gojwt.SigningMethodHS256, gojwt.MapClaims{
			"exp": testNow.Add(time.Hour).Unix(),
		})
		token.Header["kid"] = "key-1"
		signed, err := token.SignedString([]byte("0123456789abcdef0123456789abcdef"))
		require.NoError(t, err)

		_, err = verifier.Verify(ctx, signed)
		require.ErrorContains(t, err, "not permitted")
		require.Equal(t, 0, keyring.lookups)
	})

	t.Run("missing kid", func(t *testing.T) {
		verifier, privateKey, _ := newTestVerifier(t)
		token := gojwt.NewWithClaims(gojwt.SigningMethodES256, gojwt.MapClaims{
			"exp": testNow.Add(time.Hour).Unix(),
		})
		signed, err := token.SignedString(privateKey)
		require.NoError(t, err)

		_, err = verifier.Verify(ctx, signed)
		require.ErrorContains(t, err, "missing kid")
	})

	t.Run("oversized token", func(t *testing.T) {
		verifier, _, keyring := newTestVerifier(t)
		_, err := verifier.Verify(ctx, strings.Repeat("a", maxTokenLength+1))
		require.ErrorContains(t, err, "exceeds")
		require.Equal(t, 0, keyring.lookups)
	})

	t.Run("oversized kid", func(t *testing.T) {
		verifier, privateKey, keyring := newTestVerifier(t)
		token := signToken(t, privateKey, strings.Repeat("k", maxKidLength+1), gojwt.MapClaims{
			"exp": testNow.Add(time.Hour).Unix(),
		})

		_, err := verifier.Verify(ctx, token)
		require.ErrorContains(t, err, "kid")
		require.Equal(t, 0, keyring.lookups)
	})

	t.Run("signature from unknown key", func(t *testing.T) {
		verifier, _, _ := newTestVerifier(t)
		otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		token := signToken(t, otherKey, "key-1", gojwt.MapClaims{
			"exp": testNow.Add(time.Hour).Unix(),
		})

		_, err = verifier.Verify(ctx, token)
		require.Error(t, err)
	})
}

func TestDBVerifierKeyCache(t *testing.T) {
	ctx := context.Background()

	t.Run("repeated verification uses cached key", func(t *testing.T) {
		verifier, privateKey, keyring := newTestVerifier(t)
		token := signToken(t, privateKey, "key-1", gojwt.MapClaims{
			"exp": testNow.Add(time.Hour).Unix(),
			"id":  "user_123",
		})

		for range 5 {
			_, err := verifier.Verify(ctx, token)
			require.NoError(t, err)
		}
		require.Equal(t, 1, keyring.lookups)
	})

	t.Run("cached key expires after TTL", func(t *testing.T) {
		now := testNow
		verifier, privateKey, keyring := newTestVerifier(t)
		verifier.config.clock = func() time.Time { return now }
		token := signToken(t, privateKey, "key-1", gojwt.MapClaims{
			"exp": testNow.Add(24 * time.Hour).Unix(),
			"id":  "user_123",
		})

		_, err := verifier.Verify(ctx, token)
		require.NoError(t, err)

		now = now.Add(keyCacheTTL + time.Second)
		token = signToken(t, privateKey, "key-1", gojwt.MapClaims{
			"exp": now.Add(time.Hour).Unix(),
			"id":  "user_123",
		})
		_, err = verifier.Verify(ctx, token)
		require.NoError(t, err)
		require.Equal(t, 2, keyring.lookups)
	})

	t.Run("unknown kid is negative-cached", func(t *testing.T) {
		verifier, privateKey, keyring := newTestVerifier(t)
		token := signToken(t, privateKey, "key-unknown", gojwt.MapClaims{
			"exp": testNow.Add(time.Hour).Unix(),
		})

		for range 5 {
			_, err := verifier.Verify(ctx, token)
			require.ErrorContains(t, err, "not found")
		}
		require.Equal(t, 1, keyring.lookups)
	})

	t.Run("transient lookup errors are not cached", func(t *testing.T) {
		verifier, privateKey, _ := newTestVerifier(t)
		lookups := 0
		verifier.lookup = func(context.Context, string) (parsedJWK, bool, error) {
			lookups++
			return parsedJWK{}, false, errors.New("connection refused")
		}
		token := signToken(t, privateKey, "key-1", gojwt.MapClaims{
			"exp": testNow.Add(time.Hour).Unix(),
		})

		for range 2 {
			_, err := verifier.Verify(ctx, token)
			require.ErrorContains(t, err, "connection refused")
		}
		require.Equal(t, 2, lookups)
	})
}

func TestParseECKey(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	publicKeyBytes, err := privateKey.PublicKey.Bytes()
	require.NoError(t, err)
	coordinateSize := (len(publicKeyBytes) - 1) / 2

	entry := jwkEntry{
		Kty: "EC",
		Use: "sig",
		Crv: "P-256",
		X:   base64.RawURLEncoding.EncodeToString(publicKeyBytes[1 : 1+coordinateSize]),
		Y:   base64.RawURLEncoding.EncodeToString(publicKeyBytes[1+coordinateSize:]),
	}

	parsed, err := parseECKey(entry)
	require.NoError(t, err)

	parsedBytes, err := parsed.Bytes()
	require.NoError(t, err)
	require.Equal(t, publicKeyBytes, parsedBytes)
}

func TestParseECKeyRejectsPointOffCurve(t *testing.T) {
	entry := jwkEntry{
		Kty: "EC",
		Use: "sig",
		Crv: "P-256",
		X:   base64.RawURLEncoding.EncodeToString([]byte{1}),
		Y:   base64.RawURLEncoding.EncodeToString([]byte{1}),
	}

	_, err := parseJWK(entry)
	require.Error(t, err)
}
