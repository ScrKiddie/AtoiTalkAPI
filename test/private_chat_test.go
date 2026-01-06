package test

import (
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreatePrivateChat(t *testing.T) {

	user1Email := "user1@example.com"
	user2Email := "user2@example.com"
	password := "Password123!"

	t.Run("Success", func(t *testing.T) {
		clearDatabase(context.Background())

		hashedPassword, _ := helper.HashPassword(password)
		u1 := testClient.User.Create().
			SetEmail(user1Email).
			SetUsername("user1").
			SetFullName("User One").
			SetPasswordHash(hashedPassword).
			SaveX(context.Background())

		u2 := testClient.User.Create().
			SetEmail(user2Email).
			SetUsername("user2").
			SetFullName("User Two").
			SetPasswordHash(hashedPassword).
			SaveX(context.Background())

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

		reqBody := model.CreatePrivateChatRequest{
			TargetUserID: u2.ID,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/chats/private", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataMap, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "private", dataMap["type"])
		assert.NotEmpty(t, dataMap["id"])
	})

	t.Run("Fail if Blocked", func(t *testing.T) {
		clearDatabase(context.Background())

		u1 := testClient.User.Create().SetEmail("u1@test.com").SetUsername("u1").SetFullName("User 1").SaveX(context.Background())
		u2 := testClient.User.Create().SetEmail("u2@test.com").SetUsername("u2").SetFullName("User 2").SaveX(context.Background())

		testClient.UserBlock.Create().SetBlockerID(u1.ID).SetBlockedID(u2.ID).SaveX(context.Background())

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

		reqBody := model.CreatePrivateChatRequest{TargetUserID: u2.ID}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/chats/private", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Chat Already Exists", func(t *testing.T) {

		clearDatabase(context.Background())

		hashedPassword, _ := helper.HashPassword(password)
		u1 := testClient.User.Create().
			SetEmail(user1Email).
			SetUsername("user1").
			SetFullName("User One").
			SetPasswordHash(hashedPassword).
			SaveX(context.Background())

		u2 := testClient.User.Create().
			SetEmail(user2Email).
			SetUsername("user2").
			SetFullName("User Two").
			SetPasswordHash(hashedPassword).
			SaveX(context.Background())

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

		reqBody := model.CreatePrivateChatRequest{TargetUserID: u2.ID}
		body, _ := json.Marshal(reqBody)
		req1, _ := http.NewRequest("POST", "/api/chats/private", bytes.NewBuffer(body))
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("Authorization", "Bearer "+token)
		rr1 := executeRequest(req1)
		assert.Equal(t, http.StatusOK, rr1.Code)

		var resp1 helper.ResponseSuccess
		json.Unmarshal(rr1.Body.Bytes(), &resp1)
		dataMap1 := resp1.Data.(map[string]interface{})
		chatID1 := dataMap1["id"]

		req2, _ := http.NewRequest("POST", "/api/chats/private", bytes.NewBuffer(body))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Authorization", "Bearer "+token)
		rr2 := executeRequest(req2)

		if !assert.Equal(t, http.StatusOK, rr2.Code) {
			printBody(t, rr2)
		}

		var resp2 helper.ResponseSuccess
		json.Unmarshal(rr2.Body.Bytes(), &resp2)
		dataMap2 := resp2.Data.(map[string]interface{})
		chatID2 := dataMap2["id"]

		assert.Equal(t, chatID1, chatID2)
	})

	t.Run("Target User Not Found", func(t *testing.T) {
		clearDatabase(context.Background())

		hashedPassword, _ := helper.HashPassword(password)
		u1 := testClient.User.Create().
			SetEmail(user1Email).
			SetUsername("user1").
			SetFullName("User One").
			SetPasswordHash(hashedPassword).
			SaveX(context.Background())

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

		reqBody := model.CreatePrivateChatRequest{
			TargetUserID: 99999,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/chats/private", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusNotFound, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Chat With Self", func(t *testing.T) {
		clearDatabase(context.Background())

		hashedPassword, _ := helper.HashPassword(password)
		u1 := testClient.User.Create().
			SetEmail(user1Email).
			SetUsername("user1").
			SetFullName("User One").
			SetPasswordHash(hashedPassword).
			SaveX(context.Background())

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

		reqBody := model.CreatePrivateChatRequest{
			TargetUserID: u1.ID,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/chats/private", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Unauthorized", func(t *testing.T) {
		reqBody := model.CreatePrivateChatRequest{TargetUserID: 1}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/chats/private", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusUnauthorized, rr.Code) {
			printBody(t, rr)
		}
	})
}

func TestCreatePrivateChat_ReverseOrder(t *testing.T) {

	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)

	u1 := testClient.User.Create().SetEmail("u1@test.com").SetUsername("u1").SetFullName("U1").SetPasswordHash(hashedPassword).SaveX(context.Background())
	u2 := testClient.User.Create().SetEmail("u2@test.com").SetUsername("u2").SetFullName("U2").SetPasswordHash(hashedPassword).SaveX(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u2.ID)

	reqBody1 := model.CreatePrivateChatRequest{TargetUserID: u2.ID}
	body1, _ := json.Marshal(reqBody1)
	req1, _ := http.NewRequest("POST", "/api/chats/private", bytes.NewBuffer(body1))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+token1)
	rr1 := executeRequest(req1)
	assert.Equal(t, http.StatusOK, rr1.Code)

	var resp1 helper.ResponseSuccess
	json.Unmarshal(rr1.Body.Bytes(), &resp1)
	chatID1 := resp1.Data.(map[string]interface{})["id"]

	reqBody2 := model.CreatePrivateChatRequest{TargetUserID: u1.ID}
	body2, _ := json.Marshal(reqBody2)
	req2, _ := http.NewRequest("POST", "/api/chats/private", bytes.NewBuffer(body2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+token2)
	rr2 := executeRequest(req2)

	if !assert.Equal(t, http.StatusOK, rr2.Code) {
		printBody(t, rr2)
	}

	var resp2 helper.ResponseSuccess
	json.Unmarshal(rr2.Body.Bytes(), &resp2)
	chatID2 := resp2.Data.(map[string]interface{})["id"]

	assert.Equal(t, chatID1, chatID2, "Chat ID should be the same regardless of who initiated it")
}

func TestCreatePrivateChat_Validation(t *testing.T) {
	clearDatabase(context.Background())
	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)
	u1 := testClient.User.Create().SetEmail("u1@test.com").SetUsername("u1").SetFullName("U1").SetPasswordHash(hashedPassword).SaveX(context.Background())
	token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

	t.Run("Missing TargetUserID", func(t *testing.T) {

		reqBody := map[string]interface{}{}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/chats/private", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/api/chats/private", bytes.NewBuffer([]byte("invalid-json")))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})
}
