package jwt

import (
	"context"
	"time"
)

// Verifier validates JSON Web Tokens and extracts typed claims.
//
// The type parameter T is the claims type produced on success. During
// verification, the token's payload is unmarshaled into this type, and the
// registered claims are validated automatically (exp, nbf, size limits, and
// optionally iss/aud via [WithIssuer] and [WithAudience]).
//
// Implementations are safe for concurrent use. The verification options are
// captured at construction time and cannot be changed afterward.
type Verifier[T any] interface {
	// Verify validates a JWT and returns the typed claims.
	//
	// Verification proceeds in order:
	//  1. Structure validation (size limits, three dot-separated parts)
	//  2. Base64url decoding of header and payload
	//  3. Algorithm check (must be on the verifier's allowlist)
	//  4. Signature verification
	//  5. JSON parsing into claims type T
	//  6. Registered claims validation (exp, nbf, iss, aud)
	//
	// The context bounds any I/O the implementation performs to resolve the
	// verification key (e.g. a database lookup on key-cache miss).
	//
	// The optional time parameter overrides the clock for claims validation.
	// This is useful for testing; in production, configure the clock via
	// [WithClock] at construction time. If multiple times are passed, only
	// the first is used.
	//
	// Returns specific errors for validation failures:
	//   - [ErrTokenExpired]: exp claim is in the past
	//   - [ErrTokenNotYetValid]: nbf claim is in the future
	//   - [ErrInvalidIssuer]: iss claim doesn't match [WithIssuer] configuration
	//   - [ErrInvalidAudience]: aud claim doesn't include [WithAudience] value
	//
	// Other errors indicate structural problems (malformed token), cryptographic
	// failures (invalid signature, wrong algorithm), or JSON parsing errors.
	//
	// On error, the returned claims value is the zero value of T and should not be used.
	Verify(ctx context.Context, token string, at ...time.Time) (T, error)
}
