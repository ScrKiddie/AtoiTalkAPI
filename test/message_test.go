package test

import (
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/media"
	"AtoiTalkAPI/ent/privatechat"
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

func TestSendMessage(t *testing.T) {
	clearDatabase(context.Background())
	cleanupStorage(true)

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)

	u1, _ := testClient.User.Create().SetEmail("u1@test.com").SetFullName("User 1").SetPasswordHash(hashedPassword).Save(context.Background())
	u2, _ := testClient.User.Create().SetEmail("u2@test.com").SetFullName("User 2").SetPasswordHash(hashedPassword).Save(context.Background())
	u3, _ := testClient.User.Create().SetEmail("u3@test.com").SetFullName("User 3").SetPasswordHash(hashedPassword).Save(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token3, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u3.ID)

	chatEntity, _ := testClient.Chat.Create().SetType(chat.TypePrivate).Save(context.Background())
	testClient.PrivateChat.Create().SetChat(chatEntity).SetUser1(u1).SetUser2(u2).Save(context.Background())

	t.Run("Success - Send Text Message", func(t *testing.T) {
		reqBody := model.SendMessageRequest{
			ChatID:  chatEntity.ID,
			Content: "Hello User 2!",
			Type:    "text",
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataMap := resp.Data.(map[string]interface{})

		assert.Equal(t, float64(chatEntity.ID), dataMap["chat_id"])
		assert.Equal(t, float64(u1.ID), dataMap["sender_id"])
		assert.Equal(t, "Hello User 2!", dataMap["content"])
		assert.Empty(t, dataMap["attachments"])
	})

	t.Run("Success - Send Message with Attachment", func(t *testing.T) {

		m, _ := testClient.Media.Create().
			SetFileName("test_file.jpg").
			SetOriginalName("test.jpg").
			SetFileSize(1024).
			SetMimeType("image/jpeg").
			SetStatus(media.StatusActive).
			Save(context.Background())

		reqBody := model.SendMessageRequest{
			ChatID:        chatEntity.ID,
			Type:          "image",
			AttachmentIDs: []int{m.ID},
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataMap := resp.Data.(map[string]interface{})

		attachments := dataMap["attachments"].([]interface{})
		assert.Len(t, attachments, 1)
		att := attachments[0].(map[string]interface{})
		assert.Equal(t, float64(m.ID), att["id"])

		msgID := int(dataMap["id"].(float64))
		msg, _ := testClient.Message.Get(context.Background(), msgID)
		attachedMedia, _ := msg.QueryAttachments().Only(context.Background())
		assert.Equal(t, m.ID, attachedMedia.ID)
	})

	t.Run("Success - hidden_at Reset", func(t *testing.T) {

		pc, _ := testClient.PrivateChat.Query().Where(privatechat.ChatID(chatEntity.ID)).Only(context.Background())
		testClient.PrivateChat.UpdateOne(pc).SetUser1HiddenAt(time.Now()).Exec(context.Background())

		reqBody := model.SendMessageRequest{
			ChatID:  chatEntity.ID,
			Content: "I'm back!",
			Type:    "text",
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		pcUpdated, _ := testClient.PrivateChat.Query().Where(privatechat.ChatID(chatEntity.ID)).Only(context.Background())
		assert.Nil(t, pcUpdated.User1HiddenAt)
		assert.NotNil(t, pcUpdated.User1LastReadAt)
	})

	t.Run("Success - Group Chat", func(t *testing.T) {

		groupChat, _ := testClient.Chat.Create().SetType(chat.TypeGroup).Save(context.Background())
		gc, _ := testClient.GroupChat.Create().SetChat(groupChat).SetCreator(u1).SetName("Test Group").Save(context.Background())
		testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u1).Save(context.Background())
		testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u2).Save(context.Background())

		reqBody := model.SendMessageRequest{
			ChatID:  groupChat.ID,
			Content: "Hello Group!",
			Type:    "text",
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Fail - Group Non-Member", func(t *testing.T) {

		groupChat, _ := testClient.Chat.Create().SetType(chat.TypeGroup).Save(context.Background())
		gc, _ := testClient.GroupChat.Create().SetChat(groupChat).SetCreator(u1).SetName("Secret Group").Save(context.Background())
		testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u1).Save(context.Background())
		testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u2).Save(context.Background())

		reqBody := model.SendMessageRequest{
			ChatID:  groupChat.ID,
			Content: "Let me in!",
			Type:    "text",
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token3)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Fail - Pending Media Rejected", func(t *testing.T) {

		m, _ := testClient.Media.Create().
			SetFileName("pending.jpg").
			SetOriginalName("pending.jpg").
			SetFileSize(1024).
			SetMimeType("image/jpeg").
			SetStatus(media.StatusPending).
			Save(context.Background())

		reqBody := model.SendMessageRequest{
			ChatID:        chatEntity.ID,
			Type:          "image",
			AttachmentIDs: []int{m.ID},
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Avatar Media Rejected", func(t *testing.T) {

		m, _ := testClient.Media.Create().
			SetFileName("avatar.jpg").
			SetOriginalName("avatar.jpg").
			SetFileSize(1024).
			SetMimeType("image/jpeg").
			SetStatus(media.StatusActive).
			Save(context.Background())
		testClient.User.UpdateOne(u1).SetAvatar(m).Exec(context.Background())

		reqBody := model.SendMessageRequest{
			ChatID:        chatEntity.ID,
			Type:          "image",
			AttachmentIDs: []int{m.ID},
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Attachment Already Used", func(t *testing.T) {

		otherMsg, _ := testClient.Message.Create().SetChatID(chatEntity.ID).SetSenderID(u1.ID).Save(context.Background())
		m, _ := testClient.Media.Create().
			SetFileName("used.jpg").
			SetOriginalName("used.jpg").
			SetFileSize(1024).
			SetMimeType("image/jpeg").
			SetStatus(media.StatusActive).
			SetMessage(otherMsg).
			Save(context.Background())

		reqBody := model.SendMessageRequest{
			ChatID:        chatEntity.ID,
			Type:          "image",
			AttachmentIDs: []int{m.ID},
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Fail - Attachment Not Found", func(t *testing.T) {
		reqBody := model.SendMessageRequest{
			ChatID:        chatEntity.ID,
			Type:          "image",
			AttachmentIDs: []int{99999},
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Not Member (Forbidden)", func(t *testing.T) {

		reqBody := model.SendMessageRequest{
			ChatID:  chatEntity.ID,
			Content: "Intruder!",
			Type:    "text",
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token3)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusForbidden, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Fail - Chat Not Found", func(t *testing.T) {
		reqBody := model.SendMessageRequest{
			ChatID:  99999,
			Content: "Hello?",
			Type:    "text",
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Fail - Validation Error (No Content & No Attachment)", func(t *testing.T) {
		reqBody := model.SendMessageRequest{
			ChatID: chatEntity.ID,
			Type:   "text",
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}
