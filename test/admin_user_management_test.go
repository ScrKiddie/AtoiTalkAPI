package test

import (
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

func TestAdminGetUsers(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)

	createUser := func(prefix string, role user.Role) *struct {
		ID    string
		Email string
		Name  string
	} {
		email := fmt.Sprintf("%s_%d@test.com", prefix, time.Now().UnixNano())
		username := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())

		u, err := testClient.User.Create().
			SetEmail(email).
			SetUsername(username).
			SetFullName(prefix + " User").
			SetPasswordHash(hashedPassword).
			SetRole(role).
			Save(context.Background())
		if err != nil {
			t.Fatalf("Failed to create user %s: %v", prefix, err)
		}
		return &struct {
			ID    string
			Email string
			Name  string
		}{ID: u.ID.String(), Email: email, Name: prefix + " User"}
	}

	admin := createUser("admin", user.RoleAdmin)
	createUser("user1", user.RoleUser)
	createUser("user2", user.RoleUser)
	createUser("user3", user.RoleUser)

	regularUser := createUser("regular", user.RoleUser)

	loginPayload := map[string]string{
		"email":         admin.Email,
		"password":      password,
		"captcha_token": cfTurnstileAlwaysPasses,
	}
	body, _ := json.Marshal(loginPayload)
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequest(req)
	var loginResp struct {
		Data struct {
			AccessToken string `json:"token"`
		} `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &loginResp)
	adminToken := loginResp.Data.AccessToken

	regularToken, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, testClient.User.Query().Where(user.EmailEQ(regularUser.Email)).OnlyX(context.Background()).ID)

	t.Run("Success - List All Users", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/admin/users", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.GreaterOrEqual(t, len(dataList), 5)
	})

	t.Run("Success - Filter by Role", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/admin/users?role=admin", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.GreaterOrEqual(t, len(dataList), 1)

		for _, item := range dataList {
			u := item.(map[string]interface{})
			assert.Equal(t, "admin", u["role"])
		}
	})

	t.Run("Success - Search by Query", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/admin/users?query=user1", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.GreaterOrEqual(t, len(dataList), 1)
	})

	t.Run("Success - Pagination", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/admin/users?limit=2", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseWithPagination
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataList := resp.Data.([]interface{})
		assert.Len(t, dataList, 2)
		assert.True(t, resp.Meta.HasNext)
		assert.NotEmpty(t, resp.Meta.NextCursor)

		req2, _ := http.NewRequest("GET", fmt.Sprintf("/api/admin/users?limit=2&cursor=%s", resp.Meta.NextCursor), nil)
		req2.Header.Set("Authorization", "Bearer "+adminToken)
		rr2 := executeRequest(req2)
		assert.Equal(t, http.StatusOK, rr2.Code)

		var resp2 helper.ResponseWithPagination
		json.Unmarshal(rr2.Body.Bytes(), &resp2)

		dataList2 := resp2.Data.([]interface{})
		assert.NotNil(t, dataList2, "Second page should return valid data")
	})

	t.Run("Fail - Forbidden for Regular User", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/admin/users", nil)
		req.Header.Set("Authorization", "Bearer "+regularToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Fail - Unauthorized", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/admin/users", nil)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestAdminGetUserDetail(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)

	admin, _ := testClient.User.Create().
		SetEmail("admin_detail@test.com").
		SetUsername("admin_detail").
		SetFullName("Admin Detail").
		SetPasswordHash(hashedPassword).
		SetRole(user.RoleAdmin).
		Save(context.Background())

	targetUser, _ := testClient.User.Create().
		SetEmail("target_detail@test.com").
		SetUsername("target_detail").
		SetFullName("Target User").
		SetBio("Test bio").
		SetPasswordHash(hashedPassword).
		SetRole(user.RoleUser).
		Save(context.Background())

	regularUser, _ := testClient.User.Create().
		SetEmail("regular_detail@test.com").
		SetUsername("regular_detail").
		SetFullName("Regular User").
		SetPasswordHash(hashedPassword).
		SetRole(user.RoleUser).
		Save(context.Background())

	adminToken, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, admin.ID)
	regularToken, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, regularUser.ID)

	t.Run("Success - Get User Detail", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/admin/users/%s", targetUser.ID), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)

		dataMap := resp.Data.(map[string]interface{})
		assert.Equal(t, targetUser.ID.String(), dataMap["id"])
		assert.Equal(t, "target_detail", dataMap["username"])
		assert.Equal(t, "Target User", dataMap["full_name"])
		assert.Equal(t, "Test bio", dataMap["bio"])
		assert.Equal(t, "user", dataMap["role"])
	})

	t.Run("Fail - User Not Found", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/admin/users/00000000-0000-0000-0000-000000000000", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Fail - Invalid UUID", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/admin/users/invalid-uuid", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Forbidden for Regular User", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/admin/users/%s", targetUser.ID), nil)
		req.Header.Set("Authorization", "Bearer "+regularToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})
}

func TestAdminResetUserInfo(t *testing.T) {
	clearDatabase(context.Background())

	password := "Password123!"
	hashedPassword, _ := helper.HashPassword(password)

	admin, _ := testClient.User.Create().
		SetEmail("admin_reset@test.com").
		SetUsername("admin_reset").
		SetFullName("Admin Reset").
		SetPasswordHash(hashedPassword).
		SetRole(user.RoleAdmin).
		Save(context.Background())

	adminToken, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, admin.ID)

	t.Run("Success - Reset Bio", func(t *testing.T) {
		targetUser, _ := testClient.User.Create().
			SetEmail("reset_bio@test.com").
			SetUsername("reset_bio").
			SetFullName("Reset Bio User").
			SetBio("This is my bio").
			SetPasswordHash(hashedPassword).
			Save(context.Background())

		reqBody := model.ResetUserInfoRequest{
			TargetUserID: targetUser.ID,
			ResetBio:     true,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/admin/users/%s/reset", targetUser.ID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		u, _ := testClient.User.Query().Where(user.ID(targetUser.ID)).Only(context.Background())
		assert.Nil(t, u.Bio)
	})

	t.Run("Success - Reset Name", func(t *testing.T) {
		targetUser, _ := testClient.User.Create().
			SetEmail("reset_name@test.com").
			SetUsername("reset_name").
			SetFullName("Bad Name").
			SetPasswordHash(hashedPassword).
			Save(context.Background())

		reqBody := model.ResetUserInfoRequest{
			TargetUserID: targetUser.ID,
			ResetName:    true,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/admin/users/%s/reset", targetUser.ID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		u, _ := testClient.User.Query().Where(user.ID(targetUser.ID)).Only(context.Background())
		assert.Contains(t, *u.FullName, "User ")
	})

	t.Run("Success - Reset Avatar (Soft Delete)", func(t *testing.T) {
		targetUser, _ := testClient.User.Create().
			SetEmail("reset_avatar@test.com").
			SetUsername("reset_avatar").
			SetFullName("Reset Avatar User").
			SetPasswordHash(hashedPassword).
			Save(context.Background())

		media, _ := testClient.Media.Create().
			SetFileName("bad_avatar.jpg").
			SetOriginalName("bad.jpg").
			SetFileSize(1024).
			SetMimeType("image/jpeg").
			SetUploader(targetUser).
			Save(context.Background())

		testClient.User.UpdateOne(targetUser).SetAvatar(media).ExecX(context.Background())

		reqBody := model.ResetUserInfoRequest{
			TargetUserID: targetUser.ID,
			ResetAvatar:  true,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/admin/users/%s/reset", targetUser.ID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		u, _ := testClient.User.Query().Where(user.ID(targetUser.ID)).WithAvatar().Only(context.Background())
		assert.Nil(t, u.Edges.Avatar, "Avatar relation should be cleared")

		mediaStillExists, _ := testClient.Media.Query().Where().Exist(context.Background())
		assert.True(t, mediaStillExists, "Media record should still exist for scheduler cleanup")
	})

	t.Run("Fail - User Not Found", func(t *testing.T) {

		nonExistentID := "01900000-0000-7000-8000-000000000001"
		reqBody := map[string]interface{}{
			"reset_bio": true,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/admin/users/%s/reset", nonExistentID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Fail - Forbidden for Regular User", func(t *testing.T) {
		regularUser, _ := testClient.User.Create().
			SetEmail("regular_forbidden@test.com").
			SetUsername("regular_forbidden").
			SetFullName("Regular Forbidden").
			SetPasswordHash(hashedPassword).
			Save(context.Background())

		regularToken, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, regularUser.ID)

		reqBody := model.ResetUserInfoRequest{
			TargetUserID: admin.ID,
			ResetBio:     true,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/admin/users/%s/reset", admin.ID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+regularToken)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})
}
