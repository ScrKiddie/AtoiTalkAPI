package test

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/ent/userblock"
	"AtoiTalkAPI/internal/helper"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createTestImage(t *testing.T, width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}

	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, nil)
	assert.NoError(t, err)
	return buf.Bytes()
}

func TestGetCurrentUser(t *testing.T) {
	validEmail := "current@example.com"
	validUsername := "currentuser"
	validName := "Current User"
	validBio := "I am current user"

	t.Run("Success", func(t *testing.T) {
		clearDatabase(context.Background())

		u, err := testClient.User.Create().
			SetEmail(validEmail).
			SetUsername(validUsername).
			SetFullName(validName).
			SetBio(validBio).
			Save(context.Background())
		assert.NoError(t, err)

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u.ID)

		req, _ := http.NewRequest("GET", "/api/user/current", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataMap, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, validEmail, dataMap["email"])
		assert.Equal(t, validUsername, dataMap["username"])
		assert.Equal(t, validName, dataMap["full_name"])
		assert.Equal(t, validBio, dataMap["bio"])
	})

	t.Run("Unauthorized", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/user/current", nil)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusUnauthorized, rr.Code) {
			printBody(t, rr)
		}
	})
}

func TestGetUserProfile(t *testing.T) {
	validEmail := "other@example.com"
	validUsername := "otheruser"
	validName := "Other User"
	validBio := "I am another user"

	t.Run("Success", func(t *testing.T) {
		clearDatabase(context.Background())

		targetUser, err := testClient.User.Create().
			SetEmail(validEmail).
			SetUsername(validUsername).
			SetFullName(validName).
			SetBio(validBio).
			Save(context.Background())
		assert.NoError(t, err)

		requestingUser, err := testClient.User.Create().
			SetEmail("requester@example.com").
			SetUsername("requester").
			SetFullName("Requester").
			Save(context.Background())
		assert.NoError(t, err)

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, requestingUser.ID)

		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/users/%d", targetUser.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataMap, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, float64(targetUser.ID), dataMap["id"])
		assert.Nil(t, dataMap["email"])
		assert.Equal(t, validUsername, dataMap["username"])
		assert.Equal(t, validName, dataMap["full_name"])
		assert.Equal(t, validBio, dataMap["bio"])
		assert.False(t, dataMap["is_blocked_by_me"].(bool))
		assert.False(t, dataMap["is_blocked_by_other"].(bool))
	})

	t.Run("Blocked By Me (Should Return OK with flags)", func(t *testing.T) {
		clearDatabase(context.Background())

		targetUser, _ := testClient.User.Create().SetEmail("target@test.com").SetUsername("target").SetFullName("Target").Save(context.Background())
		blockerUser, _ := testClient.User.Create().SetEmail("blocker@test.com").SetUsername("blocker").SetFullName("Blocker").Save(context.Background())

		testClient.UserBlock.Create().SetBlockerID(blockerUser.ID).SetBlockedID(targetUser.ID).Save(context.Background())

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, blockerUser.ID)

		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/users/%d", targetUser.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataMap, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.True(t, dataMap["is_blocked_by_me"].(bool))
		assert.False(t, dataMap["is_blocked_by_other"].(bool))
	})

	t.Run("Blocked By Other (Should Return OK with flags)", func(t *testing.T) {
		clearDatabase(context.Background())

		targetUser, _ := testClient.User.Create().SetEmail("target@test.com").SetUsername("target").SetFullName("Target").Save(context.Background())
		blockerUser, _ := testClient.User.Create().SetEmail("blocker@test.com").SetUsername("blocker").SetFullName("Blocker").Save(context.Background())

		testClient.UserBlock.Create().SetBlockerID(targetUser.ID).SetBlockedID(blockerUser.ID).Save(context.Background())

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, blockerUser.ID)

		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/users/%d", targetUser.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataMap, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.False(t, dataMap["is_blocked_by_me"].(bool))
		assert.True(t, dataMap["is_blocked_by_other"].(bool))

		assert.Nil(t, dataMap["last_seen_at"])
	})

	t.Run("User Not Found", func(t *testing.T) {
		clearDatabase(context.Background())

		requestingUser, err := testClient.User.Create().
			SetEmail("requester@example.com").
			SetUsername("requester").
			SetFullName("Requester").
			Save(context.Background())
		assert.NoError(t, err)

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, requestingUser.ID)

		req, _ := http.NewRequest("GET", "/api/users/99999", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusNotFound, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Invalid ID Format", func(t *testing.T) {
		clearDatabase(context.Background())

		requestingUser, err := testClient.User.Create().
			SetEmail("requester@example.com").
			SetUsername("requester").
			SetFullName("Requester").
			Save(context.Background())
		assert.NoError(t, err)

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, requestingUser.ID)

		req, _ := http.NewRequest("GET", "/api/users/invalid-id", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Unauthorized", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users/1", nil)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusUnauthorized, rr.Code) {
			printBody(t, rr)
		}
	})
}

