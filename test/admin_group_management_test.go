package test

import (
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/user"
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

func TestAdminGetGroups(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)

	admin, _ := testClient.User.Create().
		SetEmail("admin_groups@test.com").
		SetUsername("admin_groups").
		SetFullName("Admin Groups").
		SetPasswordHash(hashedPassword).
		SetRole(user.RoleAdmin).
		Save(context.Background())

	regularUser, _ := testClient.User.Create().
		SetEmail("regular_groups@test.com").
		SetUsername("regular_groups").
		SetFullName("Regular Groups").
		SetPasswordHash(hashedPassword).
		Save(context.Background())

	for i := 1; i <= 5; i++ {
		chatEntity := testClient.Chat.Create().SetType(chat.TypeGroup).SaveX(context.Background())
		gc := testClient.GroupChat.Create().
			SetChat(chatEntity).
			SetCreator(admin).
			SetName(fmt.Sprintf("Test Group %d", i)).
			SetInviteCode(fmt.Sprintf("CODE%d", i)).
			SaveX(context.Background())
		testClient.GroupMember.Create().SetGroupChat(gc).SetUser(admin).SetRole(groupmember.RoleOwner).SaveX(context.Background())
	}

	adminToken, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, admin.ID)
	regularToken, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, regularUser.ID)

	t.Run("Success - List All Groups", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/admin/groups", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.GreaterOrEqual(t, len(dataList), 5)
	})

	t.Run("Success - Search by Name", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/admin/groups?query=Group%201", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.GreaterOrEqual(t, len(dataList), 1)
	})

	t.Run("Success - Pagination", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/admin/groups?limit=2", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 2)
		assert.True(t, resp.Meta.HasNext)
		assert.NotEmpty(t, resp.Meta.NextCursor)
	})

	t.Run("Fail - Forbidden for Regular User", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/admin/groups", nil)
		req.Header.Set("Authorization", "Bearer "+regularToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})
}

func TestAdminGetGroupDetail(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)

	admin, _ := testClient.User.Create().
		SetEmail("admin_group_detail@test.com").
		SetUsername("admin_group_detail").
		SetFullName("Admin Group Detail").
		SetPasswordHash(hashedPassword).
		SetRole(user.RoleAdmin).
		Save(context.Background())

	regularUser, _ := testClient.User.Create().
		SetEmail("regular_group_detail@test.com").
		SetUsername("regular_group_detail").
		SetFullName("Regular Group Detail").
		SetPasswordHash(hashedPassword).
		Save(context.Background())

	chatEntity := testClient.Chat.Create().SetType(chat.TypeGroup).SaveX(context.Background())
	gc := testClient.GroupChat.Create().
		SetChat(chatEntity).
		SetCreator(admin).
		SetName("Detail Test Group").
		SetDescription("Test Description").
		SetInviteCode("DETAILCODE").
		SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(admin).SetRole(groupmember.RoleOwner).SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(regularUser).SetRole(groupmember.RoleMember).SaveX(context.Background())

	testClient.Message.Create().SetChat(chatEntity).SetSender(admin).SetContent("Msg 1").SaveX(context.Background())
	testClient.Message.Create().SetChat(chatEntity).SetSender(admin).SetContent("Msg 2").SaveX(context.Background())

	adminToken, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, admin.ID)
	regularToken, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, regularUser.ID)

	t.Run("Success - Get Group Detail", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/admin/groups/%s", gc.ChatID), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataMap := resp.Data.(map[string]interface{})
		assert.Equal(t, gc.ID.String(), dataMap["id"])
		assert.Equal(t, "Detail Test Group", dataMap["name"])
		assert.Equal(t, "Test Description", dataMap["description"])
		assert.Equal(t, float64(2), dataMap["member_count"])
		assert.Equal(t, float64(2), dataMap["total_messages"])
	})

	t.Run("Fail - Group Not Found", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/admin/groups/01900000-0000-7000-8000-000000000001", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Fail - Forbidden for Regular User", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/admin/groups/%s", gc.ChatID), nil)
		req.Header.Set("Authorization", "Bearer "+regularToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})
}

