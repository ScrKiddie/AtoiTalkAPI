package test

import (
	"AtoiTalkAPI/ent/otp"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/internal/constant"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGoogleExchange(t *testing.T) {
	clearDatabase(context.Background())

	t.Run("Validation Error", func(t *testing.T) {
		reqBody := model.GoogleLoginRequest{Code: ""}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/google", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var resp helper.ResponseError
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.NotEmpty(t, resp.Error)
	})

	t.Run("Invalid Token", func(t *testing.T) {
		reqBody := model.GoogleLoginRequest{Code: "invalid-token-string"}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/google", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		var resp helper.ResponseError
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.NotEmpty(t, resp.Error)
	})

	t.Run("Valid Token", func(t *testing.T) {
		validToken := os.Getenv("TEST_GOOGLE_ID_TOKEN")
		if validToken == "" {
			t.Skip("Skipping Valid Token test: TEST_GOOGLE_ID_TOKEN not set")
		}

		makeRequest := func() *httptest.ResponseRecorder {
			reqBody := model.GoogleLoginRequest{Code: validToken}
			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/api/auth/google", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			return executeRequest(req)
		}

		t.Run("Register", func(t *testing.T) {
			clearDatabase(context.Background())
			rr := makeRequest()
			assert.Equal(t, http.StatusOK, rr.Code, "Response body: %s", rr.Body.String())

			var resp helper.ResponseSuccess
			err := json.Unmarshal(rr.Body.Bytes(), &resp)
			assert.NoError(t, err)

			dataMap, ok := resp.Data.(map[string]interface{})
			assert.True(t, ok, "Expected data to be a map")
			assert.Contains(t, dataMap, "token")

			userMap, ok := dataMap["user"].(map[string]interface{})
			assert.True(t, ok, "Expected user object in response data")
			assert.Contains(t, userMap, "email")

			if avatarURL, ok := userMap["avatar"].(string); ok && avatarURL != "" {
				parts := strings.Split(avatarURL, "/")
				fileName := parts[len(parts)-1]
				_, b, _, _ := runtime.Caller(0)
				testDir := filepath.Dir(b)
				physicalPath := filepath.Join(testDir, testConfig.StorageProfile, fileName)
				assert.FileExists(t, physicalPath, "Profile picture file should be created")
			} else {
				t.Log("No avatar URL returned, skipping file check")
			}
		})

		t.Run("Login Existing User", func(t *testing.T) {

			rr := makeRequest()
			assert.Equal(t, http.StatusOK, rr.Code, "Response body: %s", rr.Body.String())

			var resp helper.ResponseSuccess
			err := json.Unmarshal(rr.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.NotNil(t, resp.Data)
		})
	})
}

func TestRegister(t *testing.T) {

	validEmail := "test@example.com"
	validCode := "123456"

	t.Run("Success", func(t *testing.T) {
		clearDatabase(context.Background())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(validEmail, validCode, time.Now().Add(5*time.Minute))

		reqBody := model.RegisterUserRequest{
			Email:        validEmail,
			Code:         validCode,
			FullName:     "Test User",
			Password:     "Password123!",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataMap, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.Contains(t, dataMap, "token")

		userMap, ok := dataMap["user"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, validEmail, userMap["email"])
		assert.Contains(t, userMap, "avatar")
	})

	t.Run("Invalid Captcha", func(t *testing.T) {
		clearDatabase(context.Background())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysFails
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(validEmail, validCode, time.Now().Add(5*time.Minute))

		reqBody := model.RegisterUserRequest{
			Email:        validEmail,
			Code:         validCode,
			FullName:     "Test User",
			Password:     "Password123!",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var resp helper.ResponseError
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Equal(t, helper.MsgBadRequest, resp.Error)
	})

	t.Run("Invalid OTP", func(t *testing.T) {
		clearDatabase(context.Background())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(validEmail, validCode, time.Now().Add(5*time.Minute))

		reqBody := model.RegisterUserRequest{
			Email:        validEmail,
			Code:         "000000",
			FullName:     "Test User",
			Password:     "Password123!",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Expired OTP", func(t *testing.T) {
		clearDatabase(context.Background())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP("expired@example.com", validCode, time.Now().Add(-5*time.Minute))

		reqBody := model.RegisterUserRequest{
			Email:        "expired@example.com",
			Code:         validCode,
			FullName:     "Test User",
			Password:     "Password123!",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Email Already Registered", func(t *testing.T) {
		clearDatabase(context.Background())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		hashedPassword, _ := helper.HashPassword("Password123!")
		testClient.User.Create().
			SetEmail("existing@example.com").
			SetFullName("Existing User").
			SetPasswordHash(hashedPassword).
			Save(context.Background())

		createOTP("existing@example.com", validCode, time.Now().Add(5*time.Minute))

		reqBody := model.RegisterUserRequest{
			Email:        "existing@example.com",
			Code:         validCode,
			FullName:     "Test User",
			Password:     "Password123!",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusConflict, rr.Code)
	})
}

func TestResetPassword(t *testing.T) {
	validEmail := "reset@example.com"
	validCode := "123456"
	newPassword := "NewPassword123!"

	t.Run("Success", func(t *testing.T) {
		clearDatabase(context.Background())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		hashedPassword, _ := helper.HashPassword("OldPassword123!")
		testClient.User.Create().
			SetEmail(validEmail).
			SetFullName("Reset User").
			SetPasswordHash(hashedPassword).
			Save(context.Background())

		createOTP(validEmail, validCode, time.Now().Add(5*time.Minute))

		hashedCode := helper.HashOTP(validCode, testConfig.OTPSecret)
		testClient.OTP.Update().
			Where(otp.Email(validEmail)).
			SetMode(otp.Mode(constant.OTPModeReset)).
			SetCode(hashedCode).
			Exec(context.Background())

		reqBody := model.ResetPasswordRequest{
			Email:           validEmail,
			Code:            validCode,
			Password:        newPassword,
			ConfirmPassword: newPassword,
			CaptchaToken:    dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/reset-password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusOK, rr.Code)

		u, _ := testClient.User.Query().Where(user.Email(validEmail)).Only(context.Background())
		assert.NotNil(t, u.PasswordHash)
		assert.True(t, helper.CheckPasswordHash(newPassword, *u.PasswordHash))
	})

	t.Run("User Not Found", func(t *testing.T) {
		clearDatabase(context.Background())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP("nonexistent@example.com", validCode, time.Now().Add(5*time.Minute))
		hashedCode := helper.HashOTP(validCode, testConfig.OTPSecret)
		testClient.OTP.Update().
			Where(otp.Email("nonexistent@example.com")).
			SetMode(otp.Mode(constant.OTPModeReset)).
			SetCode(hashedCode).
			Exec(context.Background())

		reqBody := model.ResetPasswordRequest{
			Email:           "nonexistent@example.com",
			Code:            validCode,
			Password:        newPassword,
			ConfirmPassword: newPassword,
			CaptchaToken:    dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/reset-password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Invalid OTP", func(t *testing.T) {
		clearDatabase(context.Background())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		hashedPassword, _ := helper.HashPassword("OldPassword123!")
		testClient.User.Create().
			SetEmail(validEmail).
			SetFullName("Reset User").
			SetPasswordHash(hashedPassword).
			Save(context.Background())

		createOTP(validEmail, validCode, time.Now().Add(5*time.Minute))
		hashedCode := helper.HashOTP(validCode, testConfig.OTPSecret)
		testClient.OTP.Update().
			Where(otp.Email(validEmail)).
			SetMode(otp.Mode(constant.OTPModeReset)).
			SetCode(hashedCode).
			Exec(context.Background())

		reqBody := model.ResetPasswordRequest{
			Email:           validEmail,
			Code:            "000000",
			Password:        newPassword,
			ConfirmPassword: newPassword,
			CaptchaToken:    dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/reset-password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Password Mismatch", func(t *testing.T) {
		clearDatabase(context.Background())

		reqBody := model.ResetPasswordRequest{
			Email:           validEmail,
			Code:            validCode,
			Password:        newPassword,
			ConfirmPassword: "DifferentPassword123!",
			CaptchaToken:    dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/reset-password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}
