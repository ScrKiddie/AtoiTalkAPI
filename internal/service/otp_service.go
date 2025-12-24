package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/tempcodes"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"context"
	"crypto/rand"
	"embed"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"time"

	"github.com/go-playground/validator/v10"
)

//go:embed template
var templateFS embed.FS

type OTPService struct {
	client         *ent.Client
	cfg            *config.AppConfig
	validator      *validator.Validate
	emailAdapter   *adapter.EmailAdapter
	rateLimiter    *config.RateLimiter
	captchaAdapter *adapter.CaptchaAdapter
}

func NewOTPService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate, emailAdapter *adapter.EmailAdapter, rateLimiter *config.RateLimiter, captchaAdapter *adapter.CaptchaAdapter) *OTPService {
	return &OTPService{
		client:         client,
		cfg:            cfg,
		validator:      validator,
		emailAdapter:   emailAdapter,
		rateLimiter:    rateLimiter,
		captchaAdapter: captchaAdapter,
	}
}

func (s *OTPService) SendOTP(ctx context.Context, req model.SendOTPRequest) error {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err)
		return nil
	}

	allowed, retryAfter := s.rateLimiter.Allow(req.Email)
	if !allowed {
		minutes := int(math.Ceil(retryAfter.Minutes()))
		return helper.NewTooManyRequestsError(fmt.Sprintf("Too many requests. Please try again in %d minutes.", minutes))
	}

	if err := s.captchaAdapter.Verify(req.Token, ""); err != nil {
		slog.Warn("Captcha verification failed", "error", err)
		return helper.NewBadRequestError("Invalid captcha")
	}

	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		slog.Error("Failed to generate random number", "error", err)
		return helper.NewInternalServerError("")
	}
	code := fmt.Sprintf("%06d", n.Int64())

	hashedCode, err := helper.HashPassword(code)
	if err != nil {
		slog.Error("Failed to hash OTP code", "error", err)
		return helper.NewInternalServerError("")
	}

	existing, err := s.client.TempCodes.Query().
		Where(tempcodes.Email(req.Email)).
		Only(ctx)

	if err != nil && !ent.IsNotFound(err) {
		slog.Error("Failed to query temp code", "error", err)
		return helper.NewInternalServerError("")
	}

	expiresAt := time.Now().Add(time.Duration(s.cfg.TempCodeExp) * time.Second)

	if existing != nil {
		err = s.client.TempCodes.UpdateOne(existing).
			SetCode(hashedCode).
			SetMode(tempcodes.Mode(req.Mode)).
			SetExpiresAt(expiresAt).
			Exec(ctx)
	} else {
		err = s.client.TempCodes.Create().
			SetEmail(req.Email).
			SetCode(hashedCode).
			SetMode(tempcodes.Mode(req.Mode)).
			SetExpiresAt(expiresAt).
			Exec(ctx)
	}

	if err != nil {
		slog.Error("Failed to save temp code", "error", err)
		return helper.NewInternalServerError("")
	}

	sendEmail := func() {
		templateData := struct {
			Code    string
			Expired int
			Year    int
		}{
			Code:    code,
			Expired: s.cfg.TempCodeExp / 60,
			Year:    time.Now().Year(),
		}

		emailBody, err := helper.GenerateEmailBody(templateFS, "template/verify_email.html", templateData)
		if err != nil {
			slog.Error("Failed to generate email body", "error", err)
			return
		}

		err = s.emailAdapter.Send([]string{req.Email}, "Your OTP Code", emailBody)
		if err != nil {
			slog.Error("Failed to send OTP email", "error", err)

			_, delErr := s.client.TempCodes.Delete().Where(tempcodes.Email(req.Email)).Exec(context.TODO())
			if delErr != nil {
				slog.Error("Failed to delete temp code after email failure", "error", delErr)
			}
		}
	}

	if s.cfg.SMTPAsync {
		go sendEmail()
	} else {
		sendEmail()
	}

	return nil
}
