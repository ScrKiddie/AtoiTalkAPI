package test

import (
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/otp"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/ent/useridentity"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func generateUniqueEmail(prefix string) string {
	return fmt.Sprintf("%s_%d@example.com", prefix, time.Now().UnixNano())
}

func TestChangePassword(t *testing.T) {
	oldPassword := "OldPassword123!"
	newPassword := "NewPassword123!"

	setupUser := func(email string, password *string) (string, uuid.UUID) {
		clearDatabase(context.Background())

		username := "user" + strings.Split(email, "@")[0]
		username = strings.ReplaceAll(username, "_", "")
		username = strings.ReplaceAll(username, ".", "")

		create := testClient.User.Create().
			SetEmail(email).
			SetUsername(username).
			SetFullName("Change Pass User")

		if password != nil {
			hashed, _ := helper.HashPassword(*password)
			create.SetPasswordHash(hashed)
		}

		u, err := create.Save(context.Background())
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u.ID)
		return token, u.ID
	}

	t.Run("Success with Old Password", func(t *testing.T) {
		token, userID := setupUser(generateUniqueEmail("changepass1"), &oldPassword)

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
		token, userID := setupUser(generateUniqueEmail("changepass2"), nil)

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
		token, _ := setupUser(generateUniqueEmail("changepass3"), &oldPassword)

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
		token, _ := setupUser(generateUniqueEmail("changepass4"), &oldPassword)

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
		token, _ := setupUser(generateUniqueEmail("changepass5"), &oldPassword)

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
		token, _ := setupUser(generateUniqueEmail("changepass6"), &oldPassword)

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
	validCode := "123456"

	setupUser := func(email string, withPassword bool) (string, uuid.UUID) {
		clearDatabase(context.Background())

		username := "user" + strings.Split(email, "@")[0]
		username = strings.ReplaceAll(username, "_", "")
		username = strings.ReplaceAll(username, ".", "")

		create := testClient.User.Create().
			SetEmail(email).
			SetUsername(username).
			SetFullName("Change Email User")

		if withPassword {
			hashedPassword, _ := helper.HashPassword("Password123!")
			create.SetPasswordHash(hashedPassword)
		}

		u, err := create.Save(context.Background())
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u.ID)
		return token, u.ID
	}

	createEmailOTP := func(email, code string) {
		hashedCode := helper.HashOTP(code, testConfig.OTPSecret)
		testClient.OTP.Create().
			SetEmail(email).
			SetCode(hashedCode).
			SetMode(otp.ModeChangeEmail).
			SetExpiresAt(time.Now().UTC().Add(5 * time.Minute)).
			Exec(context.Background())
	}

	t.Run("Success", func(t *testing.T) {
		currentEmail := generateUniqueEmail("current1")
		newEmail := generateUniqueEmail("new1")
		token, userID := setupUser(currentEmail, true)
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
		assert.Equal(t, newEmail, *u.Email)

		count, _ := testClient.UserIdentity.Query().Where(useridentity.UserID(userID)).Count(context.Background())
		assert.Equal(t, 0, count)
	})

	t.Run("Success - Change Email with Whitespace", func(t *testing.T) {
		currentEmail := generateUniqueEmail("current_space")
		newEmail := generateUniqueEmail("new_space")
		cleanNewEmail := strings.TrimSpace(newEmail)
		token, userID := setupUser(currentEmail, true)
		createEmailOTP(cleanNewEmail, validCode)

		reqBody := model.ChangeEmailRequest{
			Email: "  " + newEmail + "  ",
			Code:  validCode,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("PUT", "/api/account/email", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		u, _ := testClient.User.Query().Where(user.ID(userID)).Only(context.Background())
		assert.Equal(t, cleanNewEmail, *u.Email)
	})

	t.Run("Fail: No Password Set", func(t *testing.T) {
		currentEmail := generateUniqueEmail("current2")
		newEmail := generateUniqueEmail("new2")
		token, _ := setupUser(currentEmail, false)
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
		currentEmail := generateUniqueEmail("current3")
		token, _ := setupUser(currentEmail, true)

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
	})

	t.Run("Fail: Invalid OTP", func(t *testing.T) {
		currentEmail := generateUniqueEmail("current4")
		newEmail := generateUniqueEmail("new4")
		token, _ := setupUser(currentEmail, true)
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
		currentEmail := generateUniqueEmail("current5")
		newEmail := generateUniqueEmail("new5")
		token, _ := setupUser(currentEmail, true)
		createEmailOTP(newEmail, validCode)

		testClient.User.Create().
			SetEmail(newEmail).
			SetUsername("existing" + strings.Split(newEmail, "@")[0]).
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
		newEmail := generateUniqueEmail("new6")
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

func TestDeleteAccount(t *testing.T) {
	password := "Password123!"

	setupUser := func(prefix string, withPassword bool) (string, uuid.UUID) {

		email := generateUniqueEmail(prefix)
		username := "user" + strings.Split(email, "@")[0]
		username = strings.ReplaceAll(username, "_", "")
		username = strings.ReplaceAll(username, ".", "")

		create := testClient.User.Create().
			SetEmail(email).
			SetUsername(username).
			SetFullName("Delete User")

		if withPassword {
			hashedPassword, _ := helper.HashPassword(password)
			create.SetPasswordHash(hashedPassword)
		}

		u, err := create.Save(context.Background())
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u.ID)
		return token, u.ID
	}

	t.Run("Success - Delete Account (No Password)", func(t *testing.T) {
		token, userID := setupUser("delete1", false)

		testClient.UserIdentity.Create().SetUserID(userID).SetProvider("google").SetProviderID("123").SaveX(context.Background())

		req, _ := http.NewRequest("DELETE", "/api/account", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		u, _ := testClient.User.Query().Where(user.ID(userID)).Only(context.Background())
		assert.NotNil(t, u.DeletedAt)
		assert.Nil(t, u.FullName)
		assert.Nil(t, u.Email)
		assert.Nil(t, u.Username)

		count, _ := testClient.UserIdentity.Query().Where(useridentity.UserID(userID)).Count(context.Background())
		assert.Equal(t, 0, count)
	})

	t.Run("Success - Delete Account (With Password)", func(t *testing.T) {
		token, userID := setupUser("delete2", true)

		reqBody := model.DeleteAccountRequest{Password: &password}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("DELETE", "/api/account", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		u, _ := testClient.User.Query().Where(user.ID(userID)).Only(context.Background())
		assert.NotNil(t, u.DeletedAt)
	})

	t.Run("Fail - Wrong Password", func(t *testing.T) {
		token, _ := setupUser("delete3", true)

		wrongPass := "wrong"
		reqBody := model.DeleteAccountRequest{Password: &wrongPass}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("DELETE", "/api/account", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Still Owner of Active Group", func(t *testing.T) {
		token, userID := setupUser("delete4", false)

		chatEntity := testClient.Chat.Create().SetType(chat.TypeGroup).SaveX(context.Background())
		gc := testClient.GroupChat.Create().SetChat(chatEntity).SetCreatorID(userID).SetName("My Group").SaveX(context.Background())
		testClient.GroupMember.Create().SetGroupChat(gc).SetUserID(userID).SetRole(groupmember.RoleOwner).SaveX(context.Background())

		req, _ := http.NewRequest("DELETE", "/api/account", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Success - Owner of Deleted Group (Should Allow)", func(t *testing.T) {
		token, userID := setupUser("delete5", false)

		chatEntity := testClient.Chat.Create().SetType(chat.TypeGroup).SetDeletedAt(time.Now().UTC()).SaveX(context.Background())
		gc := testClient.GroupChat.Create().SetChat(chatEntity).SetCreatorID(userID).SetName("Deleted Group").SaveX(context.Background())
		testClient.GroupMember.Create().SetGroupChat(gc).SetUserID(userID).SetRole(groupmember.RoleOwner).SaveX(context.Background())

		req, _ := http.NewRequest("DELETE", "/api/account", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}