func TestUpdateProfile(t *testing.T) {
	if testConfig.StorageMode != "local" {
		t.Skip("Skipping Update Profile test: Storage mode is not local")
	}

	validEmail := "profile@example.com"
	validUsername := "profileuser"
	validPassword := "Password123!"

	t.Run("Success Update Info Only", func(t *testing.T) {
		clearDatabase(context.Background())
		cleanupStorage(true)

		hashedPassword, _ := helper.HashPassword(validPassword)
		u, err := testClient.User.Create().
			SetEmail(validEmail).
			SetUsername(validUsername).
			SetFullName("Old Name").
			SetPasswordHash(hashedPassword).
			Save(context.Background())
		assert.NoError(t, err)

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u.ID)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("full_name", "New Name")
		_ = writer.WriteField("bio", "New Bio")
		_ = writer.WriteField("username", "newusername")
		_ = writer.Close()

		req, _ := http.NewRequest("PUT", "/api/user/profile", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataMap, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "New Name", dataMap["full_name"])
		assert.Equal(t, "newusername", dataMap["username"])

		updatedUser, err := testClient.User.Query().Where(user.ID(u.ID)).Only(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "New Name", updatedUser.FullName)
		assert.Equal(t, "New Bio", *updatedUser.Bio)
		assert.Equal(t, "newusername", updatedUser.Username)
	})

	t.Run("Fail Update Username Taken", func(t *testing.T) {
		clearDatabase(context.Background())
		cleanupStorage(true)

		u1, _ := testClient.User.Create().SetEmail("u1@test.com").SetUsername("user1").SetFullName("User 1").Save(context.Background())
		testClient.User.Create().SetEmail("u2@test.com").SetUsername("user2").SetFullName("User 2").Save(context.Background())

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("full_name", "User 1")
		_ = writer.WriteField("username", "user2")
		_ = writer.Close()

		req, _ := http.NewRequest("PUT", "/api/user/profile", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusConflict, rr.Code)
	})

	t.Run("Success Update Avatar", func(t *testing.T) {
		clearDatabase(context.Background())
		cleanupStorage(true)

		hashedPassword, _ := helper.HashPassword(validPassword)
		u, err := testClient.User.Create().
			SetEmail(validEmail).
			SetUsername(validUsername).
			SetFullName("Old Name").
			SetPasswordHash(hashedPassword).
			Save(context.Background())
		assert.NoError(t, err)

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u.ID)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("full_name", "New Name")

		part, _ := writer.CreateFormFile("avatar", "avatar.jpg")
		imgData := createTestImage(t, 400, 400)
		_, _ = io.Copy(part, bytes.NewReader(imgData))
		_ = writer.Close()

		req, _ := http.NewRequest("PUT", "/api/user/profile", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataMap, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		avatarURL, ok := dataMap["avatar"].(string)
		assert.True(t, ok)
		assert.NotEmpty(t, avatarURL)

		parts := strings.Split(avatarURL, "/")
		fileName := parts[len(parts)-1]
		_, b, _, _ := runtime.Caller(0)
		testDir := filepath.Dir(b)
		physicalPath := filepath.Join(testDir, testConfig.StorageProfile, fileName)
		assert.FileExists(t, physicalPath)

		updatedUser, err := testClient.User.Query().Where(user.ID(u.ID)).WithAvatar().Only(context.Background())
		assert.NoError(t, err)
		assert.NotNil(t, updatedUser.Edges.Avatar)
		assert.Equal(t, fileName, updatedUser.Edges.Avatar.FileName)
	})

	t.Run("Delete Avatar", func(t *testing.T) {
		clearDatabase(context.Background())
		cleanupStorage(true)

		u, err := testClient.User.Create().
			SetEmail(validEmail).SetUsername(validUsername).SetFullName("User With Avatar").
			Save(context.Background())
		assert.NoError(t, err)

		media, err := testClient.Media.Create().
			SetFileName("old_avatar.jpg").SetOriginalName("old.jpg").
			SetFileSize(1024).SetMimeType("image/jpeg").
			SetUploader(u).
			Save(context.Background())
		assert.NoError(t, err)

		u, err = testClient.User.UpdateOne(u).SetAvatar(media).Save(context.Background())
		assert.NoError(t, err)

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u.ID)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("full_name", "User With Avatar")
		_ = writer.WriteField("delete_avatar", "true")
		_ = writer.Close()

		req, _ := http.NewRequest("PUT", "/api/user/profile", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		updatedUser, _ := testClient.User.Query().Where(user.ID(u.ID)).WithAvatar().Only(context.Background())
		assert.Nil(t, updatedUser.Edges.Avatar)
	})

	t.Run("Invalid Image Format", func(t *testing.T) {
		clearDatabase(context.Background())
		cleanupStorage(true)

		hashedPassword, _ := helper.HashPassword(validPassword)
		u, err := testClient.User.Create().
			SetEmail(validEmail).
			SetUsername(validUsername).
			SetFullName("User").
			SetPasswordHash(hashedPassword).
			Save(context.Background())
		assert.NoError(t, err)

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u.ID)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("full_name", "User")

		part, _ := writer.CreateFormFile("avatar", "avatar.txt")
		_, _ = io.WriteString(part, "This is not an image")
		_ = writer.Close()

		req, _ := http.NewRequest("PUT", "/api/user/profile", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Image Too Large", func(t *testing.T) {
		clearDatabase(context.Background())
		cleanupStorage(true)

		hashedPassword, _ := helper.HashPassword(validPassword)
		u, err := testClient.User.Create().
			SetEmail(validEmail).
			SetUsername(validUsername).
			SetFullName("User").
			SetPasswordHash(hashedPassword).
			Save(context.Background())
		assert.NoError(t, err)

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u.ID)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("full_name", "User")

		largeData := make([]byte, 3*1024*1024)
		part, _ := writer.CreateFormFile("avatar", "large.jpg")
		_, _ = part.Write(largeData)
		_ = writer.Close()

		req, _ := http.NewRequest("PUT", "/api/user/profile", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Image Dimensions Too Large", func(t *testing.T) {
		clearDatabase(context.Background())
		cleanupStorage(true)

		hashedPassword, _ := helper.HashPassword(validPassword)
		u, err := testClient.User.Create().
			SetEmail(validEmail).
			SetUsername(validUsername).
			SetFullName("User").
			SetPasswordHash(hashedPassword).
			Save(context.Background())
		assert.NoError(t, err)

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u.ID)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("full_name", "User")

		imgData := createTestImage(t, 900, 900)
		part, _ := writer.CreateFormFile("avatar", "large_dim.jpg")
		_, _ = io.Copy(part, bytes.NewReader(imgData))
		_ = writer.Close()

		req, _ := http.NewRequest("PUT", "/api/user/profile", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Unauthorized", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("full_name", "New Name")
		_ = writer.Close()

		req, _ := http.NewRequest("PUT", "/api/user/profile", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusUnauthorized, rr.Code) {
			printBody(t, rr)
		}
	})
}

