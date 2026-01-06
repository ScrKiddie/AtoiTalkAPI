package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/otp"
	"AtoiTalkAPI/ent/user"
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

//go:embed template/*.html
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
		return helper.NewBadRequestError("")
	}

	req.Email = helper.NormalizeEmail(req.Email)

	if err := s.captchaAdapter.Verify(req.CaptchaToken, ""); err != nil {
		slog.Warn("Captcha verification failed", "error", err)
		return helper.NewBadRequestError("")
	}

	userExists, err := s.client.User.Query().
		Where(user.Email(req.Email)).
		Exist(ctx)
	if err != nil {
		slog.Error("Failed to check user existence", "error", err)
		return helper.NewInternalServerError("")
	}

	mode := otp.Mode(req.Mode)

	if (mode == otp.ModeRegister || mode == otp.ModeChangeEmail) && userExists {
		return helper.NewConflictError("Email already registered")
	}

	if mode == otp.ModeReset && !userExists {
		return helper.NewNotFoundError("")
	}

	allowed, retryAfter := s.rateLimiter.Allow(req.Email)
	if !allowed {
		minutes := int(math.Ceil(retryAfter.Minutes()))
		return helper.NewTooManyRequestsError(fmt.Sprintf("Please try again in %d minutes", minutes))
	}

	existing, err := s.client.OTP.Query().
		Where(otp.Email(req.Email)).
		Only(ctx)

	if err != nil && !ent.IsNotFound(err) {
		slog.Error("Failed to query OTP", "error", err)
		return helper.NewInternalServerError("")
	}

	expiresAt := time.Now().Add(time.Duration(s.cfg.OTPExp) * time.Second)

	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		slog.Error("Failed to generate random number", "error", err)
		return helper.NewInternalServerError("")
	}

	code := fmt.Sprintf("%06d", n.Int64())
	hashedCode := helper.HashOTP(code, s.cfg.OTPSecret)

	if existing != nil {
		err = s.client.OTP.UpdateOne(existing).
			SetCode(hashedCode).
			SetMode(mode).
			SetExpiresAt(expiresAt).
			Exec(ctx)
	} else {
		err = s.client.OTP.Create().
			SetEmail(req.Email).
			SetCode(hashedCode).
			SetMode(mode).
			SetExpiresAt(expiresAt).
			Exec(ctx)
	}

	if err != nil {
		slog.Error("Failed to save OTP", "error", err)
		return helper.NewInternalServerError("")
	}

	sendEmail := func() {
		templateData := struct {
			Code    string
			Expired int
			Year    int
		}{
			Code:    code,
			Expired: s.cfg.OTPExp / 60,
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

			_, delErr := s.client.OTP.Delete().Where(otp.Email(req.Email)).Exec(context.TODO())
			if delErr != nil {
				slog.Error("Failed to delete OTP after email failure", "error", delErr)
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
