package test

import (
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/privatechat"
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

func TestCreatePrivateChat(t *testing.T) {

	user1Email := "user1@example.com"
	user2Email := "user2@example.com"
	password := "Password123!"

	t.Run("Success", func(t *testing.T) {
		clearDatabase(context.Background())

		hashedPassword, _ := helper.HashPassword(password)
		u1 := testClient.User.Create().
			SetEmail(user1Email).
			SetFullName("User One").
			SetPasswordHash(hashedPassword).
			SaveX(context.Background())

		u2 := testClient.User.Create().
			SetEmail(user2Email).
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

	t.Run("Chat Already Exists", func(t *testing.T) {

		clearDatabase(context.Background())

		hashedPassword, _ := helper.HashPassword(password)
		u1 := testClient.User.Create().
			SetEmail(user1Email).
			SetFullName("User One").
			SetPasswordHash(hashedPassword).
			SaveX(context.Background())

		u2 := testClient.User.Create().
			SetEmail(user2Email).
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

	u1 := testClient.User.Create().SetEmail("u1@test.com").SetFullName("U1").SetPasswordHash(hashedPassword).SaveX(context.Background())
	u2 := testClient.User.Create().SetEmail("u2@test.com").SetFullName("U2").SetPasswordHash(hashedPassword).SaveX(context.Background())

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
	u1 := testClient.User.Create().SetEmail("u1@test.com").SetFullName("U1").SetPasswordHash(hashedPassword).SaveX(context.Background())
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

func TestGetChats(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)
	u1 := testClient.User.Create().SetEmail("u1@test.com").SetFullName("User 1").SetPasswordHash(hashedPassword).SaveX(context.Background())
	u2 := testClient.User.Create().SetEmail("u2@test.com").SetFullName("User 2").SetPasswordHash(hashedPassword).SaveX(context.Background())
	u3 := testClient.User.Create().SetEmail("u3@test.com").SetFullName("User 3").SetPasswordHash(hashedPassword).SaveX(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

	chat1 := testClient.Chat.Create().SetType(chat.TypePrivate).SetUpdatedAt(time.Now().Add(-2 * time.Hour)).SaveX(context.Background())

	testClient.PrivateChat.Create().SetChat(chat1).SetUser1(u1).SetUser2(u2).SetUser1UnreadCount(3).SaveX(context.Background())
	testClient.Message.Create().SetChat(chat1).SetSender(u2).SetContent("Old message").SetCreatedAt(time.Now().Add(-2 * time.Hour)).SaveX(context.Background())

	chat2 := testClient.Chat.Create().SetType(chat.TypePrivate).SetUpdatedAt(time.Now().Add(-1 * time.Hour)).SaveX(context.Background())
	testClient.PrivateChat.Create().SetChat(chat2).SetUser1(u1).SetUser2(u3).SetUser1UnreadCount(0).SaveX(context.Background())
	testClient.Message.Create().SetChat(chat2).SetSender(u3).SetContent("New message").SetCreatedAt(time.Now().Add(-1 * time.Hour)).SaveX(context.Background())

	chat3 := testClient.Chat.Create().SetType(chat.TypeGroup).SetUpdatedAt(time.Now()).SaveX(context.Background())
	gc := testClient.GroupChat.Create().SetChat(chat3).SetCreator(u1).SetName("My Group").SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u1).SetUnreadCount(5).SaveX(context.Background())
	testClient.Message.Create().SetChat(chat3).SetSender(u1).SetContent("Group message").SetCreatedAt(time.Now()).SaveX(context.Background())

	t.Run("Success - List All Chats", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/chats", nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})

		assert.Len(t, dataList, 3)

		c1 := dataList[0].(map[string]interface{})
		c2 := dataList[1].(map[string]interface{})
		c3 := dataList[2].(map[string]interface{})

		assert.Equal(t, float64(chat3.ID), c1["id"])
		assert.Equal(t, "My Group", c1["name"])
		assert.Equal(t, float64(5), c1["unread_count"])

		assert.Equal(t, float64(chat2.ID), c2["id"])
		assert.Equal(t, "User 3", c2["name"])
		assert.Equal(t, float64(0), c2["unread_count"])

		assert.Equal(t, float64(chat1.ID), c3["id"])
		assert.Equal(t, "User 2", c3["name"])
		assert.Equal(t, float64(3), c3["unread_count"])
	})

	t.Run("Success - Pagination", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/chats?limit=2", nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 2)
		assert.True(t, resp.Meta.HasNext)
		assert.NotEmpty(t, resp.Meta.NextCursor)

		cursor := resp.Meta.NextCursor
		req2, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats?limit=2&cursor=%s", cursor), nil)
		req2.Header.Set("Authorization", "Bearer "+token1)
		rr2 := executeRequest(req2)

		if !assert.Equal(t, http.StatusOK, rr2.Code) {
			printBody(t, rr2)
		}
		var resp2 helper.ResponseWithPagination
		json.Unmarshal(rr2.Body.Bytes(), &resp2)
		dataList2 := resp2.Data.([]interface{})
		assert.Len(t, dataList2, 1)
		assert.Equal(t, float64(chat1.ID), dataList2[0].(map[string]interface{})["id"])
	})

	t.Run("Success - Search", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/chats?query=Group", nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 1)
		assert.Equal(t, "My Group", dataList[0].(map[string]interface{})["name"])
	})

	t.Run("Success - Search No Results", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/chats?query=NamaNgawur123", nil)
		req.Header.Set("Authorization", "Bearer "+token1)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)
		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Empty(t, resp.Data)
	})

	t.Run("Fail - Invalid Cursor Format", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/chats?cursor=bukan-base64-valid", nil)
		req.Header.Set("Authorization", "Bearer "+token1)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Success - Last Message Placeholder for Deleted Message", func(t *testing.T) {

		u4 := testClient.User.Create().SetEmail("u4@test.com").SetFullName("User 4").SetPasswordHash(hashedPassword).SaveX(context.Background())

		delChat := testClient.Chat.Create().SetType(chat.TypePrivate).SaveX(context.Background())
		testClient.PrivateChat.Create().SetChat(delChat).SetUser1(u1).SetUser2(u4).SaveX(context.Background())

		msg := testClient.Message.Create().SetChat(delChat).SetSender(u4).SetContent("This will be deleted").SaveX(context.Background())
		testClient.Message.UpdateOne(msg).SetDeletedAt(time.Now()).ExecX(context.Background())

		delChat.Update().SetUpdatedAt(time.Now()).ExecX(context.Background())

		req, _ := http.NewRequest("GET", "/api/chats", nil)
		req.Header.Set("Authorization", "Bearer "+token1)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})

		topChat := dataList[0].(map[string]interface{})
		assert.Equal(t, float64(delChat.ID), topChat["id"])

		lastMsg, ok := topChat["last_message"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "Pesan telah dihapus", lastMsg["content"])
	})

	t.Run("Success - Exclude Hidden Private Chat", func(t *testing.T) {

		pc, _ := testClient.PrivateChat.Query().Where(privatechat.ChatID(chat1.ID)).Only(context.Background())
		testClient.PrivateChat.UpdateOne(pc).SetUser1HiddenAt(time.Now()).ExecX(context.Background())

		req, _ := http.NewRequest("GET", "/api/chats", nil)
		req.Header.Set("Authorization", "Bearer "+token1)
		rr := executeRequest(req)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})

		for _, item := range dataList {
			c := item.(map[string]interface{})
			assert.NotEqual(t, float64(chat1.ID), c["id"], "Hidden chat should not appear")
		}
	})

	t.Run("Unauthorized", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/chats", nil)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestMarkAsRead(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)
	u1 := testClient.User.Create().SetEmail("u1@test.com").SetFullName("User 1").SetPasswordHash(hashedPassword).SaveX(context.Background())
	u2 := testClient.User.Create().SetEmail("u2@test.com").SetFullName("User 2").SetPasswordHash(hashedPassword).SaveX(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

	chat1 := testClient.Chat.Create().SetType(chat.TypePrivate).SaveX(context.Background())
	testClient.PrivateChat.Create().SetChat(chat1).SetUser1(u1).SetUser2(u2).SetUser1UnreadCount(5).SaveX(context.Background())

	chat2 := testClient.Chat.Create().SetType(chat.TypeGroup).SaveX(context.Background())
	gc := testClient.GroupChat.Create().SetChat(chat2).SetCreator(u2).SetName("Test Group").SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u1).SetUnreadCount(10).SaveX(context.Background())

	t.Run("Success - Mark Private Chat Read", func(t *testing.T) {
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/%d/read", chat1.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		pc, _ := testClient.PrivateChat.Query().Where(privatechat.ChatID(chat1.ID)).Only(context.Background())
		assert.Equal(t, 0, pc.User1UnreadCount)
		assert.NotNil(t, pc.User1LastReadAt)
	})

	t.Run("Success - Mark Group Chat Read", func(t *testing.T) {
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/%d/read", chat2.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		gm, _ := testClient.GroupMember.Query().Where(groupmember.GroupChatID(gc.ID), groupmember.UserID(u1.ID)).Only(context.Background())
		assert.Equal(t, 0, gm.UnreadCount)
		assert.NotNil(t, gm.LastReadAt)
	})

	t.Run("Not Found", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/api/chats/99999/read", nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}
