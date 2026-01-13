package repository

import (
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type SessionRepository struct {
	redisAdapter *adapter.RedisAdapter
	cfg          *config.AppConfig
}

func NewSessionRepository(redisAdapter *adapter.RedisAdapter, cfg *config.AppConfig) *SessionRepository {
	return &SessionRepository{
		redisAdapter: redisAdapter,
		cfg:          cfg,
	}
}

func (r *SessionRepository) RevokeAllSessions(ctx context.Context, userID uuid.UUID) error {
	ttl := time.Duration(r.cfg.JWTExp) * time.Second
	key := fmt.Sprintf("revoked_user:%s", userID)

	err := r.redisAdapter.Set(ctx, key, time.Now().Unix(), ttl)
	if err != nil {
		slog.Error("Failed to revoke all sessions in repository", "error", err, "userID", userID)
		return helper.NewInternalServerError("Failed to revoke sessions")
	}
	return nil
}

func (r *SessionRepository) BlacklistToken(ctx context.Context, tokenString string, ttl time.Duration) error {
	key := fmt.Sprintf("blacklist:%s", tokenString)
	return r.redisAdapter.Set(ctx, key, "revoked", ttl)
}

func (r *SessionRepository) IsTokenBlacklisted(ctx context.Context, tokenString string) bool {
	key := fmt.Sprintf("blacklist:%s", tokenString)
	val, err := r.redisAdapter.Get(ctx, key)
	return err == nil && val != ""
}

func (r *SessionRepository) IsUserRevoked(ctx context.Context, userID uuid.UUID, tokenIssuedAt int64) bool {
	key := fmt.Sprintf("revoked_user:%s", userID)
	revokedAtStr, err := r.redisAdapter.Get(ctx, key)

	if err != nil || revokedAtStr == "" {
		return false
	}

	revokedAt, _ := strconv.ParseInt(revokedAtStr, 10, 64)

	return tokenIssuedAt <= revokedAt
}
