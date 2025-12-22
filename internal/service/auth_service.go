package service

import (
	"AtoiTalkAPI/ent"
	_ "AtoiTalkAPI/ent/runtime"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"context"
	"log/slog"
	"strings"

	"github.com/go-playground/validator/v10"
	"google.golang.org/api/idtoken"
)

type AuthService interface {
	GoogleExchange(ctx context.Context, req model.GoogleLoginRequest) (*model.AuthResponse, error)
}

type authService struct {
	client    *ent.Client
	cfg       *config.AppConfig
	validator *validator.Validate
}

func NewAuthService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate) AuthService {
	return &authService{
		client:    client,
		cfg:       cfg,
		validator: validator,
	}
}

func (s *authService) GoogleExchange(ctx context.Context, req model.GoogleLoginRequest) (*model.AuthResponse, error) {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err)
		return nil, helper.NewBadRequestError("", err)
	}

	payload, err := idtoken.Validate(ctx, req.Code, s.cfg.GoogleClientID)
	if err != nil {
		slog.Error("Failed to validate google token", "error", err)
		return nil, helper.NewUnauthorizedError("", err)
	}

	email, ok := payload.Claims["email"].(string)
	if !ok {
		slog.Warn("Email not found in token claims")
		return nil, helper.NewBadRequestError("", nil)
	}

	name, ok := payload.Claims["name"].(string)
	if !ok || name == "" {

		name = strings.Split(email, "@")[0]
	}

	picture, ok := payload.Claims["picture"].(string)
	if !ok {
		picture = ""
	}

	u, err := s.client.User.Query().
		Where(user.Email(email)).
		Only(ctx)

	if err != nil && !ent.IsNotFound(err) {
		slog.Error("Failed to query user", "error", err)
		return nil, helper.NewInternalServerError("", err)
	}

	if u == nil {

		u, err = s.client.User.Create().
			SetEmail(email).
			SetFullName(name).
			SetAvatarFileName(picture).
			Save(ctx)

		if err != nil {
			slog.Error("Failed to create user", "error", err)
			return nil, helper.NewInternalServerError("", err)
		}
	}

	token, err := helper.GenerateJWT(s.cfg, u.ID, u.Email)
	if err != nil {
		slog.Error("Failed to generate JWT token", "error", err)
		return nil, helper.NewInternalServerError("", err)
	}

	return &model.AuthResponse{
		Token: token,
		User: model.UserDTO{
			ID:       u.ID,
			Email:    u.Email,
			FullName: u.FullName,
			Avatar:   *u.AvatarFileName,
		},
	}, nil
}
