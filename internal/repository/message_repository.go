package repository

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/message"
	"context"
	"time"

	"github.com/google/uuid"
)

type MessageRepository struct {
	client *ent.Client
}

func NewMessageRepository(client *ent.Client) *MessageRepository {
	return &MessageRepository{
		client: client,
	}
}

func (r *MessageRepository) GetMessages(ctx context.Context, chatID uuid.UUID, hiddenAt *time.Time, cursor uuid.UUID, limit int, direction string) ([]*ent.Message, error) {
	query := r.client.Message.Query().
		Where(
			message.ChatID(chatID),
			message.HasChatWith(chat.DeletedAtIsNil()),
		)

	if hiddenAt != nil {
		query = query.Where(message.CreatedAtGT(*hiddenAt))
	}

	if direction == "newer" {
		query = query.Order(ent.Asc(message.FieldID))
		if cursor != uuid.Nil {
			query = query.Where(message.IDGT(cursor))
		}
	} else {
		query = query.Order(ent.Desc(message.FieldID))
		if cursor != uuid.Nil {
			query = query.Where(message.IDLT(cursor))
		}
	}

	query = query.Limit(limit + 1).
		WithSender().
		WithAttachments().
		WithReplyTo(func(q *ent.MessageQuery) {
			q.WithSender()
			q.WithAttachments(func(aq *ent.MediaQuery) {
				aq.Limit(1)
			})
		})

	return query.All(ctx)
}
