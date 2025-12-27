package adapter

import (
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"fmt"
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

	operation := func() (struct{}, bool, error) {
		errChan := make(chan error, 1)
		timeout := 10 * time.Second

		go func() {
			errChan <- smtp.SendMail(addr, auth, e.fromEmail, to, msg)
		}()

		var err error
		select {
		case sendErr := <-errChan:
			err = sendErr
		case <-time.After(timeout):
			err = fmt.Errorf("email sending timed out after %v", timeout)
		}

		if err != nil {

			return struct{}{}, true, err
		}

		return struct{}{}, false, nil
	}

	_, err := helper.RetryWithBackoff(operation, 3, 1*time.Second)
	return err
}
