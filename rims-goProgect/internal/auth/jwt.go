// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims holds JWT token payload with user identity and role.
type Claims struct {
	UserID   uint   `json:"uid"`
	Username string `json:"username"`
	RoleID   uint   `json:"rid"`
	RoleCode string `json:"role"`
	jwt.RegisteredClaims
}

// TokenService handles JWT token generation and parsing.
type TokenService struct {
	secretKey []byte
	expireDur time.Duration
}

// NewTokenService creates a TokenService with the given secret and expiration.
func NewTokenService(secret string, expireHours int) *TokenService {
	return &TokenService{
		secretKey: []byte(secret),
		expireDur: time.Duration(expireHours) * time.Hour,
	}
}

// GenerateToken creates a signed JWT for the given user identity.
func (s *TokenService) GenerateToken(userID uint, username string, roleID uint, roleCode string) (string, int64, error) {
	now := time.Now()
	expiresAt := now.Add(s.expireDur)
	claims := Claims{
		UserID:   userID,
		Username: username,
		RoleID:   roleID,
		RoleCode: roleCode,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secretKey)
	if err != nil {
		return "", 0, fmt.Errorf("sign jwt: %w", err)
	}
	return signed, expiresAt.Unix(), nil
}

// ParseToken validates and extracts claims from a JWT string.
func (s *TokenService) ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
		}
		return s.secretKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse jwt: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	return claims, nil
}
