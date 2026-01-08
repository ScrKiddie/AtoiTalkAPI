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
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
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
		Where(
			user.ID(claims.UserID),
			user.DeletedAtIsNil(),
		).
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
		Where(
			user.Email(req.Email),
			user.DeletedAtIsNil(),
		).
		WithAvatar().
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewUnauthorizedError("")
		}
		slog.Error("Failed to query user", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if u.PasswordHash == nil || !helper.CheckPasswordHash(req.Password, *u.PasswordHash) {
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

	username := ""
	if u.Username != nil {
		username = *u.Username
	}

	fullName := ""
	if u.FullName != nil {
		fullName = *u.FullName
	}

	return &model.AuthResponse{
		Token: token,
		User: model.UserDTO{
			ID:       u.ID,
			Email:    *u.Email,
			Username: username,
			FullName: fullName,
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

	emailVerified, ok := payload.Claims["email_verified"].(bool)
	if !ok || !emailVerified {
		slog.Warn("Email from Google token is not verified", "email", email)
		return nil, helper.NewBadRequestError("Email from Google is not verified.")
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
		Where(
			user.Email(email),
			user.DeletedAtIsNil(),
		).
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
	var mediaID uuid.UUID

	if u == nil {

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
				avatarFileName = fileName
			}
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

		baseUsername := strings.Split(email, "@")[0]
		baseUsername = helper.NormalizeUsername(baseUsername)
		if len(baseUsername) > 40 {
			baseUsername = baseUsername[:40]
		}
		if len(baseUsername) < 3 {
			baseUsername = "user" + baseUsername
		}

		var finalUsername string
		for i := 0; i < 3; i++ {
			randNum := rand.Intn(10000)
			candidate := fmt.Sprintf("%s%04d", baseUsername, randNum)

			exists, _ := tx.User.Query().Where(user.UsernameEQ(candidate)).Exist(ctx)
			if !exists {
				finalUsername = candidate
				break
			}
		}

		if finalUsername == "" {
			return nil, helper.NewConflictError("Failed to generate unique username")
		}

		u, err = tx.User.Create().
			SetEmail(email).
			SetUsername(finalUsername).
			SetFullName(name).
			Save(ctx)
		if err != nil {
			slog.Error("Failed to create user", "error", err)
			return nil, helper.NewInternalServerError("")
		}

		if fileData != nil {
			media, err := tx.Media.Create().
				SetFileName(avatarFileName).
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
					mediaID = media.ID
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

				if mediaID != uuid.Nil {
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

	username := ""
	if u.Username != nil {
		username = *u.Username
	}

	fullName := ""
	if u.FullName != nil {
		fullName = *u.FullName
	}

	return &model.AuthResponse{
		Token: token,
		User: model.UserDTO{
			ID:       u.ID,
			Email:    *u.Email,
			Username: username,
			FullName: fullName,
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
	req.Username = helper.NormalizeUsername(req.Username)

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
			otp.ModeEQ(otp.ModeRegister),
			otp.ExpiresAtGT(time.Now().UTC()),
		).
		ForUpdate().
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
		Where(
			user.Email(req.Email),
			user.DeletedAtIsNil(),
		).
		Exist(ctx)
	if err != nil {
		slog.Error("Failed to check user existence", "error", err)
		return nil, helper.NewInternalServerError("")
	}
	if exists {
		return nil, helper.NewConflictError("Email already registered")
	}

	usernameExists, err := tx.User.Query().
		Where(
			user.UsernameEQ(req.Username),
			user.DeletedAtIsNil(),
		).
		Exist(ctx)
	if err != nil {
		slog.Error("Failed to check username existence", "error", err)
		return nil, helper.NewInternalServerError("")
	}
	if usernameExists {
		return nil, helper.NewConflictError("Username already taken")
	}

	hashedPassword, err := helper.HashPassword(req.Password)
	if err != nil {
		slog.Error("Failed to hash password", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	newUser, err := tx.User.Create().
		SetEmail(req.Email).
		SetUsername(req.Username).
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

	fullName := ""
	if newUser.FullName != nil {
		fullName = *newUser.FullName
	}

	return &model.AuthResponse{
		Token: token,
		User: model.UserDTO{
			ID:       newUser.ID,
			Email:    *newUser.Email,
			Username: *newUser.Username,
			FullName: fullName,
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
			otp.ModeEQ(otp.ModeReset),
			otp.ExpiresAtGT(time.Now().UTC()),
		).
		ForUpdate().
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
		Where(
			user.Email(req.Email),
			user.DeletedAtIsNil(),
		).
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
