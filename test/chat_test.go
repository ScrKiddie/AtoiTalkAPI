package test

import (
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/privatechat"
	"AtoiTalkAPI/ent/userblock"
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

func TestGetChats(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)
	u1 := testClient.User.Create().SetEmail("u1@test.com").SetUsername("user1").SetFullName("User 1").SetPasswordHash(hashedPassword).SaveX(context.Background())
	u2 := testClient.User.Create().SetEmail("u2@test.com").SetUsername("user2").SetFullName("User 2").SetPasswordHash(hashedPassword).SaveX(context.Background())
	u3 := testClient.User.Create().SetEmail("u3@test.com").SetUsername("user3").SetFullName("User 3").SetPasswordHash(hashedPassword).SaveX(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

	chat1 := testClient.Chat.Create().SetType(chat.TypePrivate).SetUpdatedAt(time.Now().Add(-2 * time.Hour)).SaveX(context.Background())

	testClient.PrivateChat.Create().SetChat(chat1).SetUser1(u1).SetUser2(u2).SetUser1UnreadCount(3).SaveX(context.Background())
	msg1 := testClient.Message.Create().SetChat(chat1).SetSender(u2).SetContent("Old message").SetCreatedAt(time.Now().Add(-2 * time.Hour)).SaveX(context.Background())
	chat1.Update().SetLastMessage(msg1).SetLastMessageAt(msg1.CreatedAt).ExecX(context.Background())

	chat2 := testClient.Chat.Create().SetType(chat.TypePrivate).SetUpdatedAt(time.Now().Add(-1 * time.Hour)).SaveX(context.Background())
	testClient.PrivateChat.Create().SetChat(chat2).SetUser1(u1).SetUser2(u3).SetUser1UnreadCount(0).SaveX(context.Background())
	msg2 := testClient.Message.Create().SetChat(chat2).SetSender(u3).SetContent("New message").SetCreatedAt(time.Now().Add(-1 * time.Hour)).SaveX(context.Background())
	chat2.Update().SetLastMessage(msg2).SetLastMessageAt(msg2.CreatedAt).ExecX(context.Background())

	chat3 := testClient.Chat.Create().SetType(chat.TypeGroup).SetUpdatedAt(time.Now()).SaveX(context.Background())
	gc := testClient.GroupChat.Create().SetChat(chat3).SetCreator(u1).SetName("My Group").SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u1).SetUnreadCount(5).SaveX(context.Background())
	msg3 := testClient.Message.Create().SetChat(chat3).SetSender(u1).SetContent("Group message").SetCreatedAt(time.Now()).SaveX(context.Background())
	chat3.Update().SetLastMessage(msg3).SetLastMessageAt(msg3.CreatedAt).ExecX(context.Background())

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
		assert.Nil(t, c1["other_user_id"], "Group chat should not have other_user_id")

		assert.Equal(t, float64(chat2.ID), c2["id"])
		assert.Equal(t, "User 3", c2["name"])
		assert.NotContains(t, c2, "unread_count", "unread_count should be omitted when it is 0")
		assert.Equal(t, float64(u3.ID), c2["other_user_id"], "Private chat should have other_user_id")

		assert.Equal(t, float64(chat1.ID), c3["id"])
		assert.Equal(t, "User 2", c3["name"])
		assert.Equal(t, float64(3), c3["unread_count"])
		assert.Equal(t, float64(u2.ID), c3["other_user_id"], "Private chat should have other_user_id")
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

	t.Run("Success - Search by Username", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/chats?query=user3", nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 1)
		assert.Equal(t, "User 3", dataList[0].(map[string]interface{})["name"])
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

		u4 := testClient.User.Create().SetEmail("u4@test.com").SetUsername("u4").SetFullName("User 4").SetPasswordHash(hashedPassword).SaveX(context.Background())

		delChat := testClient.Chat.Create().SetType(chat.TypePrivate).SaveX(context.Background())
		testClient.PrivateChat.Create().SetChat(delChat).SetUser1(u1).SetUser2(u4).SaveX(context.Background())

		msg := testClient.Message.Create().SetChat(delChat).SetSender(u4).SetContent("This will be deleted").SaveX(context.Background())
		testClient.Message.UpdateOne(msg).SetDeletedAt(time.Now()).ExecX(context.Background())

		delChat.Update().SetUpdatedAt(time.Now()).SetLastMessage(msg).SetLastMessageAt(msg.CreatedAt).ExecX(context.Background())

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
		assert.Equal(t, "", lastMsg["content"])
		assert.NotNil(t, lastMsg["deleted_at"])
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

	t.Run("Success - Reappear Hidden Chat on New Message", func(t *testing.T) {

		pc, _ := testClient.PrivateChat.Query().Where(privatechat.ChatID(chat1.ID)).Only(context.Background())
		testClient.PrivateChat.UpdateOne(pc).SetUser1HiddenAt(time.Now()).ExecX(context.Background())

		req1, _ := http.NewRequest("GET", "/api/chats", nil)
		req1.Header.Set("Authorization", "Bearer "+token1)
		rr1 := executeRequest(req1)
		var resp1 helper.ResponseWithPagination
		json.Unmarshal(rr1.Body.Bytes(), &resp1)
		dataList1 := resp1.Data.([]interface{})
		for _, item := range dataList1 {
			c := item.(map[string]interface{})
			assert.NotEqual(t, float64(chat1.ID), c["id"], "Chat should be hidden")
		}

		newMsg := testClient.Message.Create().SetChat(chat1).SetSender(u2).SetContent("New message").SetCreatedAt(time.Now().Add(time.Second)).SaveX(context.Background())
		testClient.Chat.UpdateOneID(chat1.ID).SetUpdatedAt(time.Now().Add(time.Second)).SetLastMessage(newMsg).SetLastMessageAt(newMsg.CreatedAt).ExecX(context.Background())

		req2, _ := http.NewRequest("GET", "/api/chats", nil)
		req2.Header.Set("Authorization", "Bearer "+token1)
		rr2 := executeRequest(req2)
		var resp2 helper.ResponseWithPagination
		json.Unmarshal(rr2.Body.Bytes(), &resp2)
		dataList2 := resp2.Data.([]interface{})

		found := false
		for _, item := range dataList2 {
			c := item.(map[string]interface{})
			if c["id"].(float64) == float64(chat1.ID) {
				found = true
				break
			}
		}
		assert.True(t, found, "Chat should reappear after new activity")
	})

	t.Run("Success - List Chat with Blocked User", func(t *testing.T) {

		media, _ := testClient.Media.Create().
			SetFileName("u2_avatar.jpg").SetOriginalName("u2.jpg").
			SetFileSize(1024).SetMimeType("image/jpeg").
			SetUploader(u2).
			Save(context.Background())
		testClient.User.UpdateOne(u2).SetAvatar(media).ExecX(context.Background())

		testClient.UserBlock.Create().SetBlockerID(u1.ID).SetBlockedID(u2.ID).SaveX(context.Background())

		req, _ := http.NewRequest("GET", "/api/chats", nil)
		req.Header.Set("Authorization", "Bearer "+token1)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})

		var blockedChat map[string]interface{}
		for _, item := range dataList {
			c := item.(map[string]interface{})
			if c["id"].(float64) == float64(chat1.ID) {
				blockedChat = c
				break
			}
		}

		assert.NotNil(t, blockedChat)
		assert.True(t, blockedChat["is_blocked_by_me"].(bool))

		assert.NotEqual(t, "", blockedChat["avatar"])
		assert.NotEqual(t, "", blockedChat["name"])

		assert.False(t, blockedChat["is_online"].(bool))
		assert.Equal(t, float64(u2.ID), blockedChat["other_user_id"])

		testClient.UserBlock.Delete().Where(userblock.BlockerID(u1.ID), userblock.BlockedID(u2.ID)).ExecX(context.Background())
	})

	t.Run("Unauthorized", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/chats", nil)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestGetChatByID(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)
	u1 := testClient.User.Create().SetEmail("u1@test.com").SetUsername("user1").SetFullName("User 1").SetPasswordHash(hashedPassword).SaveX(context.Background())
	u2 := testClient.User.Create().SetEmail("u2@test.com").SetUsername("user2").SetFullName("User 2").SetPasswordHash(hashedPassword).SaveX(context.Background())
	u3 := testClient.User.Create().SetEmail("u3@test.com").SetUsername("user3").SetFullName("User 3").SetPasswordHash(hashedPassword).SaveX(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

	chat1 := testClient.Chat.Create().SetType(chat.TypePrivate).SaveX(context.Background())
	testClient.PrivateChat.Create().SetChat(chat1).SetUser1(u1).SetUser2(u2).SetUser1UnreadCount(3).SaveX(context.Background())

	t.Run("Success - Get Private Chat", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/%d", chat1.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)
		data := resp.Data.(map[string]interface{})

		assert.Equal(t, float64(chat1.ID), data["id"])
		assert.Equal(t, "User 2", data["name"])
		assert.Equal(t, float64(3), data["unread_count"])
	})

	t.Run("Success - Get Chat with Blocked User", func(t *testing.T) {
		testClient.UserBlock.Create().SetBlockerID(u1.ID).SetBlockedID(u2.ID).SaveX(context.Background())

		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/%d", chat1.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)
		data := resp.Data.(map[string]interface{})

		assert.True(t, data["is_blocked_by_me"].(bool))
		assert.NotEqual(t, "", data["name"])
		assert.False(t, data["is_online"].(bool))

		testClient.UserBlock.Delete().Where(userblock.BlockerID(u1.ID), userblock.BlockedID(u2.ID)).ExecX(context.Background())
	})

	t.Run("Fail - Invalid ID", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/chats/abc", nil)
		req.Header.Set("Authorization", "Bearer "+token1)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Not Found or Forbidden", func(t *testing.T) {

		chat2 := testClient.Chat.Create().SetType(chat.TypePrivate).SaveX(context.Background())
		testClient.PrivateChat.Create().SetChat(chat2).SetUser1(u2).SetUser2(u3).SaveX(context.Background())

		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/%d", chat2.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}

func TestMarkAsRead(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)
	u1 := testClient.User.Create().SetEmail("u1@test.com").SetUsername("u1").SetFullName("User 1").SetPasswordHash(hashedPassword).SaveX(context.Background())
	u2 := testClient.User.Create().SetEmail("u2@test.com").SetUsername("u2").SetFullName("User 2").SetPasswordHash(hashedPassword).SaveX(context.Background())
	u3 := testClient.User.Create().SetEmail("u3@test.com").SetUsername("u3").SetFullName("User 3").SetPasswordHash(hashedPassword).SaveX(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token3, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u3.ID)

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

	t.Run("Success - Mark Read While Blocked (Ninja Mode)", func(t *testing.T) {

		testClient.UserBlock.Create().SetBlockerID(u2.ID).SetBlockedID(u1.ID).SaveX(context.Background())

		pc, _ := testClient.PrivateChat.Query().Where(privatechat.ChatID(chat1.ID)).Only(context.Background())
		testClient.PrivateChat.UpdateOne(pc).SetUser1UnreadCount(5).SetUser1LastReadAt(time.Time{}).ExecX(context.Background())

		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/%d/read", chat1.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		pcAfter, _ := testClient.PrivateChat.Query().Where(privatechat.ChatID(chat1.ID)).Only(context.Background())
		assert.Equal(t, 0, pcAfter.User1UnreadCount, "Unread count should be reset")
		assert.True(t, pcAfter.User1LastReadAt.IsZero(), "LastReadAt should NOT be updated (Ninja Mode)")

		testClient.UserBlock.Delete().Where(userblock.BlockerID(u2.ID), userblock.BlockedID(u1.ID)).ExecX(context.Background())
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

	t.Run("Fail - Not a Member", func(t *testing.T) {
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/%d/read", chat1.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token3)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Not Found", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/api/chats/99999/read", nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}

func TestHideChat(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)
	u1 := testClient.User.Create().SetEmail("u1@test.com").SetUsername("u1").SetFullName("User 1").SetPasswordHash(hashedPassword).SaveX(context.Background())
	u2 := testClient.User.Create().SetEmail("u2@test.com").SetUsername("u2").SetFullName("User 2").SetPasswordHash(hashedPassword).SaveX(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

	chat1 := testClient.Chat.Create().SetType(chat.TypePrivate).SaveX(context.Background())
	testClient.PrivateChat.Create().SetChat(chat1).SetUser1(u1).SetUser2(u2).SetUser1UnreadCount(5).SaveX(context.Background())

	t.Run("Success - Hide Private Chat and Reset Unread Count", func(t *testing.T) {
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/%d/hide", chat1.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		pc, _ := testClient.PrivateChat.Query().Where(privatechat.ChatID(chat1.ID)).Only(context.Background())
		assert.NotNil(t, pc.User1HiddenAt)
		assert.Nil(t, pc.User2HiddenAt)
		assert.Equal(t, 0, pc.User1UnreadCount, "Unread count should be reset to 0 after hiding")
	})

	t.Run("Fail - Chat Not Found", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/api/chats/99999/hide", nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Fail - Not Private Chat", func(t *testing.T) {
		chatGroup := testClient.Chat.Create().SetType(chat.TypeGroup).SaveX(context.Background())
		testClient.GroupChat.Create().SetChat(chatGroup).SetCreator(u1).SetName("Group").SaveX(context.Background())

		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/%d/hide", chatGroup.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestGroupUnreadConsistency(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)
	u1 := testClient.User.Create().SetEmail("u1@test.com").SetUsername("u1").SetFullName("User 1").SetPasswordHash(hashedPassword).SaveX(context.Background())
	u2 := testClient.User.Create().SetEmail("u2@test.com").SetUsername("u2").SetFullName("User 2").SetPasswordHash(hashedPassword).SaveX(context.Background())
	u3 := testClient.User.Create().SetEmail("u3@test.com").SetUsername("u3").SetFullName("User 3").SetPasswordHash(hashedPassword).SaveX(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u2.ID)

	chatGroup := testClient.Chat.Create().SetType(chat.TypeGroup).SaveX(context.Background())
	gc := testClient.GroupChat.Create().SetChat(chatGroup).SetCreator(u1).SetName("Test Group").SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u1).SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u2).SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u3).SaveX(context.Background())

	reqBody := model.SendMessageRequest{
		ChatID:  chatGroup.ID,
		Content: "Hello Group!",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token1)
	executeRequest(req)

	gm1, _ := testClient.GroupMember.Query().Where(groupmember.GroupChatID(gc.ID), groupmember.UserID(u1.ID)).Only(context.Background())
	gm2, _ := testClient.GroupMember.Query().Where(groupmember.GroupChatID(gc.ID), groupmember.UserID(u2.ID)).Only(context.Background())
	gm3, _ := testClient.GroupMember.Query().Where(groupmember.GroupChatID(gc.ID), groupmember.UserID(u3.ID)).Only(context.Background())

	assert.Equal(t, 0, gm1.UnreadCount, "Sender should have 0 unread")
	assert.Equal(t, 1, gm2.UnreadCount, "Member 2 should have 1 unread")
	assert.Equal(t, 1, gm3.UnreadCount, "Member 3 should have 1 unread")

	reqRead, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/%d/read", chatGroup.ID), nil)
	reqRead.Header.Set("Authorization", "Bearer "+token2)
	executeRequest(reqRead)

	gm2After, _ := testClient.GroupMember.Query().Where(groupmember.GroupChatID(gc.ID), groupmember.UserID(u2.ID)).Only(context.Background())
	gm3After, _ := testClient.GroupMember.Query().Where(groupmember.GroupChatID(gc.ID), groupmember.UserID(u3.ID)).Only(context.Background())

	assert.Equal(t, 0, gm2After.UnreadCount, "Member 2 should have 0 unread after reading")
	assert.Equal(t, 1, gm3After.UnreadCount, "Member 3 should still have 1 unread")
}
