package adapter

import (
	"AtoiTalkAPI/internal/config"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type CaptchaAdapter struct {
	httpClient *http.Client
	cfg        *config.AppConfig
}

func NewCaptchaAdapter(cfg *config.AppConfig, httpClient *http.Client) *CaptchaAdapter {
	return &CaptchaAdapter{
		httpClient: httpClient,
		cfg:        cfg,
	}
}

type turnstileVerifyResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

func (c *CaptchaAdapter) Verify(token string, ip string) error {
	url := "https://challenges.cloudflare.com/turnstile/v0/siteverify"

	payload := map[string]string{
		"secret":   c.cfg.TurnstileSecretKey,
		"response": token,
		"remoteip": ip,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal captcha payload: %w", err)
	}

	var resp *http.Response
	maxRetries := 5

	for i := 0; i < maxRetries; i++ {
		resp, err = c.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}

		if resp != nil {
			resp.Body.Close()
		}

		slog.Warn("Failed to verify captcha, retrying...", "attempt", i+1, "error", err)

		if i < maxRetries-1 {
			time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to verify captcha after %d attempts: %w", maxRetries, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read captcha response body: %w", err)
	}

	var verifyResp turnstileVerifyResponse
	if err := json.Unmarshal(body, &verifyResp); err != nil {
		return fmt.Errorf("failed to unmarshal captcha response: %w", err)
	}

	if !verifyResp.Success {
		return fmt.Errorf("captcha verification failed: %v", verifyResp.ErrorCodes)
	}

	return nil
}
