package test

import (
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/ent/useridentity"
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

func TestChangePassword(t *testing.T) {
	validEmail := "changepass@example.com"
	oldPassword := "OldPassword123!"
	newPassword := "NewPassword123!"

	setupUser := func(password *string) (string, int) {
		clearDatabase(context.Background())

		create := testClient.User.Create().
			SetEmail(validEmail).
			SetFullName("Change Pass User")

		if password != nil {
			hashed, _ := helper.HashPassword(*password)
			create.SetPasswordHash(hashed)
		}

		u, _ := create.Save(context.Background())

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u.ID)
		return token, u.ID
	}

	t.Run("Success with Old Password", func(t *testing.T) {
		token, userID := setupUser(&oldPassword)

		reqBody := model.ChangePasswordRequest{
			OldPassword:     &oldPassword,
			NewPassword:     newPassword,
			ConfirmPassword: newPassword,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("PUT", "/api/account/password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		u, _ := testClient.User.Query().Where(user.ID(userID)).Only(context.Background())
		assert.True(t, helper.CheckPasswordHash(newPassword, *u.PasswordHash))
	})

	t.Run("Success without Old Password (DB password is null)", func(t *testing.T) {

		token, userID := setupUser(nil)

		reqBody := model.ChangePasswordRequest{
			OldPassword:     nil,
			NewPassword:     newPassword,
			ConfirmPassword: newPassword,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("PUT", "/api/account/password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		u, _ := testClient.User.Query().Where(user.ID(userID)).Only(context.Background())
		assert.NotNil(t, u.PasswordHash)
		assert.True(t, helper.CheckPasswordHash(newPassword, *u.PasswordHash))
	})

	t.Run("Fail: Old Password Required but Missing", func(t *testing.T) {
		token, _ := setupUser(&oldPassword)

		reqBody := model.ChangePasswordRequest{
			OldPassword:     nil,
			NewPassword:     newPassword,
			ConfirmPassword: newPassword,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("PUT", "/api/account/password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Fail: Old Password Incorrect", func(t *testing.T) {
		token, _ := setupUser(&oldPassword)

		wrongPass := "WrongPass123!"
		reqBody := model.ChangePasswordRequest{
			OldPassword:     &wrongPass,
			NewPassword:     newPassword,
			ConfirmPassword: newPassword,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("PUT", "/api/account/password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Fail: New Password Mismatch", func(t *testing.T) {
		token, _ := setupUser(&oldPassword)

		reqBody := model.ChangePasswordRequest{
			OldPassword:     &oldPassword,
			NewPassword:     newPassword,
			ConfirmPassword: "MismatchPassword123!",
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("PUT", "/api/account/password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Fail: Weak Password", func(t *testing.T) {
		token, _ := setupUser(&oldPassword)

		weakPass := "weak"
		reqBody := model.ChangePasswordRequest{
			OldPassword:     &oldPassword,
			NewPassword:     weakPass,
			ConfirmPassword: weakPass,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("PUT", "/api/account/password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Fail: Unauthorized (No Token)", func(t *testing.T) {
		reqBody := model.ChangePasswordRequest{
			OldPassword:     &oldPassword,
			NewPassword:     newPassword,
			ConfirmPassword: newPassword,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("PUT", "/api/account/password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusUnauthorized, rr.Code) {
			printBody(t, rr)
		}
	})
}

func TestChangeEmail(t *testing.T) {
	currentEmail := "current@example.com"
	newEmail := "new@example.com"
	validCode := "123456"

	setupUser := func(withPassword bool) (string, int) {
		clearDatabase(context.Background())

		create := testClient.User.Create().
			SetEmail(currentEmail).
			SetFullName("Change Email User")

		if withPassword {
			hashedPassword, _ := helper.HashPassword("Password123!")
			create.SetPasswordHash(hashedPassword)
		}

		u, _ := create.Save(context.Background())

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u.ID)
		return token, u.ID
	}

	createEmailOTP := func(email, code string) {
		hashedCode := helper.HashOTP(code, testConfig.OTPSecret)
		testClient.OTP.Create().
			SetEmail(email).
			SetCode(hashedCode).
			SetMode(constant.OTPModeChangeEmail).
			SetExpiresAt(time.Now().Add(5 * time.Minute)).
			Exec(context.Background())
	}

	t.Run("Success", func(t *testing.T) {
		token, userID := setupUser(true)
		createEmailOTP(newEmail, validCode)

		testClient.UserIdentity.Create().
			SetUserID(userID).
			SetProvider("google").
			SetProviderID("12345").
			Save(context.Background())

		reqBody := model.ChangeEmailRequest{
			Email: newEmail,
			Code:  validCode,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("PUT", "/api/account/email", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		u, _ := testClient.User.Query().Where(user.ID(userID)).Only(context.Background())
		assert.Equal(t, newEmail, u.Email)

		count, _ := testClient.UserIdentity.Query().Where(useridentity.UserID(userID)).Count(context.Background())
		assert.Equal(t, 0, count)
	})

	t.Run("Fail: No Password Set", func(t *testing.T) {
		token, _ := setupUser(false)
		createEmailOTP(newEmail, validCode)

		reqBody := model.ChangeEmailRequest{
			Email: newEmail,
			Code:  validCode,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("PUT", "/api/account/email", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}

		var resp map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Contains(t, resp["error"], "set a password")
	})

	t.Run("Fail: Same Email", func(t *testing.T) {
		token, _ := setupUser(true)

		reqBody := model.ChangeEmailRequest{
			Email: currentEmail,
			Code:  validCode,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("PUT", "/api/account/email", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}

		var resp map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Contains(t, resp["error"], "same as the current email")
	})

	t.Run("Fail: Invalid OTP", func(t *testing.T) {
		token, _ := setupUser(true)
		createEmailOTP(newEmail, validCode)

		reqBody := model.ChangeEmailRequest{
			Email: newEmail,
			Code:  "000000",
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("PUT", "/api/account/email", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Fail: Email Already Registered", func(t *testing.T) {
		token, _ := setupUser(true)
		createEmailOTP(newEmail, validCode)

		testClient.User.Create().
			SetEmail(newEmail).
			SetFullName("Existing User").
			Save(context.Background())

		reqBody := model.ChangeEmailRequest{
			Email: newEmail,
			Code:  validCode,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("PUT", "/api/account/email", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusConflict, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Fail: Unauthorized", func(t *testing.T) {
		reqBody := model.ChangeEmailRequest{
			Email: newEmail,
			Code:  validCode,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("PUT", "/api/account/email", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusUnauthorized, rr.Code) {
			printBody(t, rr)
		}
	})
}
