// Package jwt issues and verifies the application's JWTs.
// Access and refresh tokens use SEPARATE secrets and carry a typ claim so one
// can never be replayed as the other.
package jwt

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/chaosapp/backend/internal/common/constants"
	"github.com/chaosapp/backend/internal/config"
)

// Claims carried by every token.
type Claims struct {
	UserID uuid.UUID `json:"uid"`
	Role   string    `json:"role"`
	Type   string    `json:"typ"` // access | refresh
	jwt.RegisteredClaims
}

// Manager is the token port used by services and middleware.
type Manager interface {
	SignAccess(userID uuid.UUID, role string) (string, error)
	SignRefresh(userID uuid.UUID, role string) (token string, jti uuid.UUID, err error)
	ParseAccess(token string) (*Claims, error)
	ParseRefresh(token string) (*Claims, error)
	RefreshTTL() time.Duration
}

type manager struct {
	cfg config.JWTConfig
}

var _ Manager = (*manager)(nil)

func NewManager(cfg config.JWTConfig) Manager { return &manager{cfg: cfg} }

func (m *manager) SignAccess(userID uuid.UUID, role string) (string, error) {
	return m.sign(userID, role, constants.TokenAccess, m.cfg.AccessSecret, m.cfg.AccessTTL, uuid.New())
}

func (m *manager) SignRefresh(userID uuid.UUID, role string) (string, uuid.UUID, error) {
	jti := uuid.New()
	token, err := m.sign(userID, role, constants.TokenRefresh, m.cfg.RefreshSecret, m.cfg.RefreshTTL, jti)
	return token, jti, err
}

func (m *manager) RefreshTTL() time.Duration { return m.cfg.RefreshTTL }

func (m *manager) sign(userID uuid.UUID, role, typ, secret string, ttl time.Duration, jti uuid.UUID) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID: userID,
		Role:   role,
		Type:   typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.cfg.Issuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        jti.String(),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func (m *manager) ParseAccess(token string) (*Claims, error) {
	return m.parse(token, m.cfg.AccessSecret, constants.TokenAccess)
}

func (m *manager) ParseRefresh(token string) (*Claims, error) {
	return m.parse(token, m.cfg.RefreshSecret, constants.TokenRefresh)
}

func (m *manager) parse(tokenStr, secret, wantType string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return []byte(secret), nil
	}, jwt.WithIssuer(m.cfg.Issuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	if claims.Type != wantType {
		return nil, fmt.Errorf("wrong token type: got %s want %s", claims.Type, wantType)
	}
	return claims, nil
}