func TestSearchUsers(t *testing.T) {
	clearDatabase(context.Background())

	names := []string{"David", "Alice", "Charlie", "Bob", "Eve"}
	users := make(map[string]*ent.User)
	for _, name := range names {
		email := strings.ToLower(name) + "@test.com"
		username := strings.ToLower(name)
		hashedPassword, _ := helper.HashPassword("Password123!")
		u, _ := testClient.User.Create().
			SetEmail(email).
			SetUsername(username).
			SetFullName(name).
			SetPasswordHash(hashedPassword).
			Save(context.Background())
		users[name] = u
	}

	searcher, _ := testClient.User.Create().
		SetEmail("searcher@test.com").
		SetUsername("searcher").
		SetFullName("Searcher").
		SetPasswordHash("hash").
		Save(context.Background())
	token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, searcher.ID)

	chatEntity, _ := testClient.Chat.Create().SetType(chat.TypePrivate).Save(context.Background())
	testClient.PrivateChat.Create().
		SetChat(chatEntity).
		SetUser1(searcher).
		SetUser2(users["Alice"]).
		Save(context.Background())

	t.Run("Success - List All (Pagination)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users?limit=2", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 2)
		assert.True(t, resp.Meta.HasNext)
		assert.NotEmpty(t, resp.Meta.NextCursor)

		user1 := dataList[0].(map[string]interface{})
		user2 := dataList[1].(map[string]interface{})
		assert.Equal(t, "Alice", user1["full_name"])
		assert.Equal(t, "Bob", user2["full_name"])

		cursor := resp.Meta.NextCursor
		req2, _ := http.NewRequest("GET", fmt.Sprintf("/api/users?limit=2&cursor=%s", cursor), nil)
		req2.Header.Set("Authorization", "Bearer "+token)
		rr2 := executeRequest(req2)

		if !assert.Equal(t, http.StatusOK, rr2.Code) {
			printBody(t, rr2)
		}
		var resp2 helper.ResponseWithPagination
		json.Unmarshal(rr2.Body.Bytes(), &resp2)

		dataList2 := resp2.Data.([]interface{})
		assert.Len(t, dataList2, 2)

		user3 := dataList2[0].(map[string]interface{})
		user4 := dataList2[1].(map[string]interface{})
		assert.Equal(t, "Charlie", user3["full_name"])
		assert.Equal(t, "David", user4["full_name"])
	})

	t.Run("Success - Search with Private Chat ID (include_chat_id=true)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users?query=Alice&include_chat_id=true", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 1)
		userAlice := dataList[0].(map[string]interface{})
		assert.Equal(t, "Alice", userAlice["full_name"])
		assert.NotNil(t, userAlice["private_chat_id"])
		assert.Equal(t, float64(chatEntity.ID), userAlice["private_chat_id"])
	})

	t.Run("Success - Search with Private Chat ID (include_chat_id=false)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users?query=Alice&include_chat_id=false", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 1)
		userAlice := dataList[0].(map[string]interface{})
		assert.Equal(t, "Alice", userAlice["full_name"])
		assert.Nil(t, userAlice["private_chat_id"])
	})

	t.Run("Success - Search without Private Chat ID", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users?query=Bob", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 1)
		userBob := dataList[0].(map[string]interface{})
		assert.Equal(t, "Bob", userBob["full_name"])
		assert.Nil(t, userBob["private_chat_id"])
	})

	t.Run("Success - Search by Username", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users?query=alice", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 1)
		userAlice := dataList[0].(map[string]interface{})
		assert.Equal(t, "Alice", userAlice["full_name"])
	})

	t.Run("Success - Search by Email Exact Match", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users?query=alice@test.com", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		if assert.NotEmpty(t, dataList) {
			userAlice := dataList[0].(map[string]interface{})
			assert.Equal(t, "Alice", userAlice["full_name"])
		}
	})

	t.Run("Fail - Search by Partial Email (Should be Empty)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users?query=alice@test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 0)
	})

	t.Run("Success - Exclude Blocked Users", func(t *testing.T) {

		testClient.UserBlock.Create().SetBlockerID(searcher.ID).SetBlockedID(users["Bob"].ID).Save(context.Background())

		req, _ := http.NewRequest("GET", "/api/users?query=Bob", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 0, "Blocked user should not appear in search")
	})

	t.Run("Success - Mutual Block (Both should not see each other)", func(t *testing.T) {

		testClient.UserBlock.Create().SetBlockerID(users["Eve"].ID).SetBlockedID(searcher.ID).Save(context.Background())

		testClient.UserBlock.Create().SetBlockerID(searcher.ID).SetBlockedID(users["Eve"].ID).Save(context.Background())

		req, _ := http.NewRequest("GET", "/api/users?query=Eve", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 0, "Mutually blocked user should not appear in search")
	})

	t.Run("Empty Result", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users?query=zoro", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 0)
		assert.False(t, resp.Meta.HasNext)
	})

	t.Run("Invalid Cursor", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users?cursor=invalid-base64-string", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusBadRequest, rr.Code) {
			printBody(t, rr)
		}
	})

	t.Run("Unauthorized", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users", nil)
		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusUnauthorized, rr.Code) {
			printBody(t, rr)
		}
	})
}

