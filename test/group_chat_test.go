package test

import (
	"AtoiTalkAPI/ent/groupchat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/internal/helper"
	"bytes"
	"context"
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

		adminCount := 0
		memberCount := 0
		for _, m := range members {
			if m.UserID == u1.ID && m.Role == groupmember.RoleAdmin {
				adminCount++
			}
			if (m.UserID == u2.ID || m.UserID == u3.ID) && m.Role == groupmember.RoleMember {
				memberCount++
			}
		}
		assert.Equal(t, 1, adminCount, "Creator should be admin")
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
