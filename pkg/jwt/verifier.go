package jwt

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
	"wappiz/pkg/db"

	gojwt "github.com/golang-jwt/jwt/v5"
	"golang.org/x/sync/singleflight"
)

const (
	defaultLeeway = 10 * time.Second

	// maxTokenLength bounds the accepted JWT size so oversized inputs are
	// rejected before any decoding work. Real tokens from the auth service are
	// well under 4 KiB even with RSA signatures.
	maxTokenLength = 8 * 1024

	// maxKidLength bounds the key ID read from the unverified token header
	// before it is used in a database query.
	maxKidLength = 256

	// keyCacheTTL is how long a signing key fetched from the database is
	// reused before being re-fetched. Key rotation adds new kids, which miss
	// the cache and are fetched immediately, so a long TTL is safe.
	keyCacheTTL = 10 * time.Minute

	// negativeKeyCacheTTL is how long a "kid not found" result is remembered.
	// It throttles database lookups for tokens with bogus kids while keeping
	// the delay short for keys that appear mid-rotation.
	negativeKeyCacheTTL = 30 * time.Second
)

type verifierConfig struct {
	issuer   string
	audience string
	leeway   time.Duration
	clock    func() time.Time
}

// VerifierOption customizes token claim validation.
type VerifierOption func(*verifierConfig)

// WithIssuer requires tokens to contain the expected issuer.
func WithIssuer(issuer string) VerifierOption {
	return func(c *verifierConfig) {
		c.issuer = issuer
	}
}

// WithAudience requires the token's aud claim to include the expected audience.
func WithAudience(audience string) VerifierOption {
	return func(c *verifierConfig) {
		c.audience = audience
	}
}

// WithLeeway allows a bounded clock skew for time-based claims.
func WithLeeway(leeway time.Duration) VerifierOption {
	return func(c *verifierConfig) {
		c.leeway = leeway
	}
}

// WithClock overrides the time source used for claims validation and key
// cache expiry. Intended for tests; production should rely on the default
// (time.Now).
func WithClock(clock func() time.Time) VerifierOption {
	return func(c *verifierConfig) {
		c.clock = clock
	}
}

// DBVerifier validates JWTs using public keys fetched from the database jwks
// table. Parsed keys are cached in memory (see keyCacheTTL), so steady-state
// verification performs no database queries. It is safe for concurrent use.
type DBVerifier struct {
	dbtx   db.DBTX
	config verifierConfig

	// lookup resolves a kid to a parsed key. It defaults to the database
	// lookup and exists as a seam so tests can inject keys without a database.
	lookup func(ctx context.Context, kid string) (jwk parsedJWK, cacheable bool, err error)

	mu    sync.RWMutex
	keys  map[string]cachedKey
	group singleflight.Group
}

var _ Verifier[*Claims] = (*DBVerifier)(nil)

// cachedKey is a key lookup result with an expiry. err is non-nil for
// negative entries (deterministic lookup failures such as unknown kid or
// malformed key data).
type cachedKey struct {
	jwk       parsedJWK
	err       error
	expiresAt time.Time
}

// NewDBVerifier creates a DBVerifier backed by the given database connection.
func NewDBVerifier(dbtx db.DBTX, opts ...VerifierOption) *DBVerifier {
	config := verifierConfig{leeway: defaultLeeway}
	for _, opt := range opts {
		opt(&config)
	}
	v := &DBVerifier{
		dbtx:   dbtx,
		config: config,
		keys:   make(map[string]cachedKey),
	}
	v.lookup = v.lookupKeyFromDB
	return v
}