func TestGetBlockedUsers(t *testing.T) {
	clearDatabase(context.Background())

	blocker, _ := testClient.User.Create().SetEmail("blocker@test.com").SetUsername("blocker").SetFullName("Blocker").Save(context.Background())
	blocked1, _ := testClient.User.Create().SetEmail("blocked1@test.com").SetUsername("blocked1").SetFullName("Blocked One").Save(context.Background())
	blocked2, _ := testClient.User.Create().SetEmail("blocked2@test.com").SetUsername("blocked2").SetFullName("Blocked Two").Save(context.Background())
	testClient.User.Create().SetEmail("unblocked@test.com").SetUsername("unblocked").SetFullName("Unblocked").Save(context.Background())

	testClient.UserBlock.Create().SetBlockerID(blocker.ID).SetBlockedID(blocked1.ID).Save(context.Background())
	testClient.UserBlock.Create().SetBlockerID(blocker.ID).SetBlockedID(blocked2.ID).Save(context.Background())

	token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, blocker.ID)

	t.Run("Success - List All Blocked Users", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users/blocked", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 2)

		names := make(map[string]bool)
		for _, item := range dataList {
			u := item.(map[string]interface{})
			names[u["username"].(string)] = true
			assert.True(t, u["is_blocked_by_me"].(bool))
		}
		assert.True(t, names["blocked1"])
		assert.True(t, names["blocked2"])
		assert.False(t, names["unblocked"])
	})

	t.Run("Success - Search Blocked User", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users/blocked?query=One", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 1)
		assert.Equal(t, "Blocked One", dataList[0].(map[string]interface{})["full_name"])
	})

	t.Run("Success - Empty List", func(t *testing.T) {

		cleanUser, _ := testClient.User.Create().SetEmail("clean@test.com").SetUsername("clean").SetFullName("Clean").Save(context.Background())
		cleanToken, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, cleanUser.ID)

		req, _ := http.NewRequest("GET", "/api/users/blocked", nil)
		req.Header.Set("Authorization", "Bearer "+cleanToken)
		rr := executeRequest(req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Empty(t, resp.Data)
	})
}

func TestBlockUser(t *testing.T) {
	clearDatabase(context.Background())

	u1, _ := testClient.User.Create().SetEmail("u1@test.com").SetUsername("u1").SetFullName("User 1").Save(context.Background())
	u2, _ := testClient.User.Create().SetEmail("u2@test.com").SetUsername("u2").SetFullName("User 2").Save(context.Background())

	token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)

	t.Run("Success Block", func(t *testing.T) {
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/users/%d/block", u2.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		exists, _ := testClient.UserBlock.Query().Where(userblock.BlockerID(u1.ID), userblock.BlockedID(u2.ID)).Exist(context.Background())
		assert.True(t, exists)
	})

	t.Run("Success Unblock", func(t *testing.T) {
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/users/%d/unblock", u2.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		exists, _ := testClient.UserBlock.Query().Where(userblock.BlockerID(u1.ID), userblock.BlockedID(u2.ID)).Exist(context.Background())
		assert.False(t, exists)
	})

	t.Run("Block Self", func(t *testing.T) {
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/users/%d/block", u1.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Block Non-Existent User", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/api/users/99999/block", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}
