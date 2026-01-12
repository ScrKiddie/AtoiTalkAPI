package test

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/websocket"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestWebSocketConnection(t *testing.T) {
	clearDatabase(context.Background())

	user1 := createWSUser(t, "user1", "user1@example.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user1.ID)

	server := httptest.NewServer(testRouter)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token1
	header := http.Header{}

	conn, _, err := ws.DefaultDialer.Dial(wsURL, header)
	assert.NoError(t, err)
	defer conn.Close()
}

func TestWebSocketBroadcastMessage(t *testing.T) {
	clearDatabase(context.Background())

	user1 := createWSUser(t, "user1", "user1@example.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user1.ID)

	user2 := createWSUser(t, "user2", "user2@example.com")
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user2.ID)

	createWSPrivateChat(t, user2.ID, token1)

	server := httptest.NewServer(testRouter)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2

	header2 := http.Header{}
	conn2, _, err := ws.DefaultDialer.Dial(wsURL, header2)
	assert.NoError(t, err)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	chats, _ := testClient.Chat.Query().All(context.Background())
	chatID := chats[0].ID

	reqBody := model.SendMessageRequest{
		ChatID:  chatID,
		Content: "Hello WebSocket",
	}
	jsonBody, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token1)
	req.Header.Set("Content-Type", "application/json")

	rr := executeRequest(req)
	assert.Equal(t, http.StatusOK, rr.Code)

	conn2.SetReadDeadline(time.Now().UTC().Add(2 * time.Second))

	foundEvent := false
	for i := 0; i < 5; i++ {
		_, message, err := conn2.ReadMessage()
		if err != nil {
			break
		}
		var event websocket.Event
		json.Unmarshal(message, &event)

		if event.Type == websocket.EventMessageNew {
			foundEvent = true
			payloadMap, ok := event.Payload.(map[string]interface{})
			assert.True(t, ok)
			assert.Equal(t, "Hello WebSocket", payloadMap["content"])
			assert.Equal(t, "regular", payloadMap["type"])
			assert.NotNil(t, event.Meta)
			assert.Equal(t, 1, int(event.Meta.UnreadCount))
			break
		}
	}
	assert.True(t, foundEvent, "Should have received message.new event")
}

func TestWebSocketTypingStatus(t *testing.T) {
	clearDatabase(context.Background())

	user1 := createWSUser(t, "user1", "user1@example.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user1.ID)

	user2 := createWSUser(t, "user2", "user2@example.com")
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user2.ID)

	createWSPrivateChat(t, user2.ID, token1)

	chats, _ := testClient.Chat.Query().All(context.Background())
	chatID := chats[0].ID

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL1 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token1
	wsURL2 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2

	header1 := http.Header{}
	conn1, _, err := ws.DefaultDialer.Dial(wsURL1, header1)
	assert.NoError(t, err)
	defer conn1.Close()

	header2 := http.Header{}
	conn2, _, err := ws.DefaultDialer.Dial(wsURL2, header2)
	assert.NoError(t, err)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	typingEvent := websocket.Event{
		Type: websocket.EventTyping,
		Meta: &websocket.EventMeta{
			ChatID:   chatID,
			SenderID: user1.ID,
		},
	}
	err = conn1.WriteJSON(typingEvent)
	assert.NoError(t, err)

	conn2.SetReadDeadline(time.Now().UTC().Add(2 * time.Second))

	foundTyping := false
	for i := 0; i < 5; i++ {
		_, message, err := conn2.ReadMessage()
		if err != nil {
			break
		}
		var event websocket.Event
		json.Unmarshal(message, &event)
		if event.Type == websocket.EventTyping {
			foundTyping = true
			assert.Equal(t, chatID, event.Meta.ChatID)
			assert.Equal(t, user1.ID, event.Meta.SenderID)
			break
		}
	}
	assert.True(t, foundTyping, "User 2 should receive typing event")
}

