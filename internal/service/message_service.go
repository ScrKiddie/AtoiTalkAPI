package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupchat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/media"
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
		isUpdated := false

		if pc.User1ID == userID {
			update.SetUser1LastReadAt(time.Now())
			if pc.User1HiddenAt != nil {
				update.ClearUser1HiddenAt()
			}
			isUpdated = true
		} else if pc.User2ID == userID {
			update.SetUser2LastReadAt(time.Now())
			if pc.User2HiddenAt != nil {
				update.ClearUser2HiddenAt()
			}
			isUpdated = true
		}

		if isUpdated {
			if err := update.Exec(ctx); err != nil {

				slog.Error("Failed to update private chat metadata", "error", err)
			}
		}
	} else if !ent.IsNotFound(err) {
		slog.Error("Failed to query private chat for metadata update", "error", err)
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
		CreatedAt:   msg.CreatedAt.String(),
		IsRead:      false,
	}, nil
}
