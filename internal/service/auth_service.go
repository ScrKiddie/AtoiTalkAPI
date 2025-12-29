package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/media"
	"AtoiTalkAPI/ent/otp"
	_ "AtoiTalkAPI/ent/runtime"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/ent/useridentity"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/constant"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/golang-jwt/jwt/v5"
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

func (s *AuthService) VerifyUser(ctx context.Context, tokenString string) (*model.UserDTO, error) {
	token, err := jwt.ParseWithClaims(tokenString, &helper.JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.cfg.JWTSecret), nil
	})

	if err != nil {
		slog.Warn("Failed to parse JWT token", "error", err)
		return nil, helper.NewUnauthorizedError("")
	}

	claims, ok := token.Claims.(*helper.JWTClaims)
	if !ok || !token.Valid {
		return nil, helper.NewUnauthorizedError("")
	}

	exists, err := s.client.User.Query().
		Where(user.ID(claims.UserID)).
		Exist(ctx)

	if err != nil {
		slog.Error("Failed to check user existence", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if !exists {
		return nil, helper.NewUnauthorizedError("")
	}

	return &model.UserDTO{
		ID: claims.UserID,
	}, nil
}

func (s *AuthService) Login(ctx context.Context, req model.LoginRequest) (*model.AuthResponse, error) {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err)
		return nil, helper.NewBadRequestError("")
	}

	if err := s.captchaAdapter.Verify(req.CaptchaToken, ""); err != nil {
		slog.Warn("Captcha verification failed", "error", err)
		return nil, helper.NewBadRequestError("")
	}

	req.Email = helper.NormalizeEmail(req.Email)

	u, err := s.client.User.Query().
		Where(user.Email(req.Email)).
		WithAvatar().
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewUnauthorizedError("")
		}
		slog.Error("Failed to query user", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if !helper.CheckPasswordHash(req.Password, *u.PasswordHash) {
		return nil, helper.NewUnauthorizedError("")
	}

	token, err := helper.GenerateJWT(s.cfg.JWTSecret, s.cfg.JWTExp, u.ID)
	if err != nil {
		slog.Error("Failed to generate JWT token", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	avatarURL := ""
	if u.Edges.Avatar != nil {
		avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, u.Edges.Avatar.FileName)
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

	sub, ok := payload.Claims["sub"].(string)
	if !ok || sub == "" {
		slog.Warn("Subject ID not found in token claims")
		return nil, helper.NewBadRequestError("")
	}

	u, err := s.client.User.Query().
		Where(user.Email(email)).
		WithAvatar().
		Only(ctx)

	if err != nil && !ent.IsNotFound(err) {
		slog.Error("Failed to query user", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	var avatarFileName string
	var fileSize int64
	var mimeType string

	var fileData []byte
	var fileUploadPath string
	var fileContentType string
	var mediaID int

	if u == nil {

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

		u, err = tx.User.Create().
			SetEmail(email).
			SetFullName(name).
			Save(ctx)
		if err != nil {
			slog.Error("Failed to create user", "error", err)
			return nil, helper.NewInternalServerError("")
		}

		if picture != "" {
			data, contentType, err := s.storageAdapter.Download(picture)
			if err != nil {
				slog.Error("Failed to download profile picture", "error", err)
			} else {
				fileName := helper.GenerateUniqueFileName(picture)
				filePath := filepath.Join(s.cfg.StorageProfile, fileName)
				fileSize = int64(len(data))
				mimeType = contentType

				if mimeType == "" {
					mimeType = "image/jpeg"
				}

				fileData = data
				fileUploadPath = filePath
				fileContentType = mimeType

				media, err := tx.Media.Create().
					SetFileName(fileName).
					SetOriginalName(filepath.Base(picture)).
					SetFileSize(fileSize).
					SetMimeType(mimeType).
					SetStatus(media.StatusActive).
					SetUploader(u).
					Save(ctx)

				if err != nil {
					slog.Error("Failed to create media record for google avatar", "error", err)
					fileData = nil
				} else {

					err = tx.User.UpdateOne(u).SetAvatar(media).Exec(ctx)
					if err != nil {
						slog.Error("Failed to link avatar to user", "error", err)
					} else {
						avatarFileName = fileName
						mediaID = media.ID
					}
				}
			}
		}

		_, err = tx.UserIdentity.Create().
			SetUser(u).
			SetProvider(useridentity.ProviderGoogle).
			SetProviderID(sub).
			SetProviderEmail(email).
			Save(ctx)

		if err != nil {
			slog.Error("Failed to create user identity", "error", err)
			return nil, helper.NewInternalServerError("")
		}

		if err := tx.Commit(); err != nil {
			slog.Error("Failed to commit transaction", "error", err)
			return nil, helper.NewInternalServerError("")
		}

		if fileData != nil {
			err = s.storageAdapter.StoreFromReader(bytes.NewReader(fileData), fileContentType, fileUploadPath)
			if err != nil {
				slog.Error("Failed to store profile picture after db commit", "error", err)

				if mediaID != 0 {
					if delErr := s.client.Media.DeleteOneID(mediaID).Exec(context.Background()); delErr != nil {
						slog.Error("Failed to delete orphan media record after file upload failure", "error", delErr, "mediaID", mediaID)
					}
				}
			}
		}

	} else {

		exists, err := s.client.UserIdentity.Query().
			Where(
				useridentity.UserID(u.ID),
				useridentity.ProviderEQ(useridentity.ProviderGoogle),
				useridentity.ProviderID(sub),
			).
			Exist(ctx)

		if err != nil {
			slog.Error("Failed to check user identity", "error", err)
		} else if !exists {
			_, err = s.client.UserIdentity.Create().
				SetUserID(u.ID).
				SetProvider(useridentity.ProviderGoogle).
				SetProviderID(sub).
				SetProviderEmail(email).
				Save(ctx)

			if err != nil {
				slog.Error("Failed to link google identity to existing user", "error", err)
			}
		}

		if u.Edges.Avatar != nil {
			avatarFileName = u.Edges.Avatar.FileName
		}
	}

	token, err := helper.GenerateJWT(s.cfg.JWTSecret, s.cfg.JWTExp, u.ID)
	if err != nil {
		slog.Error("Failed to generate JWT token", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	avatarURL := ""
	if avatarFileName != "" {
		avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, avatarFileName)
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

	req.Email = helper.NormalizeEmail(req.Email)

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
			otp.EmailEQ(req.Email),
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

	err = tx.OTP.DeleteOne(otpRecord).Exec(ctx)
	if err != nil {
		slog.Error("Failed to delete OTP", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	exists, err := tx.User.Query().
		Where(user.Email(req.Email)).
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
		SetEmail(req.Email).
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

func (s *AuthService) ResetPassword(ctx context.Context, req model.ResetPasswordRequest) error {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err)
		return helper.NewBadRequestError("")
	}

	if err := s.captchaAdapter.Verify(req.CaptchaToken, ""); err != nil {
		slog.Warn("Captcha verification failed", "error", err)
		return helper.NewBadRequestError("")
	}

	req.Email = helper.NormalizeEmail(req.Email)

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

	hashedCode := helper.HashOTP(req.Code, s.cfg.OTPSecret)

	otpRecord, err := tx.OTP.Query().
		Where(
			otp.EmailEQ(req.Email),
			otp.CodeEQ(hashedCode),
			otp.ModeEQ(constant.OTPModeReset),
			otp.ExpiresAtGT(time.Now()),
		).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewBadRequestError("Invalid or expired OTP")
		}
		slog.Error("Failed to query OTP", "error", err)
		return helper.NewInternalServerError("")
	}

	err = tx.OTP.DeleteOne(otpRecord).Exec(ctx)
	if err != nil {
		slog.Error("Failed to delete OTP", "error", err)
		return helper.NewInternalServerError("")
	}

	u, err := tx.User.Query().
		Where(user.Email(req.Email)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("")
		}
		slog.Error("Failed to query user", "error", err)
		return helper.NewInternalServerError("")
	}

	hashedPassword, err := helper.HashPassword(req.Password)
	if err != nil {
		slog.Error("Failed to hash password", "error", err)
		return helper.NewInternalServerError("")
	}

	err = tx.User.UpdateOne(u).
		SetPasswordHash(hashedPassword).
		Exec(ctx)
	if err != nil {
		slog.Error("Failed to update password", "error", err)
		return helper.NewInternalServerError("")
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return helper.NewInternalServerError("")
	}

	return nil
}
