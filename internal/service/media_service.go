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

func (s *MediaService) UploadMedia(ctx context.Context, req model.UploadMediaRequest) (*model.MediaDTO, error) {
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

	contentType := req.File.Header.Get("Content-Type")
	finalFileName := helper.GenerateUniqueFileName(req.File.Filename)

	var mediaRecord *ent.Media
	var mediaURL string

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

	pendingMedia, err := tx.Media.Create().
		SetFileName(finalFileName).
		SetOriginalName(req.File.Filename).
		SetFileSize(req.File.Size).
		SetMimeType(contentType).
		SetStatus(media.StatusPending).
		Save(ctx)

	if err != nil {

		slog.Error("Failed to create pending media record", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	filePath := filepath.Join(s.cfg.StorageAttachment, finalFileName)
	if err := s.storageAdapter.StoreFromReader(file, contentType, filePath); err != nil {
		slog.Error("Failed to upload file to storage", "error", err)

		return nil, helper.NewInternalServerError("")
	}

	mediaRecord, err = tx.Media.UpdateOneID(pendingMedia.ID).
		SetStatus(media.StatusActive).
		Save(ctx)

	if err != nil {

		_ = s.storageAdapter.Delete(filePath)

		slog.Error("Failed to activate media record", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	mediaURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment, finalFileName)

	return &model.MediaDTO{
		ID:           mediaRecord.ID,
		FileName:     mediaRecord.FileName,
		OriginalName: mediaRecord.OriginalName,
		FileSize:     mediaRecord.FileSize,
		MimeType:     mediaRecord.MimeType,
		URL:          mediaURL,
	}, nil
}
