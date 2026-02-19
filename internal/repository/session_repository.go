package repository

import (
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
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
		return fmt.Errorf("failed to revoke sessions: %w", err)
	}
	return nil
}

func (r *SessionRepository) BlacklistToken(ctx context.Context, tokenString string, ttl time.Duration) error {
	key := fmt.Sprintf("blacklist:%s", tokenString)
	return r.redisAdapter.Set(ctx, key, "revoked", ttl)
}

func (r *SessionRepository) IsTokenBlacklisted(ctx context.Context, tokenString string) (bool, error) {
	key := fmt.Sprintf("blacklist:%s", tokenString)
	val, err := r.redisAdapter.Get(ctx, key)
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, err
	}
	return val != "", nil
}

func (r *SessionRepository) IsUserRevoked(ctx context.Context, userID uuid.UUID, tokenIssuedAt int64) (bool, error) {
	key := fmt.Sprintf("revoked_user:%s", userID)
	revokedAtStr, err := r.redisAdapter.Get(ctx, key)

	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, err
	}

	if revokedAtStr == "" {
		return false, nil
	}

	revokedAt, err := strconv.ParseInt(revokedAtStr, 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid revoked timestamp for user %s: %w", userID, err)
	}

	return tokenIssuedAt <= revokedAt, nil
}
