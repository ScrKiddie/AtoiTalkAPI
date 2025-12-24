package service

import (
	"AtoiTalkAPI/ent"
	_ "AtoiTalkAPI/ent/runtime"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/go-playground/validator/v10"
	"google.golang.org/api/idtoken"
)

type AuthService struct {
	client         *ent.Client
	cfg            *config.AppConfig
	validator      *validator.Validate
	storageAdapter *adapter.StorageAdapter
}

func NewAuthService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate, storageAdapter *adapter.StorageAdapter) *AuthService {
	return &AuthService{
		client:         client,
		cfg:            cfg,
		validator:      validator,
		storageAdapter: storageAdapter,
	}
}

func (s *AuthService) GoogleExchange(ctx context.Context, req model.GoogleLoginRequest) (*model.AuthResponse, error) {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err)
		return nil, helper.NewBadRequestError("")
	}

	payload, err := idtoken.Validate(ctx, req.Code, s.cfg.GoogleClientID)
	if err != nil {
		slog.Error("Failed to validate google token", "error", err)
		return nil, helper.NewUnauthorizedError("")
	}

	email, ok := payload.Claims["email"].(string)
	if !ok {
		slog.Warn("Email not found in token claims")
		return nil, helper.NewBadRequestError("")
	}

	name, ok := payload.Claims["name"].(string)
	if !ok || name == "" {
		name = strings.Split(email, "@")[0]
	}

	picture, _ := payload.Claims["picture"].(string)

	u, err := s.client.User.Query().
		Where(user.Email(email)).
		Only(ctx)

	if err != nil && !ent.IsNotFound(err) {
		slog.Error("Failed to query user", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	var avatarFileName string
	if u == nil {
		if picture != "" {
			data, contentType, err := s.storageAdapter.Download(picture)
			if err != nil {
				slog.Error("Failed to download profile picture", "error", err)
			} else {
				fileName := helper.GenerateUniqueFileName(picture)
				filePath := filepath.Join(s.cfg.StorageProfile, fileName)

				err = s.storageAdapter.StoreFromReader(bytes.NewReader(data), contentType, filePath)
				if err != nil {
					slog.Error("Failed to store profile picture", "error", err)
				} else {
					avatarFileName = fileName
				}
			}
		}

		u, err = s.client.User.Create().
			SetEmail(email).
			SetFullName(name).
			SetNillableAvatarFileName(&avatarFileName).
			Save(ctx)

		if err != nil {
			slog.Error("Failed to create user", "error", err)
			return nil, helper.NewInternalServerError("")
		}
	}

	token, err := helper.GenerateJWT(s.cfg.JWTSecret, s.cfg.JWTExp, u.ID)
	if err != nil {
		slog.Error("Failed to generate JWT token", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	avatarURL := ""
	if u.AvatarFileName != nil && *u.AvatarFileName != "" {
		avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, *u.AvatarFileName)
	}

	return &model.AuthResponse{
		Token: token,
		User: model.UserDTO{
			ID:       u.ID,
			Email:    u.Email,
			FullName: u.FullName,
			Avatar:   avatarURL,
		},
	}, nil
}
