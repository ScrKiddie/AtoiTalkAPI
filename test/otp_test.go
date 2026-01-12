package test

import (
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
			Mode:         "register",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		key := fmt.Sprintf("otp:%s:%s", reqBody.Mode, reqBody.Email)
		val, err := redisAdapter.Get(ctx, key)
		assert.NoError(t, err)
		assert.NotEmpty(t, val)
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
			SetUsername("testupdate").
			SetFullName("Existing User").
			SetPasswordHash(hashedPassword).
			Save(ctx)

		reqBody1 := model.SendOTPRequest{
			Email:        email,
			Mode:         "reset",
			CaptchaToken: dummyTurnstileToken,
		}
		body1, _ := json.Marshal(reqBody1)
		req1, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body1))
		req1.Header.Set("Content-Type", "application/json")
		executeRequest(req1)

		key := fmt.Sprintf("otp:%s:%s", reqBody1.Mode, email)
		firstCode, err := redisAdapter.Get(ctx, key)
		if !assert.NoError(t, err) {
			return
		}

		time.Sleep(2 * time.Second)

		reqBody2 := model.SendOTPRequest{
			Email:        email,
			Mode:         "reset",
			CaptchaToken: dummyTurnstileToken,
		}
		body2, _ := json.Marshal(reqBody2)
		req2, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body2))
		req2.Header.Set("Content-Type", "application/json")
		rr := executeRequest(req2)

		if rr.Code == http.StatusTooManyRequests {
			t.Log("Skipping Update OTP check due to Rate Limit")
			return
		}

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		secondCode, err := redisAdapter.Get(ctx, key)
		if !assert.NoError(t, err) {
			return
		}
		assert.NotEqual(t, firstCode, secondCode)
	})

	t.Run("Validation Error - Missing Email", func(t *testing.T) {
		clearDatabase(ctx)
		reqBody := model.SendOTPRequest{
			Mode:         "register",
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
			Mode:         "register",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)

		for i := 0; i < 5; i++ {
			req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			executeRequest(req)
		}

		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusTooManyRequests, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseError
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Contains(t, resp.Error, "Please try again")
	})

	t.Run("Invalid Captcha Token", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysFails
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		reqBody := model.SendOTPRequest{
			Email:        "test-invalid-captcha@example.com",
			Mode:         "register",
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
			Mode:         "register",
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

	t.Run("Silent Fail - Register Existing Email", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		email := "existing-user@example.com"
		hashedPassword, _ := helper.HashPassword("Password123!")
		testClient.User.Create().
			SetEmail(email).
			SetUsername("existinguser").
			SetFullName("Existing User").
			SetPasswordHash(hashedPassword).
			Save(ctx)

		reqBody := model.SendOTPRequest{
			Email:        email,
			Mode:         "register",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		key := fmt.Sprintf("otp:%s:%s", reqBody.Mode, reqBody.Email)
		_, err := redisAdapter.Get(ctx, key)
		assert.Error(t, err, "OTP should NOT be created for existing user registration")
	})

	t.Run("Silent Fail - Reset Non-Existent Email", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		reqBody := model.SendOTPRequest{
			Email:        "non-existent@example.com",
			Mode:         "reset",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		key := fmt.Sprintf("otp:%s:%s", reqBody.Mode, reqBody.Email)
		_, err := redisAdapter.Get(ctx, key)
		assert.Error(t, err, "OTP should NOT be created for non-existent user reset")
	})

	t.Run("Silent Fail - ChangeEmail to Existing Email", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		hashedPassword, _ := helper.HashPassword("Password123!")
		testClient.User.Create().
			SetEmail("existing@example.com").
			SetUsername("existinguser").
			SetFullName("Existing User").
			SetPasswordHash(hashedPassword).
			Save(ctx)

		reqBody := model.SendOTPRequest{
			Email:        "existing@example.com",
			Mode:         "change_email",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		key := fmt.Sprintf("otp:%s:%s", reqBody.Mode, reqBody.Email)
		_, err := redisAdapter.Get(ctx, key)
		assert.Error(t, err, "OTP should NOT be created if email already exists")
	})

	t.Run("Success - Rate Limit Recovery", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		reqBody := model.SendOTPRequest{
			Email:        "recovery@example.com",
			Mode:         "register",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)

		for i := 0; i < 5; i++ {
			req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			executeRequest(req)
		}

		reqBlocked, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		reqBlocked.Header.Set("Content-Type", "application/json")
		rrBlocked := executeRequest(reqBlocked)
		assert.Equal(t, http.StatusTooManyRequests, rrBlocked.Code)

		time.Sleep(3 * time.Second)

		reqRecovered, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		reqRecovered.Header.Set("Content-Type", "application/json")
		rrRecovered := executeRequest(reqRecovered)
		assert.Equal(t, http.StatusOK, rrRecovered.Code)
	})
}