func TestWebSocketUserPresence(t *testing.T) {
	clearDatabase(context.Background())

	user1 := createWSUser(t, "user1", "user1@example.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user1.ID)

	user2 := createWSUser(t, "user2", "user2@example.com")
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user2.ID)

	createWSPrivateChat(t, user2.ID, token1)

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL1 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token1
	wsURL2 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2

	header1 := http.Header{}
	conn1, _, err := ws.DefaultDialer.Dial(wsURL1, header1)
	assert.NoError(t, err)
	defer conn1.Close()

	time.Sleep(100 * time.Millisecond)

	header2 := http.Header{}
	conn2, _, err := ws.DefaultDialer.Dial(wsURL2, header2)
	assert.NoError(t, err)

	conn1.SetReadDeadline(time.Now().UTC().Add(2 * time.Second))
	foundOnline := false
	for i := 0; i < 5; i++ {
		_, message, err := conn1.ReadMessage()
		if err != nil {
			break
		}
		var event websocket.Event
		json.Unmarshal(message, &event)
		if event.Type == websocket.EventUserOnline {
			payload, _ := event.Payload.(map[string]interface{})

			if payload["user_id"].(string) == user2.ID.String() {
				foundOnline = true
				break
			}
		}
	}
	assert.True(t, foundOnline, "User 1 should receive user.online event for User 2")

	conn2.Close()
	time.Sleep(100 * time.Millisecond)

	conn1.SetReadDeadline(time.Now().UTC().Add(2 * time.Second))
	foundOffline := false
	for i := 0; i < 5; i++ {
		_, message, err := conn1.ReadMessage()
		if err != nil {
			break
		}
		var event websocket.Event
		json.Unmarshal(message, &event)
		if event.Type == websocket.EventUserOffline {
			payload, _ := event.Payload.(map[string]interface{})
			if payload["user_id"].(string) == user2.ID.String() {
				foundOffline = true
				break
			}
		}
	}
	assert.True(t, foundOffline, "User 1 should receive user.offline event for User 2")
}

func TestWebSocketMultiDevice(t *testing.T) {
	clearDatabase(context.Background())

	user1 := createWSUser(t, "user1", "user1@example.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user1.ID)

	user2 := createWSUser(t, "user2", "user2@example.com")
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user2.ID)

	createWSPrivateChat(t, user2.ID, token1)
	chats, _ := testClient.Chat.Query().All(context.Background())
	chatID := chats[0].ID

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2

	header2 := http.Header{}
	conn2A, _, err := ws.DefaultDialer.Dial(wsURL, header2)
	assert.NoError(t, err)
	defer conn2A.Close()

	conn2B, _, err := ws.DefaultDialer.Dial(wsURL, header2)
	assert.NoError(t, err)
	defer conn2B.Close()

	time.Sleep(100 * time.Millisecond)

	reqBody := model.SendMessageRequest{
		ChatID:  chatID,
		Content: "Sync Test",
	}
	jsonBody, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token1)
	req.Header.Set("Content-Type", "application/json")
	executeRequest(req)

	verifyEvent(t, conn2A, websocket.EventMessageNew, user1.ID, uuid.Nil)
	verifyEvent(t, conn2B, websocket.EventMessageNew, user1.ID, uuid.Nil)
}

func TestWebSocketReadStatusSync(t *testing.T) {
	clearDatabase(context.Background())

	user1 := createWSUser(t, "user1", "user1@example.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user1.ID)

	user2 := createWSUser(t, "user2", "user2@example.com")
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user2.ID)

	createWSPrivateChat(t, user2.ID, token1)
	chats, _ := testClient.Chat.Query().All(context.Background())
	chatID := chats[0].ID

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2

	header2 := http.Header{}
	conn2A, _, _ := ws.DefaultDialer.Dial(wsURL, header2)
	defer conn2A.Close()
	conn2B, _, _ := ws.DefaultDialer.Dial(wsURL, header2)
	defer conn2B.Close()

	time.Sleep(100 * time.Millisecond)

	reqMsg := model.SendMessageRequest{
		ChatID:  chatID,
		Content: "Unread Message",
	}
	jsonMsg, _ := json.Marshal(reqMsg)
	reqSend, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(jsonMsg))
	reqSend.Header.Set("Authorization", "Bearer "+token1)
	reqSend.Header.Set("Content-Type", "application/json")
	executeRequest(reqSend)

	time.Sleep(100 * time.Millisecond)

	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/%s/read", chatID), nil)
	req.Header.Set("Authorization", "Bearer "+token2)
	executeRequest(req)

	conn2B.SetReadDeadline(time.Now().UTC().Add(2 * time.Second))
	foundRead := false
	for i := 0; i < 5; i++ {
		_, message, err := conn2B.ReadMessage()
		if err != nil {
			break
		}
		var event websocket.Event
		json.Unmarshal(message, &event)
		if event.Type == websocket.EventChatRead {
			foundRead = true
			assert.Equal(t, chatID, event.Meta.ChatID)
			break
		}
	}
	assert.True(t, foundRead, "Device B should receive chat.read event triggered by Device A")
}