// Verify validates the JWT string and returns its claims.
func (v *DBVerifier) Verify(ctx context.Context, tokenStr string, at ...time.Time) (*Claims, error) {
	if len(tokenStr) > maxTokenLength {
		return nil, fmt.Errorf("token exceeds %d bytes", maxTokenLength)
	}

	kid, alg, err := extractTokenHeader(tokenStr)
	if err != nil {
		return nil, errors.New("malformed token")
	}
	if kid == "" {
		return nil, errors.New("token is missing kid")
	}
	if len(kid) > maxKidLength {
		return nil, errors.New("token kid exceeds maximum length")
	}

	// Enforce algorithm allowlist before touching any key material.
	// This prevents algorithm confusion attacks (e.g. RS256 -> HS256, alg:none).
	if !isAllowedAlg(alg) {
		return nil, fmt.Errorf("algorithm %q is not permitted", alg)
	}

	parsedKey, err := v.cachedKeyLookup(ctx, kid)
	if err != nil {
		return nil, err
	}
	if parsedKey.alg != "" && parsedKey.alg != alg {
		return nil, fmt.Errorf("token algorithm %q does not match key algorithm %q", alg, parsedKey.alg)
	}

	claims := &Claims{}
	token, err := v.newParser(alg, at).ParseWithClaims(tokenStr, claims, func(t *gojwt.Token) (any, error) {
		if t.Method.Alg() != alg {
			return nil, errors.New("unexpected signing method")
		}
		return parsedKey.key, nil
	})
	if err != nil {
		return nil, mapVerificationError(err)
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

// cachedKeyLookup returns the parsed key for kid, fetching it from the
// database on cache miss. Concurrent misses for the same kid are collapsed
// into a single query.
func (v *DBVerifier) cachedKeyLookup(ctx context.Context, kid string) (parsedJWK, error) {
	v.mu.RLock()
	entry, ok := v.keys[kid]
	v.mu.RUnlock()
	if ok && v.now().Before(entry.expiresAt) {
		return entry.jwk, entry.err
	}

	result, err, _ := v.group.Do(kid, func() (any, error) {
		return v.refreshKey(ctx, kid), nil
	})
	if err != nil {
		return parsedJWK{}, err
	}
	entry, ok = result.(cachedKey)
	if !ok {
		return parsedJWK{}, errors.New("unexpected key cache entry type")
	}
	return entry.jwk, entry.err
}

// refreshKey fetches the key for kid and stores the result in the cache.
// Transient database failures are returned but not cached.
func (v *DBVerifier) refreshKey(ctx context.Context, kid string) cachedKey {
	jwk, cacheable, err := v.lookup(ctx, kid)
	entry := cachedKey{jwk: jwk, err: err}
	if err != nil && !cacheable {
		return entry
	}

	ttl := keyCacheTTL
	if err != nil {
		ttl = negativeKeyCacheTTL
	}
	entry.expiresAt = v.now().Add(ttl)

	v.mu.Lock()
	v.keys[kid] = entry
	v.mu.Unlock()
	return entry
}

// lookupKeyFromDB fetches the public key for the given kid from the database.
// cacheable reports whether an error is deterministic for this kid (unknown
// kid, malformed key data) and therefore safe to negative-cache; transient
// database errors are not.
func (v *DBVerifier) lookupKeyFromDB(ctx context.Context, kid string) (jwk parsedJWK, cacheable bool, err error) {
	row, err := db.Query.FindJWKByID(ctx, v.dbtx, kid)
	if errors.Is(err, sql.ErrNoRows) {
		return parsedJWK{}, true, fmt.Errorf("signing key %q not found", kid)
	}
	if err != nil {
		return parsedJWK{}, false, fmt.Errorf("jwk lookup: %w", err)
	}

	var entry jwkEntry
	if err := json.Unmarshal([]byte(row.PublicKey), &entry); err != nil {
		return parsedJWK{}, true, fmt.Errorf("parse public key for kid %q: %w", kid, err)
	}

	jwk, err = parseJWK(entry)
	if err != nil {
		return parsedJWK{}, true, err
	}
	return jwk, true, nil
}

func (v *DBVerifier) newParser(alg string, at []time.Time) *gojwt.Parser {
	opts := []gojwt.ParserOption{
		gojwt.WithLeeway(v.config.leeway),
		gojwt.WithValidMethods([]string{alg}),
		gojwt.WithStrictDecoding(),
		gojwt.WithExpirationRequired(),
	}
	if v.config.issuer != "" {
		opts = append(opts, gojwt.WithIssuer(v.config.issuer))
	}
	if v.config.audience != "" {
		opts = append(opts, gojwt.WithAudience(v.config.audience))
	}
	switch {
	case len(at) > 0:
		validationTime := at[0]
		opts = append(opts, gojwt.WithTimeFunc(func() time.Time { return validationTime }))
	case v.config.clock != nil:
		opts = append(opts, gojwt.WithTimeFunc(v.config.clock))
	}
	return gojwt.NewParser(opts...)
}

func (v *DBVerifier) now() time.Time {
	if v.config.clock != nil {
		return v.config.clock()
	}
	return time.Now()
}

// mapVerificationError translates golang-jwt sentinel errors into this
// package's errors so callers never depend on the underlying library.
func mapVerificationError(err error) error {
	switch {
	case errors.Is(err, gojwt.ErrTokenExpired):
		return ErrTokenExpired
	case errors.Is(err, gojwt.ErrTokenNotValidYet):
		return ErrTokenNotYetValid
	case errors.Is(err, gojwt.ErrTokenInvalidIssuer):
		return ErrInvalidIssuer
	case errors.Is(err, gojwt.ErrTokenInvalidAudience):
		return ErrInvalidAudience
	default:
		return err
	}
}
