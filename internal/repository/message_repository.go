package repository

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/message"
	"AtoiTalkAPI/ent/user"
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
)

var ErrMessageNotFound = errors.New("message not found")

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
		WithSender(func(q *ent.UserQuery) {
			q.Select(user.FieldID, user.FieldUsername, user.FieldFullName, user.FieldAvatarID)
			q.WithAvatar()
		}).
		WithAttachments().
		WithReplyTo(func(q *ent.MessageQuery) {
			q.WithSender(func(uq *ent.UserQuery) {
				uq.Select(user.FieldID, user.FieldUsername, user.FieldFullName, user.FieldAvatarID)
				uq.WithAvatar()
			})
			q.WithAttachments(func(aq *ent.MediaQuery) {
				aq.Limit(1)
			})
		})

	return query.All(ctx)
}

func (r *MessageRepository) GetMessagesAround(ctx context.Context, chatID uuid.UUID, hiddenAt *time.Time, aroundID uuid.UUID, limit int) ([]*ent.Message, error) {

	targetMsg, err := r.client.Message.Query().
		Where(
			message.ID(aroundID),
			message.ChatID(chatID),
			message.HasChatWith(chat.DeletedAtIsNil()),
		).
		WithSender(func(q *ent.UserQuery) {
			q.Select(user.FieldID, user.FieldUsername, user.FieldFullName, user.FieldAvatarID)
			q.WithAvatar()
		}).
		WithAttachments().
		WithReplyTo(func(q *ent.MessageQuery) {
			q.WithSender(func(uq *ent.UserQuery) {
				uq.Select(user.FieldID, user.FieldUsername, user.FieldFullName, user.FieldAvatarID)
				uq.WithAvatar()
			})
			q.WithAttachments(func(aq *ent.MediaQuery) {
				aq.Limit(1)
			})
		}).
		Only(ctx)

	if err != nil {
		return nil, err
	}

	if hiddenAt != nil && !targetMsg.CreatedAt.After(*hiddenAt) {
		return nil, ErrMessageNotFound
	}

	halfLimit := limit / 2

	prevQuery := r.client.Message.Query().
		Where(
			message.ChatID(chatID),
			message.IDLT(aroundID),
		)
	if hiddenAt != nil {
		prevQuery = prevQuery.Where(message.CreatedAtGT(*hiddenAt))
	}
	prevMsgs, err := prevQuery.
		Order(ent.Desc(message.FieldID)).
		Limit(halfLimit).
		WithSender(func(q *ent.UserQuery) {
			q.Select(user.FieldID, user.FieldUsername, user.FieldFullName, user.FieldAvatarID)
			q.WithAvatar()
		}).
		WithAttachments().
		WithReplyTo(func(q *ent.MessageQuery) {
			q.WithSender(func(uq *ent.UserQuery) {
				uq.Select(user.FieldID, user.FieldUsername, user.FieldFullName, user.FieldAvatarID)
				uq.WithAvatar()
			})
			q.WithAttachments(func(aq *ent.MediaQuery) {
				aq.Limit(1)
			})
		}).
		All(ctx)
	if err != nil {
		return nil, err
	}

	nextQuery := r.client.Message.Query().
		Where(
			message.ChatID(chatID),
			message.IDGT(aroundID),
		)
	if hiddenAt != nil {
		nextQuery = nextQuery.Where(message.CreatedAtGT(*hiddenAt))
	}
	nextMsgs, err := nextQuery.
		Order(ent.Asc(message.FieldID)).
		Limit(halfLimit).
		WithSender(func(q *ent.UserQuery) {
			q.Select(user.FieldID, user.FieldUsername, user.FieldFullName, user.FieldAvatarID)
			q.WithAvatar()
		}).
		WithAttachments().
		WithReplyTo(func(q *ent.MessageQuery) {
			q.WithSender(func(uq *ent.UserQuery) {
				uq.Select(user.FieldID, user.FieldUsername, user.FieldFullName, user.FieldAvatarID)
				uq.WithAvatar()
			})
			q.WithAttachments(func(aq *ent.MediaQuery) {
				aq.Limit(1)
			})
		}).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*ent.Message, 0, len(prevMsgs)+1+len(nextMsgs))

	for i := len(prevMsgs) - 1; i >= 0; i-- {
		result = append(result, prevMsgs[i])
	}

	result = append(result, targetMsg)
	result = append(result, nextMsgs...)

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID.String() < result[j].ID.String()
	})

	return result, nil
}
