package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrInvalidToken = errors.New("invalid token")

// Service — выдача и проверка access JWT (HS256).
type Service struct {
	secret    []byte
	accessTTL time.Duration
}

func NewService(secret []byte, accessTTL time.Duration) *Service {
	return &Service{secret: secret, accessTTL: accessTTL}
}

type accessClaims struct {
	jwt.RegisteredClaims
	// sid — device session ID; позволяет идентифицировать текущее устройство в списке сессий.
	SessionID string `json:"sid"`
}

// IssueAccess возвращает JWT и срок жизни в секундах.
// sub = userID, sid = deviceSessionID.
func (s *Service) IssueAccess(userID, deviceSessionID uuid.UUID) (token string, expiresInSec int64, err error) {
	if userID == uuid.Nil {
		return "", 0, fmt.Errorf("jwt: empty user id")
	}
	now := time.Now()
	exp := now.Add(s.accessTTL)
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		SessionID: deviceSessionID.String(),
	})
	raw, err := t.SignedString(s.secret)
	if err != nil {
		return "", 0, err
	}
	return raw, int64(s.accessTTL.Seconds()), nil
}

// ParseAccessToken извлекает userID и deviceSessionID из Bearer-токена.
func (s *Service) ParseAccessToken(token string) (userID, deviceSessionID uuid.UUID, err error) {
	parsed, err := jwt.ParseWithClaims(token, &accessClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil || !parsed.Valid {
		return uuid.Nil, uuid.Nil, ErrInvalidToken
	}
	claims, ok := parsed.Claims.(*accessClaims)
	if !ok {
		return uuid.Nil, uuid.Nil, ErrInvalidToken
	}
	uid, err := uuid.Parse(claims.Subject)
	if err != nil || uid == uuid.Nil {
		return uuid.Nil, uuid.Nil, ErrInvalidToken
	}
	// sid может отсутствовать в старых токенах — не фатально.
	sid, _ := uuid.Parse(claims.SessionID)
	return uid, sid, nil
}
