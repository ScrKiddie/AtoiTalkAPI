package test

import (
	"AtoiTalkAPI/ent/groupchat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/internal/helper"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateGroupChat(t *testing.T) {
	clearDatabase(context.Background())
	cleanupStorage(true)

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)
	u1, _ := testClient.User.Create().SetEmail("u1@test.com").SetUsername("u1").SetFullName("User 1").SetPasswordHash(hashedPassword).Save(context.Background())
	u2, _ := testClient.User.Create().SetEmail("u2@test.com").SetUsername("u2").SetFullName("User 2").SetPasswordHash(hashedPassword).Save(context.Background())
	u3, _ := testClient.User.Create().SetEmail("u3@test.com").SetUsername("u3").SetFullName("User 3").SetPasswordHash(hashedPassword).Save(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

	t.Run("Success - Create Group with Text Only", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "Test Group 1")
		_ = writer.WriteField("description", "A group for testing")
		_ = writer.WriteField("member_ids", `[`+strconv.Itoa(u2.ID)+`,`+strconv.Itoa(u3.ID)+`]`)
		writer.Close()

		req, _ := http.NewRequest("POST", "/api/chats/group", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		gc, err := testClient.GroupChat.Query().Where(groupchat.Name("Test Group 1")).Only(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, u1.ID, gc.CreatedBy)

		members, err := gc.QueryMembers().All(context.Background())
		assert.NoError(t, err)
		assert.Len(t, members, 3)

		ownerCount := 0
		memberCount := 0
		for _, m := range members {
			if m.UserID == u1.ID && m.Role == groupmember.RoleOwner {
				ownerCount++
			}
			if (m.UserID == u2.ID || m.UserID == u3.ID) && m.Role == groupmember.RoleMember {
				memberCount++
			}
		}
		assert.Equal(t, 1, ownerCount, "Creator should be owner")
		assert.Equal(t, 2, memberCount, "Other users should be members")
	})

	t.Run("Success - Create Group with Avatar", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "Group With Avatar")
		_ = writer.WriteField("member_ids", `[`+strconv.Itoa(u2.ID)+`]`)

		part, _ := writer.CreateFormFile("avatar", "test_avatar.jpg")
		fileContent := []byte("dummy image data")
		_, _ = io.Copy(part, bytes.NewReader(fileContent))
		writer.Close()

		req, _ := http.NewRequest("POST", "/api/chats/group", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		gc, err := testClient.GroupChat.Query().
			Where(groupchat.Name("Group With Avatar")).
			WithAvatar().
			Only(context.Background())
		assert.NoError(t, err)
		assert.NotNil(t, gc.Edges.Avatar)
		assert.FileExists(t, filepath.Join(testConfig.StorageProfile, gc.Edges.Avatar.FileName))
	})

	t.Run("Fail - No Members", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "Empty Group")
		_ = writer.WriteField("member_ids", `[]`)
		writer.Close()

		req, _ := http.NewRequest("POST", "/api/chats/group", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Add Self", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "Self Group")
		_ = writer.WriteField("member_ids", `[`+strconv.Itoa(u1.ID)+`]`)
		writer.Close()

		req, _ := http.NewRequest("POST", "/api/chats/group", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Invalid Member ID", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "Ghost Group")
		_ = writer.WriteField("member_ids", `[99999]`)
		writer.Close()

		req, _ := http.NewRequest("POST", "/api/chats/group", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Blocked Member", func(t *testing.T) {

		testClient.UserBlock.Create().SetBlockerID(u1.ID).SetBlockedID(u2.ID).Exec(context.Background())

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "Blocked Group")
		_ = writer.WriteField("member_ids", `[`+strconv.Itoa(u2.ID)+`]`)
		writer.Close()

		req, _ := http.NewRequest("POST", "/api/chats/group", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)

		testClient.UserBlock.Delete().Exec(context.Background())
	})

	t.Run("Fail - No Name", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("member_ids", `[`+strconv.Itoa(u2.ID)+`]`)
		writer.Close()

		req, _ := http.NewRequest("POST", "/api/chats/group", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Invalid JSON in member_ids", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "Invalid JSON Group")
		_ = writer.WriteField("member_ids", `[1,2,abc]`)
		writer.Close()

		req, _ := http.NewRequest("POST", "/api/chats/group", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestSearchGroupMembers(t *testing.T) {
	clearDatabase(context.Background())
	u1 := testClient.User.Create().SetEmail("u1@test.com").SetUsername("alpha").SetFullName("Alpha User").SaveX(context.Background())
	u2 := testClient.User.Create().SetEmail("u2@test.com").SetUsername("beta").SetFullName("Beta User").SaveX(context.Background())
	u3 := testClient.User.Create().SetEmail("u3@test.com").SetUsername("gamma").SetFullName("Gamma User").SaveX(context.Background())
	u4 := testClient.User.Create().SetEmail("u4@test.com").SetUsername("delta").SetFullName("Delta User").SaveX(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token4, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u4.ID)

	chatEntity := testClient.Chat.Create().SetType("group").SaveX(context.Background())
	gc := testClient.GroupChat.Create().SetChat(chatEntity).SetCreator(u1).SetName("Search Test Group").SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u1).SetRole(groupmember.RoleOwner).SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u2).SetRole(groupmember.RoleMember).SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u3).SetRole(groupmember.RoleMember).SaveX(context.Background())

	t.Run("Success - Get All Members", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/group/%d/members", chatEntity.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})

		assert.Len(t, dataList, 3)
		assert.False(t, resp.Meta.HasNext)
	})

	t.Run("Success - Search by Username", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/group/%d/members?query=beta", chatEntity.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})

		assert.Len(t, dataList, 1)
		member := dataList[0].(map[string]interface{})
		assert.Equal(t, "beta", member["username"])
	})

	t.Run("Success - Search by Full Name", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/group/%d/members?query=Gamma", chatEntity.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})

		assert.Len(t, dataList, 1)
		member := dataList[0].(map[string]interface{})
		assert.Equal(t, "Gamma User", member["full_name"])
	})

	t.Run("Success - Pagination", func(t *testing.T) {

		req1, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/group/%d/members?limit=2", chatEntity.ID), nil)
		req1.Header.Set("Authorization", "Bearer "+token1)
		rr1 := executeRequest(req1)
		assert.Equal(t, http.StatusOK, rr1.Code)

		var resp1 helper.ResponseWithPagination
		json.Unmarshal(rr1.Body.Bytes(), &resp1)
		dataList1 := resp1.Data.([]interface{})
		assert.Len(t, dataList1, 2)
		assert.True(t, resp1.Meta.HasNext)
		assert.NotEmpty(t, resp1.Meta.NextCursor)

		req2, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/group/%d/members?limit=2&cursor=%s", chatEntity.ID, resp1.Meta.NextCursor), nil)
		req2.Header.Set("Authorization", "Bearer "+token1)
		rr2 := executeRequest(req2)
		assert.Equal(t, http.StatusOK, rr2.Code)

		var resp2 helper.ResponseWithPagination
		json.Unmarshal(rr2.Body.Bytes(), &resp2)
		dataList2 := resp2.Data.([]interface{})
		assert.Len(t, dataList2, 1)
		assert.False(t, resp2.Meta.HasNext)
	})

	t.Run("Fail - Not a Member", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/chats/group/%d/members", chatEntity.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token4)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Fail - Group Not Found", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/chats/group/99999/members", nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}
