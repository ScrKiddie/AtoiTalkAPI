package test

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupchat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/message"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestUpdateGroupChat(t *testing.T) {
	clearDatabase(context.Background())
	cleanupStorage(true)

	u1 := testClient.User.Create().SetEmail("u1@test.com").SetUsername("owner").SetFullName("Owner").SaveX(context.Background())
	u2 := testClient.User.Create().SetEmail("u2@test.com").SetUsername("admin").SetFullName("Admin").SaveX(context.Background())
	u3 := testClient.User.Create().SetEmail("u3@test.com").SetUsername("member").SetFullName("Member").SaveX(context.Background())
	u4 := testClient.User.Create().SetEmail("u4@test.com").SetUsername("outsider").SetFullName("Outsider").SaveX(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token3, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u3.ID)
	token4, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u4.ID)

	chatEntity := testClient.Chat.Create().SetType("group").SaveX(context.Background())
	gc := testClient.GroupChat.Create().SetChat(chatEntity).SetCreator(u1).SetName("Original Name").SetDescription("Original Desc").SaveX(context.Background())

	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u1).SetRole(groupmember.RoleOwner).SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u2).SetRole(groupmember.RoleAdmin).SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u3).SetRole(groupmember.RoleMember).SaveX(context.Background())

	t.Run("Success - Rename Group (Owner)", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "New Group Name")
		writer.Close()

		req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/chats/group/%d", gc.ChatID), body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
			return
		}

		gcReload, _ := testClient.GroupChat.Query().Where(groupchat.ID(gc.ID)).Only(context.Background())
		assert.Equal(t, "New Group Name", gcReload.Name)

		lastMsg, err := testClient.Chat.Query().Where(chat.ID(gc.ChatID)).QueryLastMessage().Only(context.Background())
		if assert.NoError(t, err) {
			assert.Equal(t, message.TypeSystemRename, lastMsg.Type)
			assert.Equal(t, "New Group Name", lastMsg.ActionData["new_name"])
		}
	})

	t.Run("Success - Update Description (Owner)", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("description", "New Description")
		writer.Close()

		req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/chats/group/%d", gc.ChatID), body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		gcReload, _ := testClient.GroupChat.Query().Where(groupchat.ID(gc.ID)).Only(context.Background())
		assert.Equal(t, "New Description", *gcReload.Description)
	})

	t.Run("Success - Update Avatar (Owner)", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("avatar", "new_avatar.jpg")
		fileContent := createTestImage(t, 100, 100)
		_, _ = io.Copy(part, bytes.NewReader(fileContent))
		writer.Close()

		req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/chats/group/%d", gc.ChatID), body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		gcReload, _ := testClient.GroupChat.Query().Where(groupchat.ID(gc.ID)).WithAvatar().Only(context.Background())
		assert.NotNil(t, gcReload.Edges.Avatar)
		assert.FileExists(t, filepath.Join(testConfig.StorageProfile, gcReload.Edges.Avatar.FileName))

		lastMsg, _ := testClient.Chat.Query().Where(chat.ID(gc.ChatID)).QueryLastMessage().Only(context.Background())
		assert.Equal(t, message.TypeSystemAvatar, lastMsg.Type)
	})

	t.Run("Fail - Member (Not Admin/Owner)", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "Hacked Name")
		writer.Close()

		req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/chats/group/%d", gc.ChatID), body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token3)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Fail - Not Member", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "Hacked Name")
		writer.Close()

		req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/chats/group/%d", gc.ChatID), body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token4)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Fail - Invalid Data (Name too short)", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "Hi")
		writer.Close()

		req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/chats/group/%d", gc.ChatID), body)
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

func TestAddGroupMember(t *testing.T) {
	clearDatabase(context.Background())
	u1 := testClient.User.Create().SetEmail("u1@test.com").SetUsername("owner").SetFullName("Owner User").SaveX(context.Background())
	u2 := testClient.User.Create().SetEmail("u2@test.com").SetUsername("admin").SetFullName("Admin User").SaveX(context.Background())
	u3 := testClient.User.Create().SetEmail("u3@test.com").SetUsername("member").SetFullName("Member User").SaveX(context.Background())
	u4 := testClient.User.Create().SetEmail("u4@test.com").SetUsername("newbie").SetFullName("Newbie User").SaveX(context.Background())
	u5 := testClient.User.Create().SetEmail("u5@test.com").SetUsername("newbie2").SetFullName("Newbie User 2").SaveX(context.Background())

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token3, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u3.ID)

	chatEntity := testClient.Chat.Create().SetType("group").SaveX(context.Background())
	gc := testClient.GroupChat.Create().SetChat(chatEntity).SetCreator(u1).SetName("Add Member Test").SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u1).SetRole(groupmember.RoleOwner).SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u2).SetRole(groupmember.RoleAdmin).SaveX(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u3).SetRole(groupmember.RoleMember).SaveX(context.Background())

	t.Run("Success - Owner Adds Multiple Members", func(t *testing.T) {
		reqBody := model.AddGroupMemberRequest{Usernames: []string{u4.Username, u5.Username}}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/group/%d/members", chatEntity.ID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		isMember4, _ := testClient.GroupMember.Query().Where(groupmember.GroupChatID(gc.ID), groupmember.UserID(u4.ID)).Exist(context.Background())
		assert.True(t, isMember4)
		isMember5, _ := testClient.GroupMember.Query().Where(groupmember.GroupChatID(gc.ID), groupmember.UserID(u5.ID)).Exist(context.Background())
		assert.True(t, isMember5)

		gc, _ = testClient.GroupChat.Query().Where(groupchat.ID(gc.ID)).WithChat(func(q *ent.ChatQuery) {
			q.WithLastMessage()
		}).Only(context.Background())
		sysMsg := gc.Edges.Chat.Edges.LastMessage
		assert.NotNil(t, sysMsg)
		assert.Equal(t, message.TypeSystemAdd, sysMsg.Type)
		assert.Equal(t, u1.ID, *sysMsg.SenderID)

		assert.Equal(t, float64(u5.ID), sysMsg.ActionData["target_id"])
	})

	t.Run("Fail - Member Tries to Add Member", func(t *testing.T) {
		u6 := testClient.User.Create().SetEmail("u6@test.com").SetUsername("another").SetFullName("Another User").SaveX(context.Background())
		reqBody := model.AddGroupMemberRequest{Usernames: []string{u6.Username}}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/group/%d/members", chatEntity.ID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token3)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Fail - Add Existing Member", func(t *testing.T) {
		reqBody := model.AddGroupMemberRequest{Usernames: []string{u2.Username}}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/group/%d/members", chatEntity.ID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusConflict, rr.Code)
	})

	t.Run("Fail - Add Non-Existent User", func(t *testing.T) {
		reqBody := model.AddGroupMemberRequest{Usernames: []string{"ghostuser"}}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/chats/group/%d/members", chatEntity.ID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}