func TestWebSocketSecurityLeak(t *testing.T) {
	clearDatabase(context.Background())

	user1 := createWSUser(t, "user1", "user1@example.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user1.ID)

	user2 := createWSUser(t, "user2", "user2@example.com")

	user3 := createWSUser(t, "user3", "user3@example.com")
	token3, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user3.ID)

	createWSPrivateChat(t, user2.ID, token1)
	chats, _ := testClient.Chat.Query().All(context.Background())
	chatID := chats[0].ID

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token3

	header3 := http.Header{}
	conn3, _, err := ws.DefaultDialer.Dial(wsURL, header3)
	assert.NoError(t, err)
	defer conn3.Close()

	time.Sleep(100 * time.Millisecond)

	reqBody := model.SendMessageRequest{
		ChatID:  chatID,
		Content: "Secret Message",
	}
	jsonBody, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token1)
	req.Header.Set("Content-Type", "application/json")
	executeRequest(req)

	conn3.SetReadDeadline(time.Now().UTC().Add(1 * time.Second))

	receivedSecret := false
	for {
		_, message, err := conn3.ReadMessage()
		if err != nil {
			break
		}
		var event websocket.Event
		json.Unmarshal(message, &event)
		if event.Type == websocket.EventMessageNew {
			receivedSecret = true
			break
		}
	}
	assert.False(t, receivedSecret, "User 3 (Outsider) should NOT receive message.new event")
}

func TestWebSocketBlockUnblockSync(t *testing.T) {
	clearDatabase(context.Background())

	user1 := createWSUser(t, "user1", "user1@example.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user1.ID)

	user2 := createWSUser(t, "user2", "user2@example.com")
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user2.ID)

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL1 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token1
	wsURL2 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2

	conn1A, _, err := ws.DefaultDialer.Dial(wsURL1, nil)
	assert.NoError(t, err)
	defer conn1A.Close()
	conn1B, _, err := ws.DefaultDialer.Dial(wsURL1, nil)
	assert.NoError(t, err)
	defer conn1B.Close()

	conn2, _, err := ws.DefaultDialer.Dial(wsURL2, nil)
	assert.NoError(t, err)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/users/%s/block", user2.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token1)
	executeRequest(req)

	verifyEvent(t, conn1A, websocket.EventUserBlock, user1.ID, user2.ID)
	verifyEvent(t, conn1B, websocket.EventUserBlock, user1.ID, user2.ID)
	verifyEvent(t, conn2, websocket.EventUserBlock, user1.ID, user2.ID)

	reqUnblock, _ := http.NewRequest("POST", fmt.Sprintf("/api/users/%s/unblock", user2.ID), nil)
	reqUnblock.Header.Set("Authorization", "Bearer "+token1)
	executeRequest(reqUnblock)

	verifyEvent(t, conn1A, websocket.EventUserUnblock, user1.ID, user2.ID)
	verifyEvent(t, conn1B, websocket.EventUserUnblock, user1.ID, user2.ID)
	verifyEvent(t, conn2, websocket.EventUserUnblock, user1.ID, user2.ID)
}

func TestWebSocketProfileUpdate(t *testing.T) {
	clearDatabase(context.Background())

	user1 := createWSUser(t, "user1", "user1@example.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user1.ID)

	user2 := createWSUser(t, "user2", "user2@example.com")
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user2.ID)

	createWSPrivateChat(t, user2.ID, token1)

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL1 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token1
	wsURL2 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2

	conn1A, _, err := ws.DefaultDialer.Dial(wsURL1, nil)
	assert.NoError(t, err)
	defer conn1A.Close()
	conn1B, _, err := ws.DefaultDialer.Dial(wsURL1, nil)
	assert.NoError(t, err)
	defer conn1B.Close()

	conn2, _, err := ws.DefaultDialer.Dial(wsURL2, nil)
	assert.NoError(t, err)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("full_name", "Updated Name")
	_ = writer.Close()

	req, _ := http.NewRequest("PUT", "/api/user/profile", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token1)
	executeRequest(req)

	verifyEvent(t, conn1B, websocket.EventUserUpdate, user1.ID, uuid.Nil)

	verifyEvent(t, conn2, websocket.EventUserUpdate, user1.ID, uuid.Nil)
}

