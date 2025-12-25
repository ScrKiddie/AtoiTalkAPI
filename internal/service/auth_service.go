package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/otp"
	_ "AtoiTalkAPI/ent/runtime"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/constant"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"google.golang.org/api/idtoken"
)

type AuthService struct {
	client         *ent.Client
	cfg            *config.AppConfig
	validator      *validator.Validate
	storageAdapter *adapter.StorageAdapter
	captchaAdapter *adapter.CaptchaAdapter
}

func NewAuthService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate, storageAdapter *adapter.StorageAdapter, captchaAdapter *adapter.CaptchaAdapter) *AuthService {
	return &AuthService{
		client:         client,
		cfg:            cfg,
		validator:      validator,
		storageAdapter: storageAdapter,
		captchaAdapter: captchaAdapter,
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
	email = helper.NormalizeEmail(email)

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

func (s *AuthService) Register(ctx context.Context, req model.RegisterUserRequest) (*model.AuthResponse, error) {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err)
		return nil, helper.NewBadRequestError("")
	}

	if err := s.captchaAdapter.Verify(req.CaptchaToken, ""); err != nil {
		slog.Warn("Captcha verification failed", "error", err)
		return nil, helper.NewBadRequestError("")
	}

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

	hashedCode := helper.HashOTP(req.Code, s.cfg.OTPSecret)

	otpRecord, err := tx.OTP.Query().
		Where(
			otp.CodeEQ(hashedCode),
			otp.ModeEQ(constant.OTPModeRegister),
			otp.ExpiresAtGT(time.Now()),
		).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewBadRequestError("Invalid or expired OTP")
		}
		slog.Error("Failed to query OTP", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	email := otpRecord.Email

	err = tx.OTP.DeleteOne(otpRecord).Exec(ctx)
	if err != nil {
		slog.Error("Failed to delete OTP", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	exists, err := tx.User.Query().
		Where(user.Email(email)).
		Exist(ctx)
	if err != nil {
		slog.Error("Failed to check user existence", "error", err)
		return nil, helper.NewInternalServerError("")
	}
	if exists {
		return nil, helper.NewConflictError("Email already registered")
	}

	hashedPassword, err := helper.HashPassword(req.Password)
	if err != nil {
		slog.Error("Failed to hash password", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	newUser, err := tx.User.Create().
		SetEmail(email).
		SetFullName(req.FullName).
		SetPasswordHash(hashedPassword).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to create user", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	token, err := helper.GenerateJWT(s.cfg.JWTSecret, s.cfg.JWTExp, newUser.ID)
	if err != nil {
		slog.Error("Failed to generate JWT token", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	return &model.AuthResponse{
		Token: token,
		User: model.UserDTO{
			ID:       newUser.ID,
			Email:    newUser.Email,
			FullName: newUser.FullName,
		},
	}, nil
}
