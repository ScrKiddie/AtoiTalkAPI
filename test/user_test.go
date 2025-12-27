package test

import (
	"AtoiTalkAPI/ent/user"
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
	validName := "Current User"
	validBio := "I am current user"

	t.Run("Success", func(t *testing.T) {
		clearDatabase(context.Background())

		u, err := testClient.User.Create().
			SetEmail(validEmail).
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

func TestUpdateProfile(t *testing.T) {
	if testConfig.StorageMode != "local" {
		t.Skip("Skipping Update Profile test: Storage mode is not local")
	}

	validEmail := "profile@example.com"
	validPassword := "Password123!"

	t.Run("Success Update Info Only", func(t *testing.T) {
		clearDatabase(context.Background())
		cleanupStorage(true)

		hashedPassword, _ := helper.HashPassword(validPassword)
		u, err := testClient.User.Create().
			SetEmail(validEmail).
			SetFullName("Old Name").
			SetPasswordHash(hashedPassword).
			Save(context.Background())
		assert.NoError(t, err)

		token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u.ID)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("full_name", "New Name")
		_ = writer.WriteField("bio", "New Bio")
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

		updatedUser, err := testClient.User.Query().Where(user.ID(u.ID)).Only(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "New Name", updatedUser.FullName)
		assert.Equal(t, "New Bio", *updatedUser.Bio)
	})

	t.Run("Success Update Avatar", func(t *testing.T) {
		clearDatabase(context.Background())
		cleanupStorage(true)

		hashedPassword, _ := helper.HashPassword(validPassword)
		u, err := testClient.User.Create().
			SetEmail(validEmail).
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

		media, err := testClient.Media.Create().
			SetFileName("old_avatar.jpg").SetOriginalName("old.jpg").
			SetFileSize(1024).SetMimeType("image/jpeg").
			Save(context.Background())
		assert.NoError(t, err)

		u, err := testClient.User.Create().
			SetEmail(validEmail).SetFullName("User With Avatar").
			SetAvatar(media).Save(context.Background())
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

	names := []string{"Alice", "Bob", "Charlie", "David", "Eve"}
	for _, name := range names {
		email := strings.ToLower(name) + "@test.com"
		hashedPassword, _ := helper.HashPassword("Password123!")
		testClient.User.Create().
			SetEmail(email).
			SetFullName(name).
			SetPasswordHash(hashedPassword).
			Save(context.Background())
	}

	searcher, _ := testClient.User.Create().
		SetEmail("searcher@test.com").
		SetFullName("Searcher").
		SetPasswordHash("hash").
		Save(context.Background())
	token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, searcher.ID)

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

	t.Run("Success - Search by Name", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users?query=ali", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 1)
		user1 := dataList[0].(map[string]interface{})
		assert.Equal(t, "Alice", user1["full_name"])
	})

	t.Run("Success - Search by Email", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users?query=bob@test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}
		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 1)
		user1 := dataList[0].(map[string]interface{})
		assert.Equal(t, "Bob", user1["full_name"])
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
