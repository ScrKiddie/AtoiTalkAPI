package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/constant"
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
	redisAdapter   *adapter.RedisAdapter
}

func NewOTPService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate, emailAdapter *adapter.EmailAdapter, rateLimiter *config.RateLimiter, captchaAdapter *adapter.CaptchaAdapter, redisAdapter *adapter.RedisAdapter) *OTPService {
	return &OTPService{
		client:         client,
		cfg:            cfg,
		validator:      validator,
		emailAdapter:   emailAdapter,
		rateLimiter:    rateLimiter,
		captchaAdapter: captchaAdapter,
		redisAdapter:   redisAdapter,
	}
}

func (s *OTPService) SendOTP(ctx context.Context, req model.SendOTPRequest) error {
	req.Email = helper.NormalizeEmail(req.Email)

	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err)
		return helper.NewBadRequestError("")
	}

	if err := s.captchaAdapter.Verify(req.CaptchaToken, ""); err != nil {
		slog.Warn("Captcha verification failed", "error", err)
		return helper.NewBadRequestError("")
	}

	allowed, retryAfter := s.rateLimiter.Allow(req.Email)
	if !allowed {
		minutes := int(math.Ceil(retryAfter.Minutes()))
		return helper.NewTooManyRequestsError(fmt.Sprintf("Please try again in %d minutes", minutes))
	}

	userExists, err := s.client.User.Query().
		Where(user.Email(req.Email)).
		Exist(ctx)
	if err != nil {
		slog.Error("Failed to check user existence", "error", err)
		return helper.NewInternalServerError("")
	}

	mode := constant.OTPMode(req.Mode)

	shouldSend := true

	if (mode == constant.ModeRegister || mode == constant.ModeChangeEmail) && userExists {
		shouldSend = false
		slog.Info("OTP request suppressed: Email already registered", "email", req.Email, "mode", mode)
	}

	if mode == constant.ModeReset && !userExists {
		shouldSend = false
		slog.Info("OTP request suppressed: Email not found for reset", "email", req.Email)
	}

	if !shouldSend {
		return nil
	}

	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		slog.Error("Failed to generate random number", "error", err)
		return helper.NewInternalServerError("")
	}

	code := fmt.Sprintf("%06d", n.Int64())
	hashedCode := helper.HashOTP(code, s.cfg.OTPSecret)

	key := fmt.Sprintf("otp:%s:%s", req.Mode, req.Email)
	err = s.redisAdapter.Set(ctx, key, hashedCode, time.Duration(s.cfg.OTPExp)*time.Second)
	if err != nil {
		slog.Error("Failed to save OTP to Redis", "error", err)
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
			Year:    time.Now().UTC().Year(),
		}

		emailBody, err := helper.GenerateEmailBody(templateFS, "template/verify_email.html", templateData)
		if err != nil {
			slog.Error("Failed to generate email body", "error", err)
			return
		}

		err = s.emailAdapter.Send([]string{req.Email}, "Your OTP Code", emailBody)
		if err != nil {
			slog.Error("Failed to send OTP email", "error", err)

			s.redisAdapter.Del(context.Background(), key)
		}
	}

	if s.cfg.SMTPAsync {
		go sendEmail()
	} else {
		sendEmail()
	}

	return nil
}

func (s *OTPService) VerifyOTP(ctx context.Context, email, code, mode string) error {
	key := fmt.Sprintf("otp:%s:%s", mode, email)
	hashedCode, err := s.redisAdapter.Get(ctx, key)
	if err != nil {
		return helper.NewBadRequestError("Invalid or expired OTP")
	}

	if hashedCode != helper.HashOTP(code, s.cfg.OTPSecret) {
		return helper.NewBadRequestError("Invalid OTP code")
	}

	s.redisAdapter.Del(ctx, key)
	return nil
}
