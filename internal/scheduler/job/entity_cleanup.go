package job

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/internal/config"
	"context"
	"log/slog"
	"time"
)

func RunEntityCleanup(ctx context.Context, client *ent.Client, cfg *config.AppConfig) error {
	retentionDays := cfg.SoftDeleteRetentionDays
	if retentionDays < 0 {
		retentionDays = 30
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)

	slog.Info("Running Entity Cleanup", "cutoff", cutoff)

	usersToDelete, err := client.User.Query().
		Where(user.DeletedAtLT(cutoff)).
		Select(user.FieldID).
		All(ctx)
	if err != nil {
		slog.Error("Failed to query users for cleanup", "error", err)
		return err
	}

	for _, u := range usersToDelete {
		err := client.User.DeleteOneID(u.ID).Exec(ctx)
		if err != nil {
			slog.Error("Failed to delete user", "userID", u.ID, "error", err)
			continue
		}
		slog.Info("Hard deleted user", "userID", u.ID)
	}

	chatsToDelete, err := client.Chat.Query().
		Where(chat.DeletedAtLT(cutoff)).
		Select(chat.FieldID).
		All(ctx)
	if err != nil {
		slog.Error("Failed to query chats for cleanup", "error", err)
		return err
	}

	for _, c := range chatsToDelete {
		err := client.Chat.DeleteOneID(c.ID).Exec(ctx)
		if err != nil {
			slog.Error("Failed to delete chat", "chatID", c.ID, "error", err)
			continue
		}
		slog.Info("Hard deleted chat", "chatID", c.ID)
	}

	return nil
}
