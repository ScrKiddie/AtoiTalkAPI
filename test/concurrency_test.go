package test

import (
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/privatechat"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConcurrency_UnreadCount(t *testing.T) {
	clearDatabase(context.Background())

	hashedPassword, _ := helper.HashPassword("Password123!")
	u1 := testClient.User.Create().SetEmail("u1@test.com").SetUsername("u1").SetFullName("User 1").SetPasswordHash(hashedPassword).SaveX(context.Background())
	u2 := testClient.User.Create().SetEmail("u2@test.com").SetUsername("u2").SetFullName("User 2").SetPasswordHash(hashedPassword).SaveX(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token2, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u2.ID)

	chatEntity := testClient.Chat.Create().SetType(chat.TypePrivate).SaveX(context.Background())
	testClient.PrivateChat.Create().SetChat(chatEntity).SetUser1(u1).SetUser2(u2).SaveX(context.Background())

	var wg sync.WaitGroup
	messageCount := 20
	readCount := 5

	wg.Add(messageCount)
	for i := 0; i < messageCount; i++ {
		go func(idx int) {
			defer wg.Done()
			reqBody := model.SendMessageRequest{
				ChatID:  chatEntity.ID,
				Content: fmt.Sprintf("Message %d", idx),
			}
			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/api/messages", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token1)
			executeRequest(req)
		}(i)
	}

	wg.Add(readCount)
	for i := 0; i < readCount; i++ {
		go func() {
			defer wg.Done()

			time.Sleep(time.Millisecond * 10)
			req, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/%s/read", chatEntity.ID), nil)
			req.Header.Set("Authorization", "Bearer "+token2)
			executeRequest(req)
		}()
	}

	wg.Wait()

	pc, _ := testClient.PrivateChat.Query().Where(privatechat.ChatID(chatEntity.ID)).Only(context.Background())

	assert.GreaterOrEqual(t, pc.User2UnreadCount, 0)
	assert.LessOrEqual(t, pc.User2UnreadCount, messageCount)

	count, _ := testClient.Message.Query().Count(context.Background())
	assert.Equal(t, messageCount, count)
}
