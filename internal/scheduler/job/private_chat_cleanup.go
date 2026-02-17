package job

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/privatechat"
	"AtoiTalkAPI/internal/config"
	"context"
	"log/slog"
)

func RunPrivateChatCleanup(ctx context.Context, client *ent.Client, cfg *config.AppConfig) error {
	slog.Info("Running Private Chat Cleanup (Garbage Collection)")

	abandonedChats, err := client.PrivateChat.Query().
		Where(
			privatechat.User1IDIsNil(),
			privatechat.User2IDIsNil(),
		).
		WithChat().
		All(ctx)

	if err != nil {
		slog.Error("Failed to query abandoned private chats", "error", err)
		return err
	}

	for _, pc := range abandonedChats {
		chatID := pc.Edges.Chat.ID
		err := client.Chat.DeleteOneID(chatID).Exec(ctx)
		if err != nil {
			slog.Error("Failed to delete abandoned chat", "chatID", chatID, "privateChatID", pc.ID, "error", err)
			continue
		}
		slog.Info("Deleted abandoned private chat", "chatID", chatID)
	}

	return nil
}
