package repository

import (
	"AtoiTalkAPI/internal/adapter"
	"context"
	"time"
)

type RateLimitRepository struct {
	redisAdapter *adapter.RedisAdapter
}

func NewRateLimitRepository(redisAdapter *adapter.RedisAdapter) *RateLimitRepository {
	return &RateLimitRepository{
		redisAdapter: redisAdapter,
	}
}

func (r *RateLimitRepository) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	client := r.redisAdapter.Client()
	pipe := client.Pipeline()
	incr := pipe.Incr(ctx, key)
	ttlCmd := pipe.TTL(ctx, key)
	_, err := pipe.Exec(ctx)

	if err != nil {
		return false, 0, err
	}

	count := incr.Val()
	ttl := ttlCmd.Val()

	if count == 1 || ttl == -1 {
		client.Expire(ctx, key, window)
		ttl = window
	}

	if count > int64(limit) {
		return false, ttl, nil
	}

	return true, ttl, nil
}
