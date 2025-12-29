package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupchat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/media"
	"AtoiTalkAPI/ent/message"
	"AtoiTalkAPI/ent/privatechat"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"context"
	"encoding/base64"
	"log/slog"
	"strconv"
	"time"

	"github.com/go-playground/validator/v10"
)

type MessageService struct {
	client         *ent.Client
	cfg            *config.AppConfig
	validator      *validator.Validate
	storageAdapter *adapter.StorageAdapter
}

func NewMessageService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate, storageAdapter *adapter.StorageAdapter) *MessageService {
	return &MessageService{
		client:         client,
		cfg:            cfg,
		validator:      validator,
		storageAdapter: storageAdapter,
	}
}

func (s *MessageService) SendMessage(ctx context.Context, userID int, req model.SendMessageRequest) (*model.MessageResponse, error) {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err, "userID", userID)
		return nil, helper.NewBadRequestError("")
	}

	var msg *ent.Message

	tx, err := s.client.Tx(ctx)
	if err != nil {
		slog.Error("Failed to start transaction", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	defer func() {
		_ = tx.Rollback()
		if v := recover(); v != nil {
			panic(v)
		}
	}()

	exists, err := tx.Chat.Query().
		Where(
			chat.ID(req.ChatID),
			chat.Or(
				chat.HasPrivateChatWith(
					privatechat.Or(
						privatechat.User1ID(userID),
						privatechat.User2ID(userID),
					),
				),
				chat.HasGroupChatWith(
					groupchat.HasMembersWith(
						groupmember.UserID(userID),
					),
				),
			),
		).
		Exist(ctx)

	if err != nil {
		slog.Error("Failed to check chat membership", "error", err, "chatID", req.ChatID)
		return nil, helper.NewInternalServerError("")
	}

	if !exists {

		chatExists, _ := tx.Chat.Query().Where(chat.ID(req.ChatID)).Exist(ctx)

		if !chatExists {
			return nil, helper.NewNotFoundError("")
		}
		return nil, helper.NewForbiddenError("")
	}

	if req.ReplyToID != nil {
		replyMsgExists, err := tx.Message.Query().
			Where(
				message.ID(*req.ReplyToID),
				message.ChatID(req.ChatID),
				message.DeletedAtIsNil(),
			).
			Exist(ctx)

		if err != nil {
			slog.Error("Failed to check reply message existence", "error", err)
			return nil, helper.NewInternalServerError("")
		}
		if !replyMsgExists {
			return nil, helper.NewBadRequestError("")
		}
	}

	if len(req.AttachmentIDs) > 0 {

		count, err := tx.Media.Query().
			Where(
				media.IDIn(req.AttachmentIDs...),
				media.MessageIDIsNil(),
				media.Not(media.HasUserAvatar()),
				media.Not(media.HasGroupAvatar()),
				media.StatusEQ(media.StatusActive),
			).
			Count(ctx)

		if err != nil {
			slog.Error("Failed to count valid media", "error", err)
			return nil, helper.NewInternalServerError("")
		}

		if count != len(req.AttachmentIDs) {
			return nil, helper.NewBadRequestError("")
		}
	}

	msgCreate := tx.Message.Create().
		SetChatID(req.ChatID).
		SetSenderID(userID).
		SetContent(req.Content)

	if req.ReplyToID != nil {
		msgCreate.SetReplyToID(*req.ReplyToID)
	}

	if len(req.AttachmentIDs) > 0 {
		msgCreate.AddAttachmentIDs(req.AttachmentIDs...)
	}

	msg, err = msgCreate.Save(ctx)
	if err != nil {
		slog.Error("Failed to save message", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	pc, err := tx.PrivateChat.Query().
		Where(privatechat.ChatID(req.ChatID)).
		Only(ctx)

	if err == nil && pc != nil {
		update := tx.PrivateChat.UpdateOneID(pc.ID)

		if pc.User1ID == userID {
			update.SetUser1LastReadAt(time.Now())
			update.SetUser1UnreadCount(0)
			update.AddUser2UnreadCount(1)

		} else {
			update.SetUser2LastReadAt(time.Now())
			update.SetUser2UnreadCount(0)
			update.AddUser1UnreadCount(1)
		}

		if err := update.Exec(ctx); err != nil {
			slog.Error("Failed to update private chat counters", "error", err)
		}
	} else if !ent.IsNotFound(err) {
		slog.Error("Failed to query private chat for metadata update", "error", err)
	}

	gc, err := tx.GroupChat.Query().Where(groupchat.ChatID(req.ChatID)).Only(ctx)
	if err == nil && gc != nil {

		err := tx.GroupMember.Update().
			Where(
				groupmember.GroupChatID(gc.ID),
				groupmember.UserIDNEQ(userID),
			).
			AddUnreadCount(1).
			Exec(ctx)

		if err != nil {
			slog.Error("Failed to update group member counters", "error", err)
		}

		err = tx.GroupMember.Update().
			Where(
				groupmember.GroupChatID(gc.ID),
				groupmember.UserID(userID),
			).
			SetUnreadCount(0).
			SetLastReadAt(time.Now()).
			Exec(ctx)

		if err != nil {
			slog.Error("Failed to reset sender group member counter", "error", err)
		}
	}

	err = tx.Chat.UpdateOneID(req.ChatID).
		SetUpdatedAt(time.Now()).
		Exec(ctx)
	if err != nil {
		slog.Error("Failed to update chat timestamp", "error", err)
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	fullMsg, err := s.client.Message.Query().
		Where(message.ID(msg.ID)).
		WithSender().
		WithAttachments().
		WithReplyTo(func(q *ent.MessageQuery) {
			q.WithSender().WithAttachments()
		}).
		Only(ctx)

	if err != nil {
		slog.Error("Failed to fetch full message for response", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	return helper.ToMessageResponse(fullMsg, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment), nil
}

func (s *MessageService) GetMessages(ctx context.Context, userID int, req model.GetMessagesRequest) ([]model.MessageResponse, string, bool, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, "", false, helper.NewBadRequestError("")
	}

	if req.Limit == 0 {
		req.Limit = 20
	}

	chatInfo, err := s.client.Chat.Query().
		Where(chat.ID(req.ChatID)).
		WithPrivateChat().
		WithGroupChat(func(q *ent.GroupChatQuery) {
			q.WithMembers(func(mq *ent.GroupMemberQuery) {
				mq.Where(groupmember.UserID(userID))
			})
		}).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, "", false, helper.NewNotFoundError("")
		}
		slog.Error("Failed to query chat info", "error", err, "chatID", req.ChatID)
		return nil, "", false, helper.NewInternalServerError("")
	}

	isMember := false
	var hiddenAt *time.Time

	if chatInfo.Type == chat.TypePrivate && chatInfo.Edges.PrivateChat != nil {
		pc := chatInfo.Edges.PrivateChat
		if pc.User1ID == userID {
			isMember = true
			hiddenAt = pc.User1HiddenAt
		} else if pc.User2ID == userID {
			isMember = true
			hiddenAt = pc.User2HiddenAt
		}
	} else if chatInfo.Type == chat.TypeGroup && chatInfo.Edges.GroupChat != nil {
		if len(chatInfo.Edges.GroupChat.Edges.Members) > 0 {
			isMember = true
		}
	}

	if !isMember {
		return nil, "", false, helper.NewForbiddenError("")
	}

	query := s.client.Message.Query().
		Where(
			message.ChatID(req.ChatID),
		)

	if hiddenAt != nil {
		query = query.Where(message.CreatedAtGT(*hiddenAt))
	}

	query = query.Order(ent.Desc(message.FieldID)).
		Limit(req.Limit + 1).
		WithSender().
		WithAttachments().
		WithReplyTo(func(q *ent.MessageQuery) {
			q.WithSender().WithAttachments()
		})

	if req.Cursor > 0 {
		query = query.Where(message.IDLT(req.Cursor))
	}

	messages, err := query.All(ctx)
	if err != nil {
		slog.Error("Failed to get messages", "error", err)
		return nil, "", false, helper.NewInternalServerError("")
	}

	hasNext := false
	var nextCursor string
	if len(messages) > req.Limit {
		hasNext = true
		messages = messages[:req.Limit]
		lastID := messages[len(messages)-1].ID
		nextCursor = base64.URLEncoding.EncodeToString([]byte(strconv.Itoa(lastID)))
	}

	var response []model.MessageResponse
	for _, msg := range messages {
		resp := helper.ToMessageResponse(msg, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment)
		if resp != nil {
			response = append(response, *resp)
		}
	}

	for i, j := 0, len(response)-1; i < j; i, j = i+1, j-1 {
		response[i], response[j] = response[j], response[i]
	}

	return response, nextCursor, hasNext, nil
}

func (s *MessageService) DeleteMessage(ctx context.Context, userID int, messageID int) error {
	msg, err := s.client.Message.Query().
		Where(message.ID(messageID)).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("")
		}
		slog.Error("Failed to query message", "error", err, "messageID", messageID)
		return helper.NewInternalServerError("")
	}

	if msg.SenderID != userID {
		return helper.NewForbiddenError("")
	}

	if msg.DeletedAt != nil {
		return helper.NewBadRequestError("")
	}

	err = s.client.Message.UpdateOne(msg).
		SetDeletedAt(time.Now()).
		Exec(ctx)

	if err != nil {
		slog.Error("Failed to delete message", "error", err, "messageID", messageID)
		return helper.NewInternalServerError("")
	}

	return nil
}
