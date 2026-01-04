package repository

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/message"
	"context"
	"time"
)

type MessageRepository struct {
	client *ent.Client
}

func NewMessageRepository(client *ent.Client) *MessageRepository {
	return &MessageRepository{
		client: client,
	}
}

func (r *MessageRepository) GetMessages(ctx context.Context, chatID int, hiddenAt *time.Time, cursor int, limit int) ([]*ent.Message, error) {
	query := r.client.Message.Query().
		Where(
			message.ChatID(chatID),
		)

	if hiddenAt != nil {
		query = query.Where(message.CreatedAtGT(*hiddenAt))
	}

	query = query.Order(ent.Desc(message.FieldID)).
		Limit(limit + 1).
		WithSender().
		WithAttachments().
		WithReplyTo(func(q *ent.MessageQuery) {
			q.WithSender()
			q.WithAttachments(func(aq *ent.MediaQuery) {
				aq.Limit(1)
			})
		})

	if cursor > 0 {
		query = query.Where(message.IDLT(cursor))
	}

	return query.All(ctx)
}
