package test

import (
	"AtoiTalkAPI/ent/otp"
	"AtoiTalkAPI/internal/constant"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSendOTP(t *testing.T) {
	ctx := context.Background()

	t.Run("Success - New OTP", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		reqBody := model.SendOTPRequest{
			Email:        "test-new@example.com",
			Mode:         constant.OTPModeRegister,
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		otpRecord, err := testClient.OTP.Query().Where(otp.Email(reqBody.Email)).Only(ctx)
		assert.NoError(t, err)
		assert.Equal(t, reqBody.Email, otpRecord.Email)
		assert.True(t, time.Now().Before(otpRecord.ExpiresAt))
	})

	t.Run("Success - Update Existing OTP", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		email := "test-update@example.com"

		hashedPassword, _ := helper.HashPassword("Password123!")
		testClient.User.Create().
			SetEmail(email).
			SetFullName("Existing User").
			SetPasswordHash(hashedPassword).
			Save(ctx)

		reqBody1 := model.SendOTPRequest{
			Email:        email,
			Mode:         constant.OTPModeReset,
			CaptchaToken: dummyTurnstileToken,
		}
		body1, _ := json.Marshal(reqBody1)
		req1, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body1))
		req1.Header.Set("Content-Type", "application/json")
		executeRequest(req1)

		firstCode, _ := testClient.OTP.Query().Where(otp.Email(email)).Only(ctx)

		time.Sleep(3 * time.Second)

		reqBody2 := model.SendOTPRequest{
			Email:        email,
			Mode:         constant.OTPModeReset,
			CaptchaToken: dummyTurnstileToken,
		}
		body2, _ := json.Marshal(reqBody2)
		req2, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body2))
		req2.Header.Set("Content-Type", "application/json")
		rr := executeRequest(req2)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		secondCode, _ := testClient.OTP.Query().Where(otp.Email(email)).Only(ctx)
		assert.NotEqual(t, firstCode.Code, secondCode.Code)
		assert.Equal(t, otp.Mode(constant.OTPModeReset), secondCode.Mode)
	})

	t.Run("Validation Error - Missing Email", func(t *testing.T) {
		clearDatabase(ctx)
		reqBody := model.SendOTPRequest{
			Mode:         constant.OTPModeRegister,
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Validation Error - Invalid Mode", func(t *testing.T) {
		clearDatabase(ctx)
		reqBody := model.SendOTPRequest{
			Email:        "test-invalid-mode@example.com",
			Mode:         "invalid-mode",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Rate Limit Error", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		reqBody := model.SendOTPRequest{
			Email:        "ratelimit@example.com",
			Mode:         constant.OTPModeRegister,
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)

		req1, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req1.Header.Set("Content-Type", "application/json")
		rr1 := executeRequest(req1)
		assert.Equal(t, http.StatusOK, rr1.Code)

		req2, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req2.Header.Set("Content-Type", "application/json")
		rr2 := executeRequest(req2)

		if !assert.Equal(t, http.StatusTooManyRequests, rr2.Code) {
			printBody(t, rr2)
		}
		var resp helper.ResponseError
		json.Unmarshal(rr2.Body.Bytes(), &resp)
		assert.Contains(t, resp.Error, "Too many requests. Please try again in")
	})

	t.Run("Invalid Captcha Token", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysFails
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		reqBody := model.SendOTPRequest{
			Email:        "test-invalid-captcha@example.com",
			Mode:         constant.OTPModeRegister,
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseError
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Equal(t, helper.MsgBadRequest, resp.Error)
	})

	t.Run("Captcha Token Already Spent", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileTokenAlreadySpent
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		reqBody := model.SendOTPRequest{
			Email:        "test-already-spent@example.com",
			Mode:         constant.OTPModeRegister,
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseError
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Equal(t, helper.MsgBadRequest, resp.Error)
	})

	t.Run("Register - Email Already Exists", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		email := "existing-user@example.com"
		hashedPassword, _ := helper.HashPassword("Password123!")
		testClient.User.Create().
			SetEmail(email).
			SetFullName("Existing User").
			SetPasswordHash(hashedPassword).
			Save(ctx)

		reqBody := model.SendOTPRequest{
			Email:        email,
			Mode:         constant.OTPModeRegister,
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusConflict, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseError
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Equal(t, "Email already registered", resp.Error)
	})

	t.Run("Reset - Email Not Found", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		reqBody := model.SendOTPRequest{
			Email:        "non-existent@example.com",
			Mode:         constant.OTPModeReset,
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusNotFound, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseError
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Equal(t, helper.MsgNotFound, resp.Error)
	})
}
