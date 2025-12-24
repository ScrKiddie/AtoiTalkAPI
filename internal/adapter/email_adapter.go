package adapter

import (
	"AtoiTalkAPI/internal/config"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
	"time"
)

type EmailAdapter struct {
	host      string
	port      int
	user      string
	password  string
	fromEmail string
	fromName  string
}

func NewEmailAdapter(cfg *config.AppConfig) *EmailAdapter {
	return &EmailAdapter{
		host:      cfg.SMTPHost,
		port:      cfg.SMTPPort,
		user:      cfg.SMTPUser,
		password:  cfg.SMTPPassword,
		fromEmail: cfg.SMTPFromEmail,
		fromName:  cfg.SMTPFromName,
	}
}

func (e *EmailAdapter) Send(to []string, subject string, body string) error {
	addr := fmt.Sprintf("%s:%d", e.host, e.port)
	auth := smtp.PlainAuth("", e.user, e.password, e.host)

	msg := []byte(fmt.Sprintf("From: %s <%s>\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=\"UTF-8\"\r\n"+
		"\r\n"+
		"%s\r\n", e.fromName, e.fromEmail, strings.Join(to, ","), subject, body))

	var err error
	maxRetries := 5
	timeout := 10 * time.Second

	for i := 0; i < maxRetries; i++ {
		errChan := make(chan error, 1)

		go func() {
			errChan <- smtp.SendMail(addr, auth, e.fromEmail, to, msg)
		}()

		select {
		case sendErr := <-errChan:
			err = sendErr
		case <-time.After(timeout):
			err = fmt.Errorf("email sending timed out after %v", timeout)
		}

		if err == nil {
			return nil
		}

		slog.Warn("Failed to send email, retrying...", "attempt", i+1, "error", err)

		if i < maxRetries-1 {
			time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
		}
	}

	return fmt.Errorf("failed to send email after %d attempts: %w", maxRetries, err)
}
