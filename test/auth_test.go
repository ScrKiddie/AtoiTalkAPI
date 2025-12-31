package test

import (
	"AtoiTalkAPI/ent/media"
	"AtoiTalkAPI/ent/otp"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/ent/useridentity"
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

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func TestLogin(t *testing.T) {
	validEmail := "login@example.com"
	validPassword := "Password123!"
	validUsername := "loginuser"

	t.Run("Success", func(t *testing.T) {
		clearDatabase(context.Background())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		hashedPassword, _ := helper.HashPassword(validPassword)
		testClient.User.Create().
			SetEmail(validEmail).
			SetUsername(validUsername).
			SetFullName("Login User").
			SetPasswordHash(hashedPassword).
			Save(context.Background())

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

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		hashedPassword, _ := helper.HashPassword(validPassword)
		testClient.User.Create().
			SetEmail(validEmail).
			SetUsername(validUsername).
			SetFullName("Login User").
			SetPasswordHash(hashedPassword).
			Save(context.Background())

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

	t.Run("Invalid Token", func(t *testing.T) {
		reqBody := model.GoogleLoginRequest{Code: "invalid-token-string"}
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

	t.Run("Valid Token", func(t *testing.T) {
		validToken := os.Getenv("TEST_GOOGLE_ID_TOKEN")
		if validToken == "" {
			t.Skip("Skipping Valid Token test: TEST_GOOGLE_ID_TOKEN not set")
		}

		token, _, err := new(jwt.Parser).ParseUnverified(validToken, jwt.MapClaims{})
		if err != nil {
			t.Fatalf("Failed to parse test token: %v", err)
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			t.Fatalf("Invalid token claims")
		}

		realEmail, _ := claims["email"].(string)
		realSub, _ := claims["sub"].(string)

		if realEmail == "" || realSub == "" {
			t.Skip("Skipping: Token does not contain email or sub")
		}

		makeRequest := func() *httptest.ResponseRecorder {
			reqBody := model.GoogleLoginRequest{Code: validToken}
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
			assert.Equal(t, realEmail, userMap["email"])
			assert.NotEmpty(t, userMap["username"])

			emailPrefix := strings.Split(realEmail, "@")[0]

			emailPrefix = helper.NormalizeUsername(emailPrefix)
			if len(emailPrefix) > 40 {
				emailPrefix = emailPrefix[:40]
			}
			if len(emailPrefix) < 3 {
				emailPrefix = "user" + emailPrefix
			}

			assert.True(t, strings.HasPrefix(userMap["username"].(string), emailPrefix))

			userID := int(userMap["id"].(float64))
			identity, err := testClient.UserIdentity.Query().
				Where(useridentity.UserID(userID)).
				Only(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, useridentity.ProviderGoogle, identity.Provider)
			assert.Equal(t, realSub, identity.ProviderID)

			if avatarURL, ok := userMap["avatar"].(string); ok && avatarURL != "" {
				parts := strings.Split(avatarURL, "/")
				fileName := parts[len(parts)-1]
				_, b, _, _ := runtime.Caller(0)
				testDir := filepath.Dir(b)
				physicalPath := filepath.Join(testDir, testConfig.StorageProfile, fileName)
				assert.FileExists(t, physicalPath, "Profile picture file should be created")

				m, err := testClient.Media.Query().Where(media.FileName(fileName)).Only(context.Background())
				assert.NoError(t, err)
				assert.Equal(t, userID, m.UploadedByID, "Media uploader should be set to the user")
			} else {
				t.Log("No avatar URL returned, skipping file check")
			}
		})

		t.Run("Login Existing User and Link Identity", func(t *testing.T) {

			clearDatabase(context.Background())
			u, _ := testClient.User.Create().
				SetEmail(realEmail).
				SetUsername("existinguser").
				SetFullName("Existing User").
				Save(context.Background())

			rr := makeRequest()
			if !assert.Equal(t, http.StatusOK, rr.Code, "Response body: %s", rr.Body.String()) {
				printBody(t, rr)
			}

			identity, err := testClient.UserIdentity.Query().
				Where(useridentity.UserID(u.ID)).
				Only(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, useridentity.ProviderGoogle, identity.Provider)
			assert.Equal(t, realSub, identity.ProviderID)
		})

		t.Run("Login Existing User with Existing Identity", func(t *testing.T) {

			clearDatabase(context.Background())
			u, _ := testClient.User.Create().
				SetEmail(realEmail).
				SetUsername("existinguser").
				SetFullName("Existing User").
				Save(context.Background())
			testClient.UserIdentity.Create().
				SetUserID(u.ID).
				SetProvider(useridentity.ProviderGoogle).
				SetProviderID(realSub).
				Save(context.Background())

			rr := makeRequest()
			if !assert.Equal(t, http.StatusOK, rr.Code, "Response body: %s", rr.Body.String()) {
				printBody(t, rr)
			}

			count, _ := testClient.UserIdentity.Query().
				Where(useridentity.UserID(u.ID)).
				Count(context.Background())
			assert.Equal(t, 1, count)
		})
	})
}

func TestRegister(t *testing.T) {

	validEmail := "test@example.com"
	validCode := "123456"
	validUsername := "testuser"

	t.Run("Success", func(t *testing.T) {
		clearDatabase(context.Background())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(validEmail, validCode, time.Now().Add(5*time.Minute))

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

	t.Run("Username Already Taken (Case Insensitive)", func(t *testing.T) {
		clearDatabase(context.Background())
		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		testClient.User.Create().
			SetEmail("other@example.com").
			SetUsername("scrkiddie").
			SetFullName("Other User").
			Save(context.Background())

		createOTP(validEmail, validCode, time.Now().Add(5*time.Minute))

		reqBody := model.RegisterUserRequest{
			Email:        validEmail,
			Username:     "scrkiddie",
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

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysFails
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(validEmail, validCode, time.Now().Add(5*time.Minute))

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

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP(validEmail, validCode, time.Now().Add(5*time.Minute))

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

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		createOTP("expired@example.com", validCode, time.Now().Add(-5*time.Minute))

		reqBody := model.RegisterUserRequest{
			Email:        "expired@example.com",
			Username:     "expireduser",
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

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		hashedPassword, _ := helper.HashPassword("Password123!")
		testClient.User.Create().
			SetEmail("existing@example.com").
			SetUsername("existinguser").
			SetFullName("Existing User").
			SetPasswordHash(hashedPassword).
			Save(context.Background())

		createOTP("existing@example.com", validCode, time.Now().Add(5*time.Minute))

		reqBody := model.RegisterUserRequest{
			Email:        "existing@example.com",
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
			SetUsername("resetuser").
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

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

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

		if !assert.Equal(t, http.StatusNotFound, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Invalid OTP", func(t *testing.T) {
		clearDatabase(context.Background())

		originalSecret := testConfig.TurnstileSecretKey
		testConfig.TurnstileSecretKey = cfTurnstileAlwaysPasses
		defer func() { testConfig.TurnstileSecretKey = originalSecret }()

		hashedPassword, _ := helper.HashPassword("OldPassword123!")
		testClient.User.Create().
			SetEmail(validEmail).
			SetUsername("resetuser").
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

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
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

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Token Integrity", func(t *testing.T) {
		clearDatabase(context.Background())
		u, _ := testClient.User.Create().SetEmail("token@test.com").SetUsername("tokenuser").SetFullName("Token User").Save(context.Background())

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
