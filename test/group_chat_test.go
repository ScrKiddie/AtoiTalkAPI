package test

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/groupchat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/message"
	"AtoiTalkAPI/internal/helper"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateGroupChat(t *testing.T) {
	clearDatabase(context.Background())
	cleanupStorage(true)

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)

	u1, _ := testClient.User.Create().SetEmail("u1@test.com").SetUsername("user1").SetFullName("User 1").SetPasswordHash(hashedPassword).Save(context.Background())
	u2, _ := testClient.User.Create().SetEmail("u2@test.com").SetUsername("user2").SetFullName("User 2").SetPasswordHash(hashedPassword).Save(context.Background())
	u3, _ := testClient.User.Create().SetEmail("u3@test.com").SetUsername("user3").SetFullName("User 3").SetPasswordHash(hashedPassword).Save(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

	t.Run("Success - Create Group with Text Only", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "Test Group 1")
		_ = writer.WriteField("description", "A group for testing")

		usernamesJSON, _ := json.Marshal([]string{u2.Username, u3.Username})
		_ = writer.WriteField("member_usernames", string(usernamesJSON))

		writer.Close()

		req, _ := http.NewRequest("POST", "/api/chats/group", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
			return
		}

		gc, err := testClient.GroupChat.Query().Where(groupchat.Name("Test Group 1")).WithChat().Only(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, u1.ID, *gc.CreatedBy)

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

		sysMsg, err := gc.Edges.Chat.QueryMessages().Order(ent.Asc(message.FieldID)).First(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, message.TypeSystemCreate, sysMsg.Type)
		assert.Equal(t, u1.ID, *sysMsg.SenderID)
		assert.Equal(t, "Test Group 1", sysMsg.ActionData["initial_name"])
		assert.Equal(t, sysMsg.ID, *gc.Edges.Chat.LastMessageID)
	})

	t.Run("Success - Create Group with Avatar", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "Group With Avatar")

		usernamesJSON, _ := json.Marshal([]string{u2.Username})
		_ = writer.WriteField("member_usernames", string(usernamesJSON))

		part, _ := writer.CreateFormFile("avatar", "test_avatar.jpg")
		fileContent := createTestImage(t, 100, 100)
		_, _ = io.Copy(part, bytes.NewReader(fileContent))
		writer.Close()

		req, _ := http.NewRequest("POST", "/api/chats/group", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
			return
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

		usernamesJSON, _ := json.Marshal([]string{u1.Username})
		_ = writer.WriteField("member_usernames", string(usernamesJSON))

		writer.Close()

		req, _ := http.NewRequest("POST", "/api/chats/group", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Invalid Member Username", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "Ghost Group")

		usernamesJSON, _ := json.Marshal([]string{"ghostuser"})
		_ = writer.WriteField("member_usernames", string(usernamesJSON))

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

		usernamesJSON, _ := json.Marshal([]string{u2.Username})
		_ = writer.WriteField("member_usernames", string(usernamesJSON))

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

		usernamesJSON, _ := json.Marshal([]string{u2.Username})
		_ = writer.WriteField("member_usernames", string(usernamesJSON))

		writer.Close()

		req, _ := http.NewRequest("POST", "/api/chats/group", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Invalid JSON in member_usernames", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "Invalid JSON Group")
		_ = writer.WriteField("member_usernames", `[1,2,abc]`)
		writer.Close()

		req, _ := http.NewRequest("POST", "/api/chats/group", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Success - Group Survives Creator Deletion (SetNull)", func(t *testing.T) {

		creator, _ := testClient.User.Create().SetEmail("creator@test.com").SetUsername("creator").SetFullName("Creator").Save(context.Background())
		member, _ := testClient.User.Create().SetEmail("member@test.com").SetUsername("member").SetFullName("Member").Save(context.Background())

		chatEntity := testClient.Chat.Create().SetType("group").SaveX(context.Background())
		gc := testClient.GroupChat.Create().SetChat(chatEntity).SetCreator(creator).SetName("Survivor Group").SaveX(context.Background())
		testClient.GroupMember.Create().SetGroupChat(gc).SetUser(creator).SetRole(groupmember.RoleOwner).SaveX(context.Background())
		testClient.GroupMember.Create().SetGroupChat(gc).SetUser(member).SetRole(groupmember.RoleMember).SaveX(context.Background())

		err := testClient.User.DeleteOneID(creator.ID).Exec(context.Background())
		assert.NoError(t, err)

		gcReload, err := testClient.GroupChat.Query().Where(groupchat.ID(gc.ID)).Only(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "Survivor Group", gcReload.Name)
		assert.Nil(t, gcReload.CreatedBy, "CreatedBy should be NULL")

		exists, _ := testClient.GroupMember.Query().Where(groupmember.GroupChatID(gc.ID), groupmember.UserID(member.ID)).Exist(context.Background())
		assert.True(t, exists, "Other members should remain in the group")
	})
}