func TestAdminDissolveGroup(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)

	admin, _ := testClient.User.Create().
		SetEmail("admin_dissolve@test.com").
		SetUsername("admin_dissolve").
		SetFullName("Admin Dissolve").
		SetPasswordHash(hashedPassword).
		SetRole(user.RoleAdmin).
		Save(context.Background())

	regularUser, _ := testClient.User.Create().
		SetEmail("regular_dissolve@test.com").
		SetUsername("regular_dissolve").
		SetFullName("Regular Dissolve").
		SetPasswordHash(hashedPassword).
		Save(context.Background())

	adminToken, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, admin.ID)
	regularToken, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, regularUser.ID)

	t.Run("Success - Dissolve Group (Soft Delete)", func(t *testing.T) {
		chatEntity := testClient.Chat.Create().SetType(chat.TypeGroup).SaveX(context.Background())
		gc := testClient.GroupChat.Create().
			SetChat(chatEntity).
			SetCreator(admin).
			SetName("Group To Dissolve").
			SetInviteCode(fmt.Sprintf("DISSOLVE%d", time.Now().UnixNano())).
			SaveX(context.Background())
		testClient.GroupMember.Create().SetGroupChat(gc).SetUser(admin).SetRole(groupmember.RoleOwner).SaveX(context.Background())

		req, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/admin/groups/%s", gc.ChatID), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		updatedChat, _ := testClient.Chat.Get(context.Background(), chatEntity.ID)
		assert.NotNil(t, updatedChat.DeletedAt, "Chat should have deleted_at set")
	})

	t.Run("Fail - Group Not Found", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", "/api/admin/groups/01900000-0000-7000-8000-000000000001", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Fail - Forbidden for Regular User", func(t *testing.T) {
		chatEntity := testClient.Chat.Create().SetType(chat.TypeGroup).SaveX(context.Background())
		gc := testClient.GroupChat.Create().
			SetChat(chatEntity).
			SetCreator(admin).
			SetName("Group Cannot Dissolve").
			SetInviteCode(fmt.Sprintf("NODISSOLVE%d", time.Now().UnixNano())).
			SaveX(context.Background())

		req, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/admin/groups/%s", gc.ChatID), nil)
		req.Header.Set("Authorization", "Bearer "+regularToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})
}

func TestAdminResetGroupInfo(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)

	admin, _ := testClient.User.Create().
		SetEmail("admin_reset_group@test.com").
		SetUsername("admin_reset_group").
		SetFullName("Admin Reset Group").
		SetPasswordHash(hashedPassword).
		SetRole(user.RoleAdmin).
		Save(context.Background())

	adminToken, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, admin.ID)

	t.Run("Success - Reset Description", func(t *testing.T) {
		chatEntity := testClient.Chat.Create().SetType(chat.TypeGroup).SaveX(context.Background())
		gc := testClient.GroupChat.Create().
			SetChat(chatEntity).
			SetCreator(admin).
			SetName("Reset Desc Group").
			SetDescription("This is a bad description").
			SetInviteCode(fmt.Sprintf("RESETDESC%d", time.Now().UnixNano())).
			SaveX(context.Background())

		reqBody := model.ResetGroupInfoRequest{
			ResetDescription: true,
		}
		body, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/admin/groups/%s/reset", gc.ChatID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		updatedGC, _ := testClient.GroupChat.Get(context.Background(), gc.ID)
		if updatedGC.Description != nil {
			assert.Equal(t, "", *updatedGC.Description)
		}
	})

	t.Run("Success - Reset Name", func(t *testing.T) {
		chatEntity := testClient.Chat.Create().SetType(chat.TypeGroup).SaveX(context.Background())
		gc := testClient.GroupChat.Create().
			SetChat(chatEntity).
			SetCreator(admin).
			SetName("Bad Group Name").
			SetInviteCode(fmt.Sprintf("RESETNAME%d", time.Now().UnixNano())).
			SaveX(context.Background())

		reqBody := model.ResetGroupInfoRequest{
			ResetName: true,
		}
		body, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/admin/groups/%s/reset", gc.ChatID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		updatedGC, _ := testClient.GroupChat.Get(context.Background(), gc.ID)
		assert.Contains(t, updatedGC.Name, "Group ")
	})

	t.Run("Success - Reset Avatar (Soft Delete)", func(t *testing.T) {
		chatEntity := testClient.Chat.Create().SetType(chat.TypeGroup).SaveX(context.Background())
		gc := testClient.GroupChat.Create().
			SetChat(chatEntity).
			SetCreator(admin).
			SetName("Reset Avatar Group").
			SetInviteCode(fmt.Sprintf("RESETAVATAR%d", time.Now().UnixNano())).
			SaveX(context.Background())

		media, _ := testClient.Media.Create().
			SetFileName("bad_group_avatar.jpg").
			SetOriginalName("bad.jpg").
			SetFileSize(1024).
			SetMimeType("image/jpeg").
			SetUploader(admin).
			Save(context.Background())

		testClient.GroupChat.UpdateOne(gc).SetAvatar(media).ExecX(context.Background())

		reqBody := model.ResetGroupInfoRequest{
			ResetAvatar: true,
		}
		body, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/admin/groups/%s/reset", gc.ChatID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		updatedGC, _ := testClient.GroupChat.Query().Where().WithAvatar().First(context.Background())
		assert.Nil(t, updatedGC.Edges.Avatar)
	})

	t.Run("Fail - Group Not Found", func(t *testing.T) {
		reqBody := model.ResetGroupInfoRequest{
			ResetDescription: true,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/admin/groups/01900000-0000-7000-8000-000000000001/reset", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}
