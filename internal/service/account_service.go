package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/otp"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/ent/useridentity"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"context"
	"log/slog"
	"time"

	"github.com/go-playground/validator/v10"
)

type AccountService struct {
	client    *ent.Client
	cfg       *config.AppConfig
	validator *validator.Validate
}

func NewAccountService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate) *AccountService {
	return &AccountService{
		client:    client,
		cfg:       cfg,
		validator: validator,
	}
}

func (s *AccountService) ChangePassword(ctx context.Context, userID int, req model.ChangePasswordRequest) error {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err, "userID", userID)
		return helper.NewBadRequestError("")
	}

	u, err := s.client.User.Query().Where(user.ID(userID)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("")
		}
		slog.Error("Failed to query user", "error", err, "userID", userID)
		return helper.NewInternalServerError("")
	}

	if u.PasswordHash != nil {

		if req.OldPassword == nil {
			return helper.NewBadRequestError("")
		}
		if !helper.CheckPasswordHash(*req.OldPassword, *u.PasswordHash) {
			return helper.NewBadRequestError("")
		}
	}

	hashedPassword, err := helper.HashPassword(req.NewPassword)
	if err != nil {
		slog.Error("Failed to hash password", "error", err, "userID", userID)
		return helper.NewInternalServerError("")
	}

	err = s.client.User.UpdateOneID(userID).SetPasswordHash(hashedPassword).Exec(ctx)
	if err != nil {
		slog.Error("Failed to update password", "error", err, "userID", userID)
		return helper.NewInternalServerError("")
	}

	return nil
}

func (s *AccountService) ChangeEmail(ctx context.Context, userID int, req model.ChangeEmailRequest) error {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err, "userID", userID)
		return helper.NewBadRequestError("")
	}

	req.Email = helper.NormalizeEmail(req.Email)

	u, err := s.client.User.Query().Where(user.ID(userID)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("")
		}
		slog.Error("Failed to query user", "error", err, "userID", userID)
		return helper.NewInternalServerError("")
	}

	if u.PasswordHash == nil {
		return helper.NewBadRequestError("You must set a password before changing your email address")
	}

	if u.Email == req.Email {
		return helper.NewBadRequestError("")
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		slog.Error("Failed to start transaction", "error", err)
		return helper.NewInternalServerError("")
	}

	defer func() {
		_ = tx.Rollback()
		if v := recover(); v != nil {
			panic(v)
		}
	}()

	exists, err := tx.User.Query().
		Where(user.Email(req.Email)).
		Exist(ctx)
	if err != nil {
		slog.Error("Failed to check user existence", "error", err)
		return helper.NewInternalServerError("")
	}
	if exists {
		return helper.NewConflictError("Email already registered")
	}

	hashedCode := helper.HashOTP(req.Code, s.cfg.OTPSecret)

	otpRecord, err := tx.OTP.Query().
		Where(
			otp.EmailEQ(req.Email),
			otp.CodeEQ(hashedCode),
			otp.ModeEQ(otp.ModeChangeEmail),
			otp.ExpiresAtGT(time.Now()),
		).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewBadRequestError("")
		}
		slog.Error("Failed to query OTP", "error", err)
		return helper.NewInternalServerError("")
	}

	err = tx.OTP.DeleteOne(otpRecord).Exec(ctx)
	if err != nil {
		slog.Error("Failed to delete OTP", "error", err)
		return helper.NewInternalServerError("")
	}

	err = tx.User.UpdateOneID(userID).SetEmail(req.Email).Exec(ctx)
	if err != nil {
		slog.Error("Failed to update email", "error", err, "userID", userID)
		return helper.NewInternalServerError("")
	}

	_, err = tx.UserIdentity.Delete().
		Where(useridentity.UserID(userID)).
		Exec(ctx)
	if err != nil {
		slog.Error("Failed to unlink user identities", "error", err, "userID", userID)
		return helper.NewInternalServerError("")
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return helper.NewInternalServerError("")
	}

	return nil
}
