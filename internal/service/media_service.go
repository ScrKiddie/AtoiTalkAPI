package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/media"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"context"
	"log/slog"
	"path/filepath"

	"github.com/go-playground/validator/v10"
)

type MediaService struct {
	client         *ent.Client
	cfg            *config.AppConfig
	validator      *validator.Validate
	storageAdapter *adapter.StorageAdapter
}

func NewMediaService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate, storageAdapter *adapter.StorageAdapter) *MediaService {
	return &MediaService{
		client:         client,
		cfg:            cfg,
		validator:      validator,
		storageAdapter: storageAdapter,
	}
}

func (s *MediaService) UploadMedia(ctx context.Context, userID int, req model.UploadMediaRequest) (*model.MediaDTO, error) {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err)
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
		SetUploaderID(userID).
		Save(ctx)

	if err != nil {
		slog.Error("Failed to create media record", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	filePath := filepath.Join(s.cfg.StorageAttachment, finalFileName)
	if err := s.storageAdapter.StoreFromReader(file, contentType, filePath); err != nil {
		slog.Error("Failed to upload file to storage", "error", err)

		if delErr := s.client.Media.DeleteOneID(mediaRecord.ID).Exec(context.Background()); delErr != nil {
			slog.Error("Failed to delete media record after file upload failure", "error", delErr)
		}

		return nil, helper.NewInternalServerError("")
	}

	mediaURL := helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment, finalFileName)

	return &model.MediaDTO{
		ID:           mediaRecord.ID,
		FileName:     mediaRecord.FileName,
		OriginalName: mediaRecord.OriginalName,
		FileSize:     mediaRecord.FileSize,
		MimeType:     mediaRecord.MimeType,
		URL:          mediaURL,
	}, nil
}
