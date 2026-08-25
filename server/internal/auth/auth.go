// Package auth handles JWT signing and verification.
//
// Multi-server migration path:
//   - Now (single binary): NewHS256(sharedSecret) — same key signs and verifies.
//   - Later (separate auth service): auth server uses an RSA/Ed25519 private key to sign;
//     game servers call NewVerifier(publicKeyPEM) — no shared secret, no network call to auth.
//     The PlayerStore interface and all game-server code stays unchanged.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const tokenTTL = 7 * 24 * time.Hour // re-login after 7 days of inactivity

// claims is the JWT payload. Minimal by design: the game server always fetches
// fresh player state from the DB; the token is proof of identity only.
type claims struct {
	PlayerID int64 `json:"pid"`
	jwt.RegisteredClaims
}

// Service signs and verifies tokens. Create with NewHS256.
type Service struct {
	secret []byte
}

// NewHS256 creates a Service using HMAC-SHA256 with the given secret.
// In production, read the secret from the JWT_SECRET environment variable.
func NewHS256(secret string) *Service {
	return &Service{secret: []byte(secret)}
}

// Sign creates a signed token for the given player ID.
func (s *Service) Sign(playerID int64) (string, error) {
	c := claims{
		PlayerID: playerID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(s.secret)
}

// Validate parses and verifies a token string. Returns the player ID on success.
func (s *Service) Validate(tokenStr string) (int64, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return 0, err
	}
	c, ok := tok.Claims.(*claims)
	if !ok || !tok.Valid {
		return 0, errors.New("invalid token")
	}
	return c.PlayerID, nil
}