func TestWebSocketMessageDelete(t *testing.T) {
	clearDatabase(context.Background())

	user1 := createWSUser(t, "user1", "user1@example.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user1.ID)

	user2 := createWSUser(t, "user2", "user2@example.com")
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user2.ID)

	createWSPrivateChat(t, user2.ID, token1)
	chats, _ := testClient.Chat.Query().All(context.Background())
	chatID := chats[0].ID

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2

	header2 := http.Header{}
	conn2, _, err := ws.DefaultDialer.Dial(wsURL, header2)
	assert.NoError(t, err)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	reqBody := model.SendMessageRequest{
		ChatID:  chatID,
		Content: "To be deleted",
	}
	jsonBody, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token1)
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequest(req)

	var msgResp helper.ResponseSuccess
	json.Unmarshal(rr.Body.Bytes(), &msgResp)
	msgData := msgResp.Data.(map[string]interface{})
	msgIDStr := msgData["id"].(string)
	msgID, _ := uuid.Parse(msgIDStr)

	reqDel, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/messages/%s", msgID), nil)
	reqDel.Header.Set("Authorization", "Bearer "+token1)
	executeRequest(reqDel)

	verifyEvent(t, conn2, websocket.EventMessageDelete, user1.ID, uuid.Nil)
}

func TestWebSocketMessageUpdate(t *testing.T) {
	clearDatabase(context.Background())

	user1 := createWSUser(t, "user1", "user1@example.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user1.ID)

	user2 := createWSUser(t, "user2", "user2@example.com")
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user2.ID)

	createWSPrivateChat(t, user2.ID, token1)
	chats, _ := testClient.Chat.Query().All(context.Background())
	chatID := chats[0].ID

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2

	header2 := http.Header{}
	conn2, _, err := ws.DefaultDialer.Dial(wsURL, header2)
	assert.NoError(t, err)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	reqBody := model.SendMessageRequest{
		ChatID:  chatID,
		Content: "Original Content",
	}
	jsonBody, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token1)
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequest(req)

	var msgResp helper.ResponseSuccess
	json.Unmarshal(rr.Body.Bytes(), &msgResp)
	msgData := msgResp.Data.(map[string]interface{})
	msgIDStr := msgData["id"].(string)
	msgID, _ := uuid.Parse(msgIDStr)

	conn2.SetReadDeadline(time.Now().UTC().Add(2 * time.Second))
	conn2.ReadMessage()

	editBody := model.EditMessageRequest{
		Content: "Edited Content",
	}
	jsonEdit, _ := json.Marshal(editBody)
	reqEdit, _ := http.NewRequest("PUT", fmt.Sprintf("/api/messages/%s", msgID), bytes.NewBuffer(jsonEdit))
	reqEdit.Header.Set("Authorization", "Bearer "+token1)
	reqEdit.Header.Set("Content-Type", "application/json")
	executeRequest(reqEdit)

	verifyEvent(t, conn2, websocket.EventMessageUpdate, user1.ID, uuid.Nil)
}

