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

	ws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestWebSocketConnection(t *testing.T) {
	clearDatabase(context.Background())

	user1 := createWSUser(t, "user1", "user1@example.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user1.ID)

	server := httptest.NewServer(testRouter)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	header := http.Header{}
	header.Add("Authorization", "Bearer "+token1)

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

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	header2 := http.Header{}
	header2.Add("Authorization", "Bearer "+token2)
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

	conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, message, err := conn2.ReadMessage()
	assert.NoError(t, err)

	var event websocket.Event
	err = json.Unmarshal(message, &event)
	assert.NoError(t, err)

	assert.Equal(t, websocket.EventMessageNew, event.Type)
	payloadMap, ok := event.Payload.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "Hello WebSocket", payloadMap["content"])
	assert.NotNil(t, event.Meta)
	assert.Equal(t, 1, int(event.Meta.UnreadCount))
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
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	header1 := http.Header{}
	header1.Add("Authorization", "Bearer "+token1)
	conn1, _, err := ws.DefaultDialer.Dial(wsURL, header1)
	assert.NoError(t, err)
	defer conn1.Close()

	header2 := http.Header{}
	header2.Add("Authorization", "Bearer "+token2)
	conn2, _, err := ws.DefaultDialer.Dial(wsURL, header2)
	assert.NoError(t, err)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	typingEvent := websocket.Event{
		Type: websocket.EventTyping,
		Meta: &websocket.EventMeta{
			ChatID: chatID,
		},
	}
	err = conn1.WriteJSON(typingEvent)
	assert.NoError(t, err)

	conn2.SetReadDeadline(time.Now().Add(2 * time.Second))

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
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	header1 := http.Header{}
	header1.Add("Authorization", "Bearer "+token1)
	conn1, _, err := ws.DefaultDialer.Dial(wsURL, header1)
	assert.NoError(t, err)
	defer conn1.Close()

	time.Sleep(100 * time.Millisecond)

	header2 := http.Header{}
	header2.Add("Authorization", "Bearer "+token2)
	conn2, _, err := ws.DefaultDialer.Dial(wsURL, header2)
	assert.NoError(t, err)

	conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
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
			if int(payload["user_id"].(float64)) == user2.ID {
				foundOnline = true
				break
			}
		}
	}
	assert.True(t, foundOnline, "User 1 should receive user.online event for User 2")

	conn2.Close()
	time.Sleep(100 * time.Millisecond)

	conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
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
			if int(payload["user_id"].(float64)) == user2.ID {
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
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	header2 := http.Header{}
	header2.Add("Authorization", "Bearer "+token2)
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

	conn2A.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msgA, err := conn2A.ReadMessage()
	assert.NoError(t, err)
	var eventA websocket.Event
	json.Unmarshal(msgA, &eventA)
	assert.Equal(t, websocket.EventMessageNew, eventA.Type)
	assert.Equal(t, 1, int(eventA.Meta.UnreadCount))

	conn2B.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msgB, err := conn2B.ReadMessage()
	assert.NoError(t, err)
	var eventB websocket.Event
	json.Unmarshal(msgB, &eventB)
	assert.Equal(t, websocket.EventMessageNew, eventB.Type)
	assert.Equal(t, 1, int(eventB.Meta.UnreadCount))
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
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	header2 := http.Header{}
	header2.Add("Authorization", "Bearer "+token2)
	conn2A, _, _ := ws.DefaultDialer.Dial(wsURL, header2)
	defer conn2A.Close()
	conn2B, _, _ := ws.DefaultDialer.Dial(wsURL, header2)
	defer conn2B.Close()

	time.Sleep(100 * time.Millisecond)

	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/%d/read", chatID), nil)
	req.Header.Set("Authorization", "Bearer "+token2)
	executeRequest(req)

	conn2B.SetReadDeadline(time.Now().Add(2 * time.Second))
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
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	header3 := http.Header{}
	header3.Add("Authorization", "Bearer "+token3)
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

	conn3.SetReadDeadline(time.Now().Add(1 * time.Second))

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

func TestWebSocketRealtimeBlock(t *testing.T) {
	clearDatabase(context.Background())

	user1 := createWSUser(t, "user1", "user1@example.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user1.ID)

	user2 := createWSUser(t, "user2", "user2@example.com")
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user2.ID)

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	header2 := http.Header{}
	header2.Add("Authorization", "Bearer "+token2)
	conn2, _, err := ws.DefaultDialer.Dial(wsURL, header2)
	assert.NoError(t, err)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/users/%d/block", user2.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token1)
	executeRequest(req)

	conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	foundBlock := false
	for i := 0; i < 5; i++ {
		_, message, err := conn2.ReadMessage()
		if err != nil {
			break
		}
		var event websocket.Event
		json.Unmarshal(message, &event)
		if event.Type == websocket.EventUserBlock {
			foundBlock = true
			assert.Equal(t, user1.ID, event.Meta.SenderID)
			break
		}
	}
	assert.True(t, foundBlock, "User 2 should receive user.block event immediately")
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
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	header2 := http.Header{}
	header2.Add("Authorization", "Bearer "+token2)
	conn2, _, err := ws.DefaultDialer.Dial(wsURL, header2)
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

	conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	foundUpdate := false
	for i := 0; i < 5; i++ {
		_, message, err := conn2.ReadMessage()
		if err != nil {
			break
		}
		var event websocket.Event
		json.Unmarshal(message, &event)
		if event.Type == websocket.EventUserUpdate {
			payload, _ := event.Payload.(map[string]interface{})
			if payload["full_name"] == "Updated Name" {
				foundUpdate = true
				break
			}
		}
	}
	assert.True(t, foundUpdate, "User 2 should receive user.update event")
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
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	header2 := http.Header{}
	header2.Add("Authorization", "Bearer "+token2)
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
	msgID := int(msgData["id"].(float64))

	reqDel, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/messages/%d", msgID), nil)
	reqDel.Header.Set("Authorization", "Bearer "+token1)
	executeRequest(reqDel)

	conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	foundDelete := false
	for i := 0; i < 5; i++ {
		_, message, err := conn2.ReadMessage()
		if err != nil {
			break
		}
		var event websocket.Event
		json.Unmarshal(message, &event)
		if event.Type == websocket.EventMessageDelete {
			payload, _ := event.Payload.(map[string]interface{})
			if int(payload["message_id"].(float64)) == msgID {
				foundDelete = true
				break
			}
		}
	}
	assert.True(t, foundDelete, "User 2 should receive message.delete event")
}

func TestWebSocketRealtimeUnblock(t *testing.T) {
	clearDatabase(context.Background())

	user1 := createWSUser(t, "user1", "user1@example.com")
	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user1.ID)

	user2 := createWSUser(t, "user2", "user2@example.com")
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, user2.ID)

	testClient.UserBlock.Create().SetBlockerID(user1.ID).SetBlockedID(user2.ID).Save(context.Background())

	server := httptest.NewServer(testRouter)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	header2 := http.Header{}
	header2.Add("Authorization", "Bearer "+token2)
	conn2, _, err := ws.DefaultDialer.Dial(wsURL, header2)
	assert.NoError(t, err)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/users/%d/unblock", user2.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token1)
	executeRequest(req)

	conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	foundUnblock := false
	for i := 0; i < 5; i++ {
		_, message, err := conn2.ReadMessage()
		if err != nil {
			break
		}
		var event websocket.Event
		json.Unmarshal(message, &event)
		if event.Type == websocket.EventUserUnblock {
			foundUnblock = true
			assert.Equal(t, user1.ID, event.Meta.SenderID)
			break
		}
	}
	assert.True(t, foundUnblock, "User 2 should receive user.unblock event")
}

func createWSUser(t *testing.T, username, email string) *ent.User {
	hashedPassword, _ := helper.HashPassword("password123")
	u, err := testClient.User.Create().
		SetUsername(username).
		SetEmail(email).
		SetFullName(username).
		SetPasswordHash(hashedPassword).
		Save(context.Background())
	assert.NoError(t, err)
	return u
}

func createWSPrivateChat(t *testing.T, user2ID int, token string) {
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
