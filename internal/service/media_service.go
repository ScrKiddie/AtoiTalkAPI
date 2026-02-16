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
	"github.com/google/uuid"
)

type MediaService struct {
	client         *ent.Client
	cfg            *config.AppConfig
	validator      *validator.Validate
	storageAdapter *adapter.StorageAdapter
	captchaAdapter *adapter.CaptchaAdapter
}

func NewMediaService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate, storageAdapter *adapter.StorageAdapter, captchaAdapter *adapter.CaptchaAdapter) *MediaService {
	return &MediaService{
		client:         client,
		cfg:            cfg,
		validator:      validator,
		storageAdapter: storageAdapter,
		captchaAdapter: captchaAdapter,
	}
}

func (s *MediaService) UploadMedia(ctx context.Context, userID uuid.UUID, req model.UploadMediaRequest) (*model.MediaDTO, error) {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err)
		return nil, helper.NewBadRequestError("")
	}

	if err := s.captchaAdapter.Verify(req.CaptchaToken, ""); err != nil {
		slog.Warn("Captcha verification failed", "error", err)
		return nil, helper.NewBadRequestError("")
	}

	file, err := req.File.Open()
	if err != nil {
		slog.Error("Failed to open uploaded file", "error", err)
		return nil, helper.NewInternalServerError("")
	}
	defer file.Close()

	contentType, err := helper.DetectFileContentType(file)
	if err != nil {
		slog.Error("Failed to detect file content type", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	finalFileName := helper.GenerateUniqueFileName(req.File.Filename)

	mediaRecord, err := s.client.Media.Create().
		SetFileName(finalFileName).
		SetOriginalName(req.File.Filename).
		SetFileSize(req.File.Size).
		SetMimeType(contentType).
		SetStatus(media.StatusActive).
		SetCategory(media.CategoryMessageAttachment).
		SetUploaderID(userID).
		Save(ctx)

	if err != nil {
		slog.Error("Failed to create media record", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	filePath := finalFileName

	if err := s.storageAdapter.StoreFromReader(file, contentType, filePath, false); err != nil {
		slog.Error("Failed to upload file to storage", "error", err)

		if delErr := s.client.Media.DeleteOneID(mediaRecord.ID).Exec(context.Background()); delErr != nil {
			slog.Error("Failed to delete media record after file upload failure", "error", delErr)
		}

		return nil, helper.NewInternalServerError("")
	}

	mediaURL, err := s.storageAdapter.GetPresignedURL(finalFileName, 15*time.Minute)
	if err != nil {
		slog.Error("Failed to generate presigned URL", "error", err)

		mediaURL = ""
	}

	return &model.MediaDTO{
		ID:           mediaRecord.ID,
		FileName:     mediaRecord.FileName,
		OriginalName: mediaRecord.OriginalName,
		FileSize:     mediaRecord.FileSize,
		MimeType:     mediaRecord.MimeType,
		URL:          mediaURL,
	}, nil
}

func (s *MediaService) GetMediaURL(ctx context.Context, userID, mediaID uuid.UUID) (*model.MediaURLResponse, error) {
	m, err := s.client.Media.Query().
		Where(media.ID(mediaID)).
		Select(media.FieldID, media.FieldFileName).
		WithMessage(func(q *ent.MessageQuery) {
			q.Select(message.FieldID, message.FieldChatID)
			q.WithChat(func(cq *ent.ChatQuery) {
				cq.Select(chat.FieldID, chat.FieldType)
				cq.WithPrivateChat(func(pq *ent.PrivateChatQuery) {
					pq.Select(privatechat.FieldUser1ID, privatechat.FieldUser2ID)
				})
				cq.WithGroupChat(func(gq *ent.GroupChatQuery) {
					gq.Select(groupchat.FieldID)
				})
			})
		}).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("Media not found")
		}
		slog.Error("Failed to query media", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if m.Edges.Message == nil || m.Edges.Message.Edges.Chat == nil {
		return nil, helper.NewForbiddenError("Media is not associated with a chat")
	}

	c := m.Edges.Message.Edges.Chat
	isMember := false
	if c.Type == chat.TypePrivate && c.Edges.PrivateChat != nil {
		if c.Edges.PrivateChat.User1ID == userID || c.Edges.PrivateChat.User2ID == userID {
			isMember = true
		}
	} else if c.Type == chat.TypeGroup && c.Edges.GroupChat != nil {
		exists, err := s.client.GroupMember.Query().
			Where(
				groupmember.GroupChatID(c.Edges.GroupChat.ID),
				groupmember.UserID(userID),
			).Exist(ctx)
		if err == nil && exists {
			isMember = true
		}
	}

	if !isMember {
		return nil, helper.NewForbiddenError("You do not have access to this media")
	}

	url, err := s.storageAdapter.GetPresignedURL(m.FileName, 15*time.Minute)
	if err != nil {
		slog.Error("Failed to generate presigned URL for refresh", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	return &model.MediaURLResponse{
		URL: url,
	}, nil
}