func TestWebSocketChatHide(t *testing.T) {
	clearDatabase(context.Background())

	user1 := createWSUser(t, "user1", "user1@example.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user1.ID)

	user2 := createWSUser(t, "user2", "user2@example.com")

	createWSPrivateChat(t, user2.ID, token1)
	chats, _ := testClient.Chat.Query().All(context.Background())
	chatID := chats[0].ID

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token1

	header1 := http.Header{}
	conn1, _, err := ws.DefaultDialer.Dial(wsURL, header1)
	assert.NoError(t, err)
	defer conn1.Close()

	time.Sleep(100 * time.Millisecond)

	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/%s/hide", chatID), nil)
	req.Header.Set("Authorization", "Bearer "+token1)
	executeRequest(req)

	verifyEvent(t, conn1, websocket.EventChatHide, user1.ID, uuid.Nil)
}

func TestWebSocketGroupChatCreation(t *testing.T) {
	clearDatabase(context.Background())
	u1 := createWSUser(t, "u1", "u1@test.com")
	u2 := createWSUser(t, "u2", "u2@test.com")
	u3 := createWSUser(t, "u3", "u3@test.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u2.ID)
	token3, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u3.ID)

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL1 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token1
	wsURL2 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2
	wsURL3 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token3

	conn1, _, _ := ws.DefaultDialer.Dial(wsURL1, nil)
	defer conn1.Close()
	conn2, _, _ := ws.DefaultDialer.Dial(wsURL2, nil)
	defer conn2.Close()
	conn3, _, _ := ws.DefaultDialer.Dial(wsURL3, nil)
	defer conn3.Close()

	time.Sleep(100 * time.Millisecond)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("name", "Test Group WS")

	idsJSON, _ := json.Marshal([]string{u2.ID.String(), u3.ID.String()})
	_ = writer.WriteField("member_ids", string(idsJSON))

	writer.Close()

	req, _ := http.NewRequest("POST", "/api/chats/group", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token1)
	executeRequest(req)

	verifyEvent(t, conn1, websocket.EventChatNew, u1.ID, uuid.Nil)
	verifyEvent(t, conn2, websocket.EventChatNew, u1.ID, uuid.Nil)
	verifyEvent(t, conn3, websocket.EventChatNew, u1.ID, uuid.Nil)
}

func TestWebSocketAddGroupMember(t *testing.T) {
	clearDatabase(context.Background())
	u1 := createWSUser(t, "u1", "u1@test.com")
	u2 := createWSUser(t, "u2", "u2@test.com")
	u3 := createWSUser(t, "u3", "u3@test.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u2.ID)
	token3, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u3.ID)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("name", "Add Member WS Test")

	idsJSON, _ := json.Marshal([]string{u2.ID.String()})
	_ = writer.WriteField("member_ids", string(idsJSON))

	writer.Close()
	req, _ := http.NewRequest("POST", "/api/chats/group", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token1)
	rr := executeRequest(req)

	if !assert.Equal(t, http.StatusOK, rr.Code) {
		return
	}

	var chatResp helper.ResponseSuccess
	json.Unmarshal(rr.Body.Bytes(), &chatResp)
	chatData := chatResp.Data.(map[string]interface{})
	chatIDStr := chatData["id"].(string)
	chatID, _ := uuid.Parse(chatIDStr)

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL1 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token1
	wsURL2 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2
	wsURL3 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token3

	conn1, _, _ := ws.DefaultDialer.Dial(wsURL1, nil)
	defer conn1.Close()
	conn2, _, _ := ws.DefaultDialer.Dial(wsURL2, nil)
	defer conn2.Close()
	conn3, _, _ := ws.DefaultDialer.Dial(wsURL3, nil)
	defer conn3.Close()

	time.Sleep(100 * time.Millisecond)

	addReqBody := model.AddGroupMemberRequest{UserIDs: []uuid.UUID{u3.ID}}
	addBody, _ := json.Marshal(addReqBody)
	addReq, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/group/%s/members", chatID), bytes.NewBuffer(addBody))
	addReq.Header.Set("Content-Type", "application/json")
	addReq.Header.Set("Authorization", "Bearer "+token1)
	executeRequest(addReq)

	verifyEvent(t, conn3, websocket.EventChatNew, u1.ID, uuid.Nil)

	verifyEvent(t, conn1, websocket.EventMessageNew, u1.ID, uuid.Nil)
	verifyEvent(t, conn2, websocket.EventMessageNew, u1.ID, uuid.Nil)
	verifyEvent(t, conn3, websocket.EventMessageNew, u1.ID, uuid.Nil)
}

func TestWebSocketUpdateGroupChat(t *testing.T) {
	clearDatabase(context.Background())
	u1 := createWSUser(t, "u1", "u1@test.com")
	u2 := createWSUser(t, "u2", "u2@test.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u2.ID)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("name", "Update WS Test")
	idsJSON, _ := json.Marshal([]string{u2.ID.String()})
	_ = writer.WriteField("member_ids", string(idsJSON))
	writer.Close()
	req, _ := http.NewRequest("POST", "/api/chats/group", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token1)
	rr := executeRequest(req)
	var chatResp helper.ResponseSuccess
	json.Unmarshal(rr.Body.Bytes(), &chatResp)
	chatData := chatResp.Data.(map[string]interface{})
	chatIDStr := chatData["id"].(string)
	chatID, _ := uuid.Parse(chatIDStr)

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL1 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token1
	wsURL2 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2

	conn1, _, _ := ws.DefaultDialer.Dial(wsURL1, nil)
	defer conn1.Close()
	conn2, _, _ := ws.DefaultDialer.Dial(wsURL2, nil)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	updateBody := &bytes.Buffer{}
	updateWriter := multipart.NewWriter(updateBody)
	_ = updateWriter.WriteField("name", "New Group Name")
	updateWriter.Close()

	updateReq, _ := http.NewRequest("PUT", fmt.Sprintf("/api/chats/group/%s", chatID), updateBody)
	updateReq.Header.Set("Content-Type", updateWriter.FormDataContentType())
	updateReq.Header.Set("Authorization", "Bearer "+token1)
	executeRequest(updateReq)

	conn2.SetReadDeadline(time.Now().UTC().Add(2 * time.Second))
	eventsReceived := make(map[websocket.EventType]bool)
	for i := 0; i < 2; i++ {
		_, msg, err := conn2.ReadMessage()
		if err == nil {
			var event websocket.Event
			json.Unmarshal(msg, &event)
			eventsReceived[event.Type] = true
		}
	}
	assert.True(t, eventsReceived[websocket.EventChatNew], "User 2 should receive chat.new")
	assert.True(t, eventsReceived[websocket.EventMessageNew], "User 2 should receive message.new")
}

func TestWebSocketKickMember(t *testing.T) {
	clearDatabase(context.Background())
	u1 := createWSUser(t, "u1", "u1@test.com")
	u2 := createWSUser(t, "u2", "u2@test.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u2.ID)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("name", "Kick WS Test")
	idsJSON, _ := json.Marshal([]string{u2.ID.String()})
	_ = writer.WriteField("member_ids", string(idsJSON))
	writer.Close()
	req, _ := http.NewRequest("POST", "/api/chats/group", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token1)
	rr := executeRequest(req)
	var chatResp helper.ResponseSuccess
	json.Unmarshal(rr.Body.Bytes(), &chatResp)
	chatData := chatResp.Data.(map[string]interface{})
	chatIDStr := chatData["id"].(string)
	chatID, _ := uuid.Parse(chatIDStr)

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL1 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token1
	wsURL2 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2

	conn1, _, _ := ws.DefaultDialer.Dial(wsURL1, nil)
	defer conn1.Close()
	conn2, _, _ := ws.DefaultDialer.Dial(wsURL2, nil)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	kickReq, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/group/%s/members/%s/kick", chatID, u2.ID), nil)
	kickReq.Header.Set("Authorization", "Bearer "+token1)
	executeRequest(kickReq)

	verifyEvent(t, conn1, websocket.EventMessageNew, u1.ID, uuid.Nil)

	verifyEvent(t, conn2, websocket.EventMessageNew, u1.ID, uuid.Nil)
}

func TestWebSocketUpdateRole(t *testing.T) {
	clearDatabase(context.Background())
	u1 := createWSUser(t, "u1", "u1@test.com")
	u2 := createWSUser(t, "u2", "u2@test.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u2.ID)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("name", "Role WS Test")
	idsJSON, _ := json.Marshal([]string{u2.ID.String()})
	_ = writer.WriteField("member_ids", string(idsJSON))
	writer.Close()
	req, _ := http.NewRequest("POST", "/api/chats/group", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token1)
	rr := executeRequest(req)
	var chatResp helper.ResponseSuccess
	json.Unmarshal(rr.Body.Bytes(), &chatResp)
	chatData := chatResp.Data.(map[string]interface{})
	chatIDStr := chatData["id"].(string)
	chatID, _ := uuid.Parse(chatIDStr)

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL1 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token1
	wsURL2 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2

	conn1, _, _ := ws.DefaultDialer.Dial(wsURL1, nil)
	defer conn1.Close()
	conn2, _, _ := ws.DefaultDialer.Dial(wsURL2, nil)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	roleBody := model.UpdateGroupMemberRoleRequest{Role: "admin"}
	jsonRole, _ := json.Marshal(roleBody)
	roleReq, _ := http.NewRequest("PUT", fmt.Sprintf("/api/chats/group/%s/members/%s/role", chatID, u2.ID), bytes.NewBuffer(jsonRole))
	roleReq.Header.Set("Content-Type", "application/json")
	roleReq.Header.Set("Authorization", "Bearer "+token1)
	executeRequest(roleReq)

	verifyEvent(t, conn1, websocket.EventMessageNew, u1.ID, uuid.Nil)
	verifyEvent(t, conn2, websocket.EventMessageNew, u1.ID, uuid.Nil)
}

func TestWebSocketTransferOwnership(t *testing.T) {
	clearDatabase(context.Background())
	u1 := createWSUser(t, "u1", "u1@test.com")
	u2 := createWSUser(t, "u2", "u2@test.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u2.ID)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("name", "Transfer WS Test")
	idsJSON, _ := json.Marshal([]string{u2.ID.String()})
	_ = writer.WriteField("member_ids", string(idsJSON))
	writer.Close()
	req, _ := http.NewRequest("POST", "/api/chats/group", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token1)
	rr := executeRequest(req)
	var chatResp helper.ResponseSuccess
	json.Unmarshal(rr.Body.Bytes(), &chatResp)
	chatData := chatResp.Data.(map[string]interface{})
	chatIDStr := chatData["id"].(string)
	chatID, _ := uuid.Parse(chatIDStr)

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL1 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token1
	wsURL2 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2

	conn1, _, _ := ws.DefaultDialer.Dial(wsURL1, nil)
	defer conn1.Close()
	conn2, _, _ := ws.DefaultDialer.Dial(wsURL2, nil)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	transferBody := model.TransferGroupOwnershipRequest{NewOwnerID: u2.ID}
	jsonTransfer, _ := json.Marshal(transferBody)
	transferReq, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/group/%s/transfer", chatID), bytes.NewBuffer(jsonTransfer))
	transferReq.Header.Set("Content-Type", "application/json")
	transferReq.Header.Set("Authorization", "Bearer "+token1)
	executeRequest(transferReq)

	verifyEvent(t, conn1, websocket.EventMessageNew, u1.ID, uuid.Nil)
	verifyEvent(t, conn2, websocket.EventMessageNew, u1.ID, uuid.Nil)
}

func TestWebSocketAccountDeletion(t *testing.T) {
	clearDatabase(context.Background())

	u1 := createWSUser(t, "u1", "u1@test.com")
	u2 := createWSUser(t, "u2", "u2@test.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u2.ID)

	createWSPrivateChat(t, u2.ID, token1)

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL2 := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token2

	conn2, _, err := ws.DefaultDialer.Dial(wsURL2, nil)
	assert.NoError(t, err)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	password := "password123"
	reqBody := model.DeleteAccountRequest{Password: &password}
	jsonBody, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("DELETE", "/api/account", bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token1)
	req.Header.Set("Content-Type", "application/json")
	executeRequest(req)

	verifyEvent(t, conn2, websocket.EventUserDeleted, u1.ID, uuid.Nil)
}

func createWSUser(t *testing.T, username, email string) *ent.User {
	hashedPassword, _ := helper.HashPassword("password123")

	if len(username) < 3 {
		username = username + "user"
	}
	u, err := testClient.User.Create().
		SetUsername(username).
		SetEmail(email).
		SetFullName(username).
		SetPasswordHash(hashedPassword).
		Save(context.Background())
	assert.NoError(t, err)
	return u
}

func createWSPrivateChat(t *testing.T, user2ID uuid.UUID, token string) {
	reqBody := model.CreatePrivateChatRequest{
		TargetUserID: user2ID,
	}
	jsonBody, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/chats/private", bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequest(req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func verifyEvent(t *testing.T, conn *ws.Conn, eventType websocket.EventType, senderID, blockedID uuid.UUID) {
	conn.SetReadDeadline(time.Now().UTC().Add(2 * time.Second))
	foundEvent := false
	for i := 0; i < 5; i++ {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var event websocket.Event
		json.Unmarshal(message, &event)

		if event.Type == eventType {
			foundEvent = true
			if event.Meta != nil {
				assert.Equal(t, senderID.String(), event.Meta.SenderID.String())
			}

			if eventType == websocket.EventUserBlock || eventType == websocket.EventUserUnblock {
				payload, ok := event.Payload.(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, senderID.String(), payload["blocker_id"])
				assert.Equal(t, blockedID.String(), payload["blocked_id"])
			}
			break
		}
	}
	assert.True(t, foundEvent, "Should have received event '"+string(eventType)+"'")
}
