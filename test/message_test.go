package test

import (
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/media"
	"AtoiTalkAPI/ent/privatechat"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSendMessage(t *testing.T) {
	clearDatabase(context.Background())
	cleanupStorage(true)

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)

	u1, _ := testClient.User.Create().SetEmail("u1@test.com").SetUsername("u1").SetFullName("User 1").SetPasswordHash(hashedPassword).Save(context.Background())
	u2, _ := testClient.User.Create().SetEmail("u2@test.com").SetUsername("u2").SetFullName("User 2").SetPasswordHash(hashedPassword).Save(context.Background())
	u3, _ := testClient.User.Create().SetEmail("u3@test.com").SetUsername("u3").SetFullName("User 3").SetPasswordHash(hashedPassword).Save(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token3, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u3.ID)

	chatEntity, _ := testClient.Chat.Create().SetType(chat.TypePrivate).Save(context.Background())
	testClient.PrivateChat.Create().SetChat(chatEntity).SetUser1(u1).SetUser2(u2).Save(context.Background())

	t.Run("Success - Send Text Message", func(t *testing.T) {
		reqBody := model.SendMessageRequest{
			ChatID:  chatEntity.ID,
			Content: "Hello User 2!",
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
		assert.Nil(t, dataMap["attachments"])

		pc, _ := testClient.PrivateChat.Query().Where(privatechat.ChatID(chatEntity.ID)).Only(context.Background())
		assert.Equal(t, 0, pc.User1UnreadCount)
		assert.Equal(t, 1, pc.User2UnreadCount)
	})

	t.Run("Fail - Sender is Blocked", func(t *testing.T) {

		testClient.UserBlock.Create().SetBlockerID(u2.ID).SetBlockedID(u1.ID).Exec(context.Background())

		reqBody := model.SendMessageRequest{
			ChatID:  chatEntity.ID,
			Content: "This message should be blocked",
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)

		testClient.UserBlock.Delete().Exec(context.Background())
	})

	t.Run("Success - Send Message with Reply", func(t *testing.T) {

		msg1, _ := testClient.Message.Create().SetChatID(chatEntity.ID).SetSenderID(u2.ID).SetContent("Original Message").Save(context.Background())

		replyToID := msg1.ID
		reqBody := model.SendMessageRequest{
			ChatID:    chatEntity.ID,
			Content:   "This is a reply",
			ReplyToID: &replyToID,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataMap := resp.Data.(map[string]interface{})

		replyToMap, ok := dataMap["reply_to"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, float64(msg1.ID), replyToMap["id"])
		assert.Equal(t, "User 2", replyToMap["sender_name"])
		assert.Equal(t, "Original Message", replyToMap["content"])

		msg2ID := int(dataMap["id"].(float64))
		msg2, _ := testClient.Message.Get(context.Background(), msg2ID)
		assert.Equal(t, msg1.ID, *msg2.ReplyToID)
	})

	t.Run("Fail - Reply to Non-Existent Message", func(t *testing.T) {
		replyToID := 99999
		reqBody := model.SendMessageRequest{
			ChatID:    chatEntity.ID,
			Content:   "Reply to ghost",
			ReplyToID: &replyToID,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Reply to Message in Different Chat", func(t *testing.T) {

		chat2, _ := testClient.Chat.Create().SetType(chat.TypePrivate).Save(context.Background())
		testClient.PrivateChat.Create().SetChat(chat2).SetUser1(u1).SetUser2(u3).Save(context.Background())
		msgOther, _ := testClient.Message.Create().SetChatID(chat2.ID).SetSenderID(u3.ID).SetContent("Other Chat Msg").Save(context.Background())

		replyToID := msgOther.ID
		reqBody := model.SendMessageRequest{
			ChatID:    chatEntity.ID,
			Content:   "Cross-chat reply",
			ReplyToID: &replyToID,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Success - Send Message with Attachment", func(t *testing.T) {

		m, _ := testClient.Media.Create().
			SetFileName("test_file.jpg").
			SetOriginalName("test.jpg").
			SetFileSize(1024).
			SetMimeType("image/jpeg").
			SetStatus(media.StatusActive).
			SetUploaderID(u1.ID).
			Save(context.Background())

		reqBody := model.SendMessageRequest{
			ChatID:        chatEntity.ID,
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

		pc, _ := testClient.PrivateChat.Query().Where(privatechat.ChatID(chatEntity.ID)).Only(context.Background())
		assert.Equal(t, 0, pc.User1UnreadCount)
		assert.Equal(t, 3, pc.User2UnreadCount)
	})

	t.Run("Fail - Attachment Belongs to Another User", func(t *testing.T) {

		m, _ := testClient.Media.Create().
			SetFileName("user2_file.jpg").
			SetOriginalName("user2.jpg").
			SetFileSize(1024).
			SetMimeType("image/jpeg").
			SetStatus(media.StatusActive).
			SetUploaderID(u2.ID).
			Save(context.Background())

		reqBody := model.SendMessageRequest{
			ChatID:        chatEntity.ID,
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

	t.Run("Fail - Attachment Already Used", func(t *testing.T) {

		otherMsg, _ := testClient.Message.Create().SetChatID(chatEntity.ID).SetSenderID(u1.ID).Save(context.Background())
		m, _ := testClient.Media.Create().
			SetFileName("used.jpg").
			SetOriginalName("used.jpg").
			SetFileSize(1024).
			SetMimeType("image/jpeg").
			SetStatus(media.StatusActive).
			SetMessage(otherMsg).
			SetUploaderID(u1.ID).
			Save(context.Background())

		reqBody := model.SendMessageRequest{
			ChatID:        chatEntity.ID,
			AttachmentIDs: []int{m.ID},
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Pending Media Rejected", func(t *testing.T) {
		m, _ := testClient.Media.Create().
			SetFileName("pending.jpg").
			SetOriginalName("pending.jpg").
			SetFileSize(1024).
			SetMimeType("image/jpeg").
			SetStatus(media.StatusPending).
			SetUploaderID(u1.ID).
			Save(context.Background())

		reqBody := model.SendMessageRequest{
			ChatID:        chatEntity.ID,
			AttachmentIDs: []int{m.ID},
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Success - SendMessage does not unhide", func(t *testing.T) {
		pc, _ := testClient.PrivateChat.Query().Where(privatechat.ChatID(chatEntity.ID)).Only(context.Background())
		hiddenTime := time.Now().Add(-time.Hour)
		testClient.PrivateChat.UpdateOne(pc).SetUser1HiddenAt(hiddenTime).Exec(context.Background())

		reqBody := model.SendMessageRequest{
			ChatID:  chatEntity.ID,
			Content: "This should not unhide",
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		pcUpdated, _ := testClient.PrivateChat.Query().Where(privatechat.ChatID(chatEntity.ID)).Only(context.Background())
		assert.NotNil(t, pcUpdated.User1HiddenAt)
		assert.WithinDuration(t, hiddenTime, *pcUpdated.User1HiddenAt, time.Second)
	})

	t.Run("Success - Group Chat (Multiple Recipients)", func(t *testing.T) {

		groupChat, _ := testClient.Chat.Create().SetType(chat.TypeGroup).Save(context.Background())
		gc, _ := testClient.GroupChat.Create().SetChat(groupChat).SetCreator(u1).SetName("Test Group").Save(context.Background())
		testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u1).Save(context.Background())
		testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u2).Save(context.Background())
		testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u3).Save(context.Background())

		reqBody := model.SendMessageRequest{
			ChatID:  groupChat.ID,
			Content: "Hello Group!",
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		gm1, _ := testClient.GroupMember.Query().Where(groupmember.GroupChatID(gc.ID), groupmember.UserID(u1.ID)).Only(context.Background())
		gm2, _ := testClient.GroupMember.Query().Where(groupmember.GroupChatID(gc.ID), groupmember.UserID(u2.ID)).Only(context.Background())
		gm3, _ := testClient.GroupMember.Query().Where(groupmember.GroupChatID(gc.ID), groupmember.UserID(u3.ID)).Only(context.Background())

		assert.Equal(t, 0, gm1.UnreadCount)
		assert.Equal(t, 1, gm2.UnreadCount)
		assert.Equal(t, 1, gm3.UnreadCount)
	})

	t.Run("Fail - Group Non-Member", func(t *testing.T) {

		groupChat, _ := testClient.Chat.Create().SetType(chat.TypeGroup).Save(context.Background())
		gc, _ := testClient.GroupChat.Create().SetChat(groupChat).SetCreator(u1).SetName("Secret Group").Save(context.Background())
		testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u1).Save(context.Background())
		testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u2).Save(context.Background())

		reqBody := model.SendMessageRequest{
			ChatID:  groupChat.ID,
			Content: "Let me in!",
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token3)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Fail - Attachment Not Found", func(t *testing.T) {
		reqBody := model.SendMessageRequest{
			ChatID:        chatEntity.ID,
			AttachmentIDs: []int{99999},
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Chat Not Found", func(t *testing.T) {
		reqBody := model.SendMessageRequest{
			ChatID:  99999,
			Content: "Hello?",
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
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestGetMessages(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)
	u1, _ := testClient.User.Create().SetEmail("u1@test.com").SetUsername("u1").SetFullName("User 1").SetPasswordHash(hashedPassword).Save(context.Background())
	u2, _ := testClient.User.Create().SetEmail("u2@test.com").SetUsername("u2").SetFullName("User 2").SetPasswordHash(hashedPassword).Save(context.Background())
	u3, _ := testClient.User.Create().SetEmail("u3@test.com").SetUsername("u3").SetFullName("User 3").SetPasswordHash(hashedPassword).Save(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token3, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u3.ID)

	chatEntity, _ := testClient.Chat.Create().SetType(chat.TypePrivate).Save(context.Background())
	testClient.PrivateChat.Create().SetChat(chatEntity).SetUser1(u1).SetUser2(u2).Save(context.Background())

	var msgIDs []int
	for i := 1; i <= 5; i++ {
		msg, _ := testClient.Message.Create().
			SetChatID(chatEntity.ID).
			SetSenderID(u1.ID).
			SetContent(fmt.Sprintf("Message %d", i)).
			Save(context.Background())
		msgIDs = append(msgIDs, msg.ID)
		time.Sleep(10 * time.Millisecond)
	}

	t.Run("Success - Get Messages (First Page)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/%d/messages?limit=2", chatEntity.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})

		assert.Len(t, dataList, 2)

		m1 := dataList[0].(map[string]interface{})
		m2 := dataList[1].(map[string]interface{})

		assert.Equal(t, float64(msgIDs[3]), m1["id"])
		assert.Equal(t, float64(msgIDs[4]), m2["id"])

		assert.True(t, resp.Meta.HasNext)
		assert.NotEmpty(t, resp.Meta.NextCursor)
	})

	t.Run("Success - Get Messages (Next Page)", func(t *testing.T) {

		cursor := base64.URLEncoding.EncodeToString([]byte(strconv.Itoa(msgIDs[3])))

		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/%d/messages?limit=2&cursor=%s", chatEntity.ID, cursor), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})

		assert.Len(t, dataList, 2)

		m1 := dataList[0].(map[string]interface{})
		m2 := dataList[1].(map[string]interface{})

		assert.Equal(t, float64(msgIDs[1]), m1["id"])
		assert.Equal(t, float64(msgIDs[2]), m2["id"])
	})

	t.Run("Success - Empty Chat", func(t *testing.T) {
		emptyChat, _ := testClient.Chat.Create().SetType(chat.TypePrivate).Save(context.Background())

		testClient.PrivateChat.Create().SetChat(emptyChat).SetUser1(u1).SetUser2(u3).Save(context.Background())

		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/%d/messages", emptyChat.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Empty(t, resp.Data)
	})

	t.Run("Fail - Not Member (Forbidden)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/%d/messages", chatEntity.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token3)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Fail - Chat Not Found", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/chats/99999/messages", nil)
		req.Header.Set("Authorization", "Bearer "+token1)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Success - Soft Deleted Message Shows Placeholder", func(t *testing.T) {

		delMsg, _ := testClient.Message.Create().
			SetChatID(chatEntity.ID).
			SetSenderID(u1.ID).
			SetContent("Sensitive Info").
			SetDeletedAt(time.Now()).
			Save(context.Background())

		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/%d/messages", chatEntity.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})

		found := false
		for _, item := range dataList {
			m := item.(map[string]interface{})
			if m["id"].(float64) == float64(delMsg.ID) {
				assert.Equal(t, "", m["content"])
				assert.NotNil(t, m["deleted_at"])
				found = true
			}
		}
		assert.True(t, found, "Deleted message should still be in the list with placeholder")
	})

	t.Run("Success - Reply Preview for Deleted Message", func(t *testing.T) {

		originalMsg, _ := testClient.Message.Create().
			SetChatID(chatEntity.ID).
			SetSenderID(u2.ID).
			SetContent("I will be deleted").
			Save(context.Background())

		replyMsg, _ := testClient.Message.Create().
			SetChatID(chatEntity.ID).
			SetSenderID(u1.ID).
			SetContent("Replying to a soon-to-be-deleted message").
			SetReplyToID(originalMsg.ID).
			Save(context.Background())

		testClient.Message.UpdateOne(originalMsg).SetDeletedAt(time.Now()).Exec(context.Background())

		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/%d/messages", chatEntity.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})

		var targetReply map[string]interface{}
		for _, item := range dataList {
			m := item.(map[string]interface{})
			if m["id"].(float64) == float64(replyMsg.ID) {
				targetReply = m
				break
			}
		}

		assert.NotNil(t, targetReply)
		replyToMap, ok := targetReply["reply_to"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "", replyToMap["content"])
		assert.NotNil(t, replyToMap["deleted_at"])
		assert.Equal(t, "User 2", replyToMap["sender_name"])
	})

	t.Run("Success - Media Placeholder Preview", func(t *testing.T) {

		media, _ := testClient.Media.Create().
			SetFileName("test_image.jpg").
			SetOriginalName("image.jpg").
			SetFileSize(12345).
			SetMimeType("image/jpeg").
			SetStatus("active").
			SetUploaderID(u1.ID).
			Save(context.Background())

		msgWithMedia, _ := testClient.Message.Create().
			SetChatID(chatEntity.ID).
			SetSenderID(u1.ID).
			AddAttachmentIDs(media.ID).
			Save(context.Background())

		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/%d/messages", chatEntity.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})

		var targetMsg map[string]interface{}
		for _, item := range dataList {
			m := item.(map[string]interface{})
			if m["id"].(float64) == float64(msgWithMedia.ID) {
				targetMsg = m
				break
			}
		}

		assert.NotNil(t, targetMsg)
		assert.Equal(t, "", targetMsg["content"])
		assert.NotNil(t, targetMsg["attachments"])
		assert.Len(t, targetMsg["attachments"], 1)
	})

	t.Run("Success - GetMessages respects hidden_at", func(t *testing.T) {
		clearDatabase(context.Background())
		u1, _ := testClient.User.Create().SetEmail("u1@test.com").SetUsername("u1").SetFullName("User 1").Save(context.Background())
		u2, _ := testClient.User.Create().SetEmail("u2@test.com").SetUsername("u2").SetFullName("User 2").Save(context.Background())
		token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

		chatEntity, _ := testClient.Chat.Create().SetType(chat.TypePrivate).Save(context.Background())
		pc, _ := testClient.PrivateChat.Create().SetChat(chatEntity).SetUser1(u1).SetUser2(u2).Save(context.Background())

		testClient.Message.Create().SetChatID(chatEntity.ID).SetSenderID(u2.ID).SetContent("Old Message 1").SetCreatedAt(time.Now().Add(-2 * time.Hour)).SaveX(context.Background())
		testClient.Message.Create().SetChatID(chatEntity.ID).SetSenderID(u1.ID).SetContent("Old Message 2").SetCreatedAt(time.Now().Add(-1 * time.Hour)).SaveX(context.Background())

		hideTime := time.Now()
		testClient.PrivateChat.UpdateOne(pc).SetUser1HiddenAt(hideTime).ExecX(context.Background())
		time.Sleep(10 * time.Millisecond)

		msg3, _ := testClient.Message.Create().SetChatID(chatEntity.ID).SetSenderID(u2.ID).SetContent("New Message 3").Save(context.Background())

		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/%d/messages", chatEntity.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})

		assert.Len(t, dataList, 1, "Should only get messages after hidden_at")
		msgData := dataList[0].(map[string]interface{})
		assert.Equal(t, float64(msg3.ID), msgData["id"])
		assert.Equal(t, "New Message 3", msgData["content"])
	})
}

func TestDeleteMessage(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)
	u1, _ := testClient.User.Create().SetEmail("u1@test.com").SetUsername("u1").SetFullName("User 1").SetPasswordHash(hashedPassword).Save(context.Background())
	u2, _ := testClient.User.Create().SetEmail("u2@test.com").SetUsername("u2").SetFullName("User 2").SetPasswordHash(hashedPassword).Save(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u2.ID)

	chatEntity, _ := testClient.Chat.Create().SetType(chat.TypePrivate).Save(context.Background())
	testClient.PrivateChat.Create().SetChat(chatEntity).SetUser1(u1).SetUser2(u2).Save(context.Background())

	t.Run("Success - Delete Own Message", func(t *testing.T) {
		msg, _ := testClient.Message.Create().
			SetChatID(chatEntity.ID).
			SetSenderID(u1.ID).
			SetContent("To be deleted").
			Save(context.Background())

		req, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/messages/%d", msg.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		deletedMsg, _ := testClient.Message.Get(context.Background(), msg.ID)
		assert.NotNil(t, deletedMsg.DeletedAt)
	})

	t.Run("Fail - Delete Other User's Message", func(t *testing.T) {
		msg, _ := testClient.Message.Create().
			SetChatID(chatEntity.ID).
			SetSenderID(u1.ID).
			SetContent("User 1 Message").
			Save(context.Background())

		req, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/messages/%d", msg.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token2)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)

		notDeletedMsg, _ := testClient.Message.Get(context.Background(), msg.ID)
		assert.Nil(t, notDeletedMsg.DeletedAt)
	})

	t.Run("Fail - Message Not Found", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", "/api/messages/99999", nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Fail - Already Deleted", func(t *testing.T) {
		msg, _ := testClient.Message.Create().
			SetChatID(chatEntity.ID).
			SetSenderID(u1.ID).
			SetContent("Already deleted").
			SetDeletedAt(time.Now()).
			Save(context.Background())

		req, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/messages/%d", msg.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}
