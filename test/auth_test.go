package test

import (
	"AtoiTalkAPI/ent/media"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/ent/useridentity"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestLogin(t *testing.T) {
	validPassword := "Password123!"

	t.Run("Success", func(t *testing.T) {
		clearDatabase(context.Background())
		u := createTestUser(t, "loginsuccess")
		validEmail := *u.Email
		validUsername := *u.Username

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		reqBody := model.LoginRequest{
			Email:        validEmail,
			Password:     validPassword,
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataMap, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.Contains(t, dataMap, "token")

		userMap, ok := dataMap["user"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, validEmail, userMap["email"])
		assert.Equal(t, validUsername, userMap["username"])
	})

	t.Run("Invalid Captcha", func(t *testing.T) {
		clearDatabase(context.Background())
		u := createTestUser(t, "logincaptcha")
		validEmail := *u.Email

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysFails
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		reqBody := model.LoginRequest{
			Email:        validEmail,
			Password:     validPassword,
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("User Not Found", func(t *testing.T) {
		clearDatabase(context.Background())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		reqBody := model.LoginRequest{
			Email:        "nonexistent@example.com",
			Password:     validPassword,
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusUnauthorized, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Invalid Password", func(t *testing.T) {
		clearDatabase(context.Background())
		u := createTestUser(t, "loginwrongpass")
		validEmail := *u.Email

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		reqBody := model.LoginRequest{
			Email:        validEmail,
			Password:     "WrongPassword123!",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusUnauthorized, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Fail - Login Deleted User", func(t *testing.T) {
		clearDatabase(context.Background())
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		hashedPassword, _ := helper.HashPassword(validPassword)

		email := fmt.Sprintf("deletedlogin%d@test.com", time.Now().UnixNano())
		username := fmt.Sprintf("deletedlogin%d", time.Now().UnixNano())

		testClient.User.Create().
			SetEmail(email).
			SetUsername(username).
			SetFullName("Deleted User").
			SetPasswordHash(hashedPassword).
			SetDeletedAt(time.Now().UTC()).
			Save(context.Background())

		reqBody := model.LoginRequest{
			Email:        email,
			Password:     validPassword,
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestLogout(t *testing.T) {
	clearDatabase(context.Background())
	u := createTestUser(t, "logoutuser")
	token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u.ID)

	t.Run("Success Logout", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/api/auth/logout", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		key := fmt.Sprintf("blacklist:%s", token)
		val, err := redisAdapter.Get(context.Background(), key)
		assert.NoError(t, err, "Redis should contain the blacklisted token")
		assert.Equal(t, "revoked", val, "Token value in Redis should be 'revoked'")

		req2, _ := http.NewRequest("GET", "/api/user/current", nil)
		req2.Header.Set("Authorization", "Bearer "+token)
		rr2 := executeRequest(req2)
		assert.Equal(t, http.StatusUnauthorized, rr2.Code)
	})
}

func TestGoogleExchange(t *testing.T) {
	clearDatabase(context.Background())

	t.Run("Validation Error", func(t *testing.T) {
		reqBody := model.GoogleLoginRequest{Code: ""}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/google", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseError
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.NotEmpty(t, resp.Error)
	})

	t.Run("Invalid Code", func(t *testing.T) {
		reqBody := model.GoogleLoginRequest{Code: "invalid-auth-code"}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/google", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusUnauthorized, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseError
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.NotEmpty(t, resp.Error)
	})

	t.Run("Valid Code", func(t *testing.T) {
		validCode := os.Getenv("TEST_GOOGLE_AUTH_CODE")
		if validCode == "" {
			t.Skip("Skipping Valid Code test: TEST_GOOGLE_AUTH_CODE not set")
		}

		makeRequest := func() *httptest.ResponseRecorder {
			reqBody := model.GoogleLoginRequest{Code: validCode}
			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/api/auth/google", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			return executeRequest(req)
		}

		t.Run("Register and Link Identity", func(t *testing.T) {
			clearDatabase(context.Background())
			rr := makeRequest()
			if !assert.Equal(t, http.StatusOK, rr.Code, "Response body: %s", rr.Body.String()) {
				printBody(t, rr)
			}

			var resp helper.ResponseSuccess
			err := json.Unmarshal(rr.Body.Bytes(), &resp)
			assert.NoError(t, err)

			dataMap, ok := resp.Data.(map[string]interface{})
			assert.True(t, ok, "Expected data to be a map")
			assert.Contains(t, dataMap, "token")

			userMap, ok := dataMap["user"].(map[string]interface{})
			assert.True(t, ok, "Expected user object in response data")
			assert.NotEmpty(t, userMap["email"])
			assert.NotEmpty(t, userMap["username"])

			userIDStr := userMap["id"].(string)
			userID, _ := uuid.Parse(userIDStr)

			identity, err := testClient.UserIdentity.Query().
				Where(useridentity.UserID(userID)).
				Only(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, useridentity.ProviderGoogle, identity.Provider)
			assert.NotEmpty(t, identity.ProviderID)

			if avatarURL, ok := userMap["avatar"].(string); ok && avatarURL != "" {
				parts := strings.Split(avatarURL, "/")
				fileName := parts[len(parts)-1]

				m, err := testClient.Media.Query().Where(media.FileName(fileName)).Only(context.Background())
				assert.NoError(t, err)
				assert.Equal(t, userID, m.UploadedByID, "Media uploader should be set to the user")
			} else {
				t.Log("No avatar URL returned, skipping file check")
			}
		})
	})
}

func TestRegister(t *testing.T) {
	validCode := "123456"

	t.Run("Success", func(t *testing.T) {
		clearDatabase(context.Background())

		validEmail := fmt.Sprintf("regsuccess%d@example.com", time.Now().UnixNano())
		validUsername := fmt.Sprintf("reguser%d", time.Now().UnixNano())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(validEmail, validCode, time.Now().UTC().Add(5*time.Minute))

		reqBody := model.RegisterUserRequest{
			Email:        validEmail,
			Username:     validUsername,
			Code:         validCode,
			FullName:     "Test User",
			Password:     "Password123!",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataMap, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.Contains(t, dataMap, "token")

		userMap, ok := dataMap["user"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, validEmail, userMap["email"])
		assert.Equal(t, validUsername, userMap["username"])
		assert.Contains(t, userMap, "avatar")
	})

	t.Run("Success - Register with Whitespace", func(t *testing.T) {
		clearDatabase(context.Background())

		validEmail := fmt.Sprintf("  regspace%d@example.com  ", time.Now().UnixNano())
		validUsername := fmt.Sprintf("  regspace%d  ", time.Now().UnixNano())
		cleanEmail := strings.TrimSpace(validEmail)
		cleanUsername := strings.TrimSpace(validUsername)

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(cleanEmail, validCode, time.Now().UTC().Add(5*time.Minute))

		reqBody := model.RegisterUserRequest{
			Email:        validEmail,
			Username:     validUsername,
			Code:         validCode,
			FullName:     "  Test User  ",
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
		dataMap := resp.Data.(map[string]interface{})
		userMap := dataMap["user"].(map[string]interface{})

		assert.Equal(t, cleanEmail, userMap["email"])
		assert.Equal(t, cleanUsername, userMap["username"])
		assert.Equal(t, "Test User", userMap["full_name"])
	})

	t.Run("Username Already Taken (Case Insensitive)", func(t *testing.T) {
		clearDatabase(context.Background())
		u := createTestUser(t, "regtaken")

		validEmail := fmt.Sprintf("regnew%d@example.com", time.Now().UnixNano())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(validEmail, validCode, time.Now().UTC().Add(5*time.Minute))

		reqBody := model.RegisterUserRequest{
			Email:        validEmail,
			Username:     *u.Username,
			Code:         validCode,
			FullName:     "Test User",
			Password:     "Password123!",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusConflict, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Invalid Captcha", func(t *testing.T) {
		clearDatabase(context.Background())
		validEmail := fmt.Sprintf("regcaptcha%d@example.com", time.Now().UnixNano())
		validUsername := fmt.Sprintf("regcaptcha%d", time.Now().UnixNano())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysFails
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(validEmail, validCode, time.Now().UTC().Add(5*time.Minute))

		reqBody := model.RegisterUserRequest{
			Email:        validEmail,
			Username:     validUsername,
			Code:         validCode,
			FullName:     "Test User",
			Password:     "Password123!",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseError
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Equal(t, helper.MsgBadRequest, resp.Error)
	})

	t.Run("Invalid OTP", func(t *testing.T) {
		clearDatabase(context.Background())
		validEmail := fmt.Sprintf("regotp%d@example.com", time.Now().UnixNano())
		validUsername := fmt.Sprintf("regotp%d", time.Now().UnixNano())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(validEmail, validCode, time.Now().UTC().Add(5*time.Minute))

		reqBody := model.RegisterUserRequest{
			Email:        validEmail,
			Username:     validUsername,
			Code:         "000000",
			FullName:     "Test User",
			Password:     "Password123!",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Expired OTP", func(t *testing.T) {
		clearDatabase(context.Background())
		validEmail := fmt.Sprintf("regexpired%d@example.com", time.Now().UnixNano())
		validUsername := fmt.Sprintf("regexpired%d", time.Now().UnixNano())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(validEmail, validCode, time.Now().UTC().Add(-5*time.Minute))

		reqBody := model.RegisterUserRequest{
			Email:        validEmail,
			Username:     validUsername,
			Code:         validCode,
			FullName:     "Test User",
			Password:     "Password123!",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Email Already Registered", func(t *testing.T) {
		clearDatabase(context.Background())
		u := createTestUser(t, "regexist")
		validEmail := *u.Email
		validUsername := fmt.Sprintf("regnew%d", time.Now().UnixNano())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(validEmail, validCode, time.Now().UTC().Add(5*time.Minute))

		reqBody := model.RegisterUserRequest{
			Email:        validEmail,
			Username:     validUsername,
			Code:         validCode,
			FullName:     "Test User",
			Password:     "Password123!",
			CaptchaToken: dummyTurnstileToken,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusConflict, rr.Code) {
			printBody(t, rr)
		}
	})
}

func TestResetPassword(t *testing.T) {
	validCode := "123456"
	newPassword := "NewPassword123!"

	t.Run("Success", func(t *testing.T) {
		clearDatabase(context.Background())
		u := createTestUser(t, "resetsuccess")
		validEmail := *u.Email

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(validEmail, validCode, time.Now().UTC().Add(5*time.Minute))

		key := fmt.Sprintf("otp:%s:%s", "reset", validEmail)
		hashedCode := helper.HashOTP(validCode, testConfig.OTPSecret)
		redisAdapter.Set(context.Background(), key, hashedCode, 5*time.Minute)

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

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		u, _ = testClient.User.Query().Where(user.Email(validEmail)).Only(context.Background())
		assert.NotNil(t, u.PasswordHash)
		assert.True(t, helper.CheckPasswordHash(newPassword, *u.PasswordHash))
	})

	t.Run("User Not Found", func(t *testing.T) {
		clearDatabase(context.Background())
		validEmail := fmt.Sprintf("reset404%d@example.com", time.Now().UnixNano())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(validEmail, validCode, time.Now().UTC().Add(5*time.Minute))
		key := fmt.Sprintf("otp:%s:%s", "reset", validEmail)
		hashedCode := helper.HashOTP(validCode, testConfig.OTPSecret)
		redisAdapter.Set(context.Background(), key, hashedCode, 5*time.Minute)

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

		if !assert.Equal(t, http.StatusNotFound, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Invalid OTP", func(t *testing.T) {
		clearDatabase(context.Background())
		u := createTestUser(t, "resetotp")
		validEmail := *u.Email

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(validEmail, validCode, time.Now().UTC().Add(5*time.Minute))
		key := fmt.Sprintf("otp:%s:%s", "reset", validEmail)
		hashedCode := helper.HashOTP(validCode, testConfig.OTPSecret)
		redisAdapter.Set(context.Background(), key, hashedCode, 5*time.Minute)

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

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Password Mismatch", func(t *testing.T) {
		clearDatabase(context.Background())
		validEmail := fmt.Sprintf("resetmismatch%d@example.com", time.Now().UnixNano())

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

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Token Integrity", func(t *testing.T) {
		clearDatabase(context.Background())
		u := createTestUser(t, "tokenintegrity")

		t.Run("Expired Token", func(t *testing.T) {
			token, _ := helper.GenerateJWT(testConfig.JWTSecret, -1, u.ID)
			req, _ := http.NewRequest("GET", "/api/user/current", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rr := executeRequest(req)
			assert.Equal(t, http.StatusUnauthorized, rr.Code)
		})

		t.Run("Invalid Signature", func(t *testing.T) {
			token, _ := helper.GenerateJWT("wrong-secret", 3600, u.ID)
			req, _ := http.NewRequest("GET", "/api/user/current", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rr := executeRequest(req)
			assert.Equal(t, http.StatusUnauthorized, rr.Code)
		})

		t.Run("Deleted User", func(t *testing.T) {
			token, _ := helper.GenerateJWT(testConfig.JWTSecret, 3600, u.ID)
			testClient.User.DeleteOneID(u.ID).Exec(context.Background())
			req, _ := http.NewRequest("GET", "/api/user/current", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rr := executeRequest(req)
			assert.Equal(t, http.StatusUnauthorized, rr.Code)
		})
	})
}
