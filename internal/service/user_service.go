package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"context"
	"log/slog"
	"path/filepath"

	"github.com/go-playground/validator/v10"
)

type UserService struct {
	client         *ent.Client
	cfg            *config.AppConfig
	validator      *validator.Validate
	storageAdapter *adapter.StorageAdapter
}

func NewUserService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate, storageAdapter *adapter.StorageAdapter) *UserService {
	return &UserService{
		client:         client,
		cfg:            cfg,
		validator:      validator,
		storageAdapter: storageAdapter,
	}
}

func (s *UserService) GetCurrentUser(ctx context.Context, userID int) (*model.UserDTO, error) {
	u, err := s.client.User.Query().
		Where(user.ID(userID)).
		WithAvatar().
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("")
		}
		slog.Error("Failed to query user", "error", err, "userID", userID)
		return nil, helper.NewInternalServerError("")
	}

	avatarURL := ""
	if u.Edges.Avatar != nil {
		avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, u.Edges.Avatar.FileName)
	}

	bio := ""
	if u.Bio != nil {
		bio = *u.Bio
	}

	return &model.UserDTO{
		ID:          u.ID,
		Email:       u.Email,
		FullName:    u.FullName,
		Avatar:      avatarURL,
		Bio:         bio,
		HasPassword: u.PasswordHash != nil,
	}, nil
}

func (s *UserService) UpdateProfile(ctx context.Context, userID int, req model.UpdateProfileRequest) (*model.UserDTO, error) {
	if req.DeleteAvatar && req.Avatar != nil {
		return nil, helper.NewBadRequestError("")
	}

	if err := s.validator.Struct(&req); err != nil {
		slog.Warn("Validation failed", "error", err, "userID", userID)
		return nil, helper.NewBadRequestError("")
	}

	u, err := s.client.User.Query().Where(user.ID(userID)).WithAvatar().Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("")
		}

		slog.Error("Failed to query user", "error", err, "userID", userID)
		return nil, helper.NewInternalServerError("")
	}

	update := s.client.User.UpdateOneID(userID).SetFullName(req.FullName)
	if req.Bio != "" {
		update.SetBio(req.Bio)
	} else {
		update.ClearBio()
	}

	var newAvatarFileName string
	var isAvatarUpdated bool

	if req.DeleteAvatar {
		update.ClearAvatar().ClearAvatarID()
		isAvatarUpdated = true
	} else if req.Avatar != nil {
		file, err := req.Avatar.Open()
		if err != nil {
			slog.Error("Failed to open avatar file", "error", err, "userID", userID)
			return nil, helper.NewInternalServerError("")
		}
		defer file.Close()

		fileName := helper.GenerateUniqueFileName(req.Avatar.Filename)
		filePath := filepath.Join(s.cfg.StorageProfile, fileName)
		contentType := req.Avatar.Header.Get("Content-Type")

		if err := s.storageAdapter.StoreFromReader(file, contentType, filePath); err != nil {

			slog.Error("Failed to store avatar to storage", "error", err, "userID", userID)
			return nil, helper.NewInternalServerError("")
		}

		media, err := s.client.Media.Create().
			SetFileName(fileName).SetOriginalName(req.Avatar.Filename).
			SetFileSize(req.Avatar.Size).SetMimeType(contentType).
			Save(ctx)

		if err != nil {

			slog.Error("Failed to create media record", "error", err, "userID", userID)
			return nil, helper.NewInternalServerError("")
		}

		update.SetAvatar(media)
		newAvatarFileName = fileName
		isAvatarUpdated = true
	}

	updatedUser, err := update.Save(ctx)
	if err != nil {

		slog.Error("Failed to save user profile update", "error", err, "userID", userID)
		return nil, helper.NewInternalServerError("")
	}

	var avatarFileName string
	if isAvatarUpdated {
		avatarFileName = newAvatarFileName
	} else if u.Edges.Avatar != nil {
		avatarFileName = u.Edges.Avatar.FileName
	}

	avatarURL := ""
	if avatarFileName != "" {
		avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, avatarFileName)
	}

	bio := ""
	if updatedUser.Bio != nil {
		bio = *updatedUser.Bio
	}

	return &model.UserDTO{
		ID:          updatedUser.ID,
		Email:       updatedUser.Email,
		FullName:    updatedUser.FullName,
		Avatar:      avatarURL,
		Bio:         bio,
		HasPassword: updatedUser.PasswordHash != nil,
	}, nil
}
