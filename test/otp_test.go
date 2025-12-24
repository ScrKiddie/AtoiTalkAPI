package test

import (
	"AtoiTalkAPI/ent/tempcodes"
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

const (
	cfTurnstileAlwaysPasses      = "1x0000000000000000000000000000000AA"
	cfTurnstileAlwaysFails       = "2x0000000000000000000000000000000AA"
	cfTurnstileTokenAlreadySpent = "3x0000000000000000000000000000000AA"
)

func TestSendOTP(t *testing.T) {
	ctx := context.Background()

	const dummyTurnstileToken = "XXXX.DUMMY.TOKEN.XXXX"

	t.Run("Success - New OTP", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		reqBody := model.SendOTPRequest{
			Email: "test-new@example.com",
			Mode:  constant.TempCodeModeRegister,
			Token: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusOK, rr.Code)

		tempCode, err := testClient.TempCodes.Query().Where(tempcodes.Email(reqBody.Email)).Only(ctx)
		assert.NoError(t, err)
		assert.Equal(t, reqBody.Email, tempCode.Email)
		assert.True(t, time.Now().Before(tempCode.ExpiresAt))
	})

	t.Run("Success - Update Existing OTP", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		email := "test-update@example.com"

		reqBody1 := model.SendOTPRequest{
			Email: email,
			Mode:  constant.TempCodeModeRegister,
			Token: dummyTurnstileToken,
		}
		body1, _ := json.Marshal(reqBody1)
		req1, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body1))
		req1.Header.Set("Content-Type", "application/json")
		executeRequest(req1)

		firstCode, _ := testClient.TempCodes.Query().Where(tempcodes.Email(email)).Only(ctx)

		time.Sleep(3 * time.Second)

		reqBody2 := model.SendOTPRequest{
			Email: email,
			Mode:  constant.TempCodeModeReset,
			Token: dummyTurnstileToken,
		}
		body2, _ := json.Marshal(reqBody2)
		req2, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body2))
		req2.Header.Set("Content-Type", "application/json")
		rr := executeRequest(req2)

		assert.Equal(t, http.StatusOK, rr.Code)

		secondCode, _ := testClient.TempCodes.Query().Where(tempcodes.Email(email)).Only(ctx)
		assert.NotEqual(t, firstCode.Code, secondCode.Code)
		assert.Equal(t, tempcodes.Mode(constant.TempCodeModeReset), secondCode.Mode)
	})

	t.Run("Validation Error - Missing Email", func(t *testing.T) {
		clearDatabase(ctx)
		reqBody := model.SendOTPRequest{
			Mode:  constant.TempCodeModeRegister,
			Token: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Validation Error - Invalid Mode", func(t *testing.T) {
		clearDatabase(ctx)
		reqBody := model.SendOTPRequest{
			Email: "test-invalid-mode@example.com",
			Mode:  "invalid-mode",
			Token: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Rate Limit Error", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		reqBody := model.SendOTPRequest{
			Email: "ratelimit@example.com",
			Mode:  constant.TempCodeModeRegister,
			Token: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)

		req1, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req1.Header.Set("Content-Type", "application/json")
		rr1 := executeRequest(req1)
		assert.Equal(t, http.StatusOK, rr1.Code)

		req2, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req2.Header.Set("Content-Type", "application/json")
		rr2 := executeRequest(req2)

		assert.Equal(t, http.StatusTooManyRequests, rr2.Code)
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
			Email: "test-invalid-captcha@example.com",
			Mode:  constant.TempCodeModeRegister,
			Token: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var resp helper.ResponseError
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Equal(t, "Invalid captcha", resp.Error)
	})

	t.Run("Captcha Token Already Spent", func(t *testing.T) {
		clearDatabase(ctx)
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileTokenAlreadySpent
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		reqBody := model.SendOTPRequest{
			Email: "test-already-spent@example.com",
			Mode:  constant.TempCodeModeRegister,
			Token: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/otp/send", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var resp helper.ResponseError
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Equal(t, "Invalid captcha", resp.Error)
	})
}
