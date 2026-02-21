package helper

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type SessionRevokeSnapshot struct {
	Exists bool
	Value  string
	TTL    time.Duration
}

type SessionRevoker interface {
	SnapshotUserRevoke(ctx context.Context, userID uuid.UUID) (SessionRevokeSnapshot, error)
	RevokeAllSessionsAt(ctx context.Context, userID uuid.UUID, revokedAt int64) error
	RollbackUserRevoke(ctx context.Context, userID uuid.UUID, expectedValue string, snapshot SessionRevokeSnapshot) error
}

func RevokeSessionsForTransaction(ctx context.Context, sessionRepo SessionRevoker, userID uuid.UUID) (string, SessionRevokeSnapshot, error) {
	snapshot, err := sessionRepo.SnapshotUserRevoke(ctx, userID)
	if err != nil {
		return "", snapshot, err
	}

	revokedAt := time.Now().Unix()
	if err := sessionRepo.RevokeAllSessionsAt(ctx, userID, revokedAt); err != nil {
		return "", snapshot, err
	}

	return strconv.FormatInt(revokedAt, 10), snapshot, nil
}

func RollbackSessionRevokeIfNeeded(sessionRepo SessionRevoker, userID uuid.UUID, expectedValue string, snapshot SessionRevokeSnapshot) {
	if expectedValue == "" {
		return
	}

	if err := sessionRepo.RollbackUserRevoke(context.Background(), userID, expectedValue, snapshot); err != nil {
		slog.Error("Failed to rollback session revoke after DB transaction failure", "error", err, "userID", userID)
	}
}
