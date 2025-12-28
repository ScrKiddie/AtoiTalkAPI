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
	"log/slog"
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
	var attachmentDTOs []model.MediaDTO
	var replyPreview *model.ReplyPreviewDTO

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
		replyMsg, err := tx.Message.Query().
			Where(
				message.ID(*req.ReplyToID),
				message.ChatID(req.ChatID),
				message.DeletedAtIsNil(),
			).
			WithSender().
			Only(ctx)

		if err != nil {
			if ent.IsNotFound(err) {
				return nil, helper.NewBadRequestError("Replied message not found or in a different chat")
			}
			slog.Error("Failed to check reply message existence", "error", err)
			return nil, helper.NewInternalServerError("")
		}

		content := ""
		if replyMsg.Content != nil {
			content = *replyMsg.Content
		}
		replyPreview = &model.ReplyPreviewDTO{
			ID:         replyMsg.ID,
			SenderName: replyMsg.Edges.Sender.FullName,
			Content:    content,
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
			if pc.User1HiddenAt != nil {
				update.ClearUser1HiddenAt()
			}
		} else {
			update.SetUser2LastReadAt(time.Now())
			update.SetUser2UnreadCount(0)
			update.AddUser1UnreadCount(1)
			if pc.User2HiddenAt != nil {
				update.ClearUser2HiddenAt()
			}
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

	if len(req.AttachmentIDs) > 0 {
		attachments, err := msg.QueryAttachments().All(ctx)
		if err == nil {
			for _, att := range attachments {
				url := helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment, att.FileName)
				attachmentDTOs = append(attachmentDTOs, model.MediaDTO{
					ID:           att.ID,
					FileName:     att.FileName,
					OriginalName: att.OriginalName,
					FileSize:     att.FileSize,
					MimeType:     att.MimeType,
					URL:          url,
				})
			}
		} else {
			slog.Error("Failed to fetch attachments for response", "error", err)
			return nil, helper.NewInternalServerError("")
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	content := ""
	if msg.Content != nil {
		content = *msg.Content
	}

	return &model.MessageResponse{
		ID:          msg.ID,
		ChatID:      msg.ChatID,
		SenderID:    msg.SenderID,
		Content:     content,
		Attachments: attachmentDTOs,
		ReplyTo:     replyPreview,
		CreatedAt:   msg.CreatedAt.String(),
	}, nil
}
