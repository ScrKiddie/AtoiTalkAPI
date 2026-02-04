package test

import (
	"AtoiTalkAPI/ent/groupchat"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDashboardStats(t *testing.T) {
	if testClient == nil {
		t.Fatal("testClient is nil")
	}
	clearDatabase(context.Background())

	adminUser := createTestUser(t, "admin_stats")
	if adminUser == nil {
		t.Fatal("adminUser is nil")
	}
	t.Logf("Admin User Created: %v", adminUser.ID)

	_, err := testClient.User.UpdateOne(adminUser).SetRole(user.RoleAdmin).Save(context.Background())
	if err != nil {
		t.Fatalf("Failed to promote user to admin: %v", err)
	}

	createTestUser(t, "user1")
	createTestUser(t, "user2")
	createTestUser(t, "user3")

	owner := adminUser
	_, err = testClient.GroupChat.Create().
		SetChat(testClient.Chat.Create().SetType("group").SaveX(context.Background())).
		SetName("Group 1").
		SetInviteCode("CODE1").
		SetCreatedBy(owner.ID).
		Save(context.Background())
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}
	_, err = testClient.GroupChat.Create().
		SetChat(testClient.Chat.Create().SetType("group").SaveX(context.Background())).
		SetName("Group 2").
		SetInviteCode("CODE2").
		SetCreatedBy(owner.ID).
		Save(context.Background())
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	group1 := testClient.GroupChat.Query().Where(groupchat.Name("Group 1")).WithChat().OnlyX(context.Background())
	chat1 := group1.Edges.Chat

	testClient.Message.Create().SetChat(chat1).SetSender(adminUser).SetContent("Msg 1").SaveX(context.Background())
	testClient.Message.Create().SetChat(chat1).SetSender(adminUser).SetContent("Msg 2").SaveX(context.Background())
	testClient.Message.Create().SetChat(chat1).SetSender(adminUser).SetContent("Msg 3").SaveX(context.Background())

	users := testClient.User.Query().AllX(context.Background())
	target := users[1]

	testClient.Report.Create().
		SetReporter(adminUser).
		SetTargetUserID(target.ID).
		SetTargetType("user").
		SetReason("Spam").
		SetEvidenceSnapshot(map[string]interface{}{"foo": "bar"}).
		SaveX(context.Background())

	testClient.Report.Create().
		SetReporter(adminUser).
		SetTargetUserID(target.ID).
		SetTargetType("user").
		SetReason("Abuse").
		SetEvidenceSnapshot(map[string]interface{}{"foo": "bar"}).
		SaveX(context.Background())

	loginPayload := map[string]string{
		"email":         *adminUser.Email,
		"password":      "Password123!",
		"captcha_token": cfTurnstileAlwaysPasses,
	}
	body, _ := json.Marshal(loginPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequest(req)

	var loginResp struct {
		Data struct {
			AccessToken string `json:"token"`
		} `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &loginResp)
	token := loginResp.Data.AccessToken

	req = httptest.NewRequest(http.MethodGet, "/api/admin/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr = executeRequest(req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp helper.ResponseSuccess
	json.Unmarshal(rr.Body.Bytes(), &resp)

	dataBytes, _ := json.Marshal(resp.Data)
	var stats model.DashboardStatsResponse
	json.Unmarshal(dataBytes, &stats)

	assert.Equal(t, 4, stats.TotalUsers)
	assert.Equal(t, 2, stats.TotalGroups)
	assert.Equal(t, 3, stats.TotalMessages)
	assert.Equal(t, 2, stats.ActiveReports)
}
