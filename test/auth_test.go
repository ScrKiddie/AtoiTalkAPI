package test

import (
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGoogleExchange(t *testing.T) {
	clearDatabase(context.Background())

	t.Run("Validation Error", func(t *testing.T) {
		reqBody := model.GoogleLoginRequest{Code: ""}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/google", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var resp helper.ResponseError
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.NotEmpty(t, resp.Error)
	})

	t.Run("Invalid Token", func(t *testing.T) {
		reqBody := model.GoogleLoginRequest{Code: "invalid-token-string"}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/api/auth/google", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		var resp helper.ResponseError
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.NotEmpty(t, resp.Error)
	})

	t.Run("Valid Token", func(t *testing.T) {
		validToken := os.Getenv("TEST_GOOGLE_ID_TOKEN")
		if validToken == "" {
			t.Skip("Skipping Valid Token test: TEST_GOOGLE_ID_TOKEN not set")
		}

		makeRequest := func() *httptest.ResponseRecorder {
			reqBody := model.GoogleLoginRequest{Code: validToken}
			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/api/auth/google", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			return executeRequest(req)
		}

		t.Run("Register", func(t *testing.T) {
			rr := makeRequest()
			assert.Equal(t, http.StatusOK, rr.Code, "Response body: %s", rr.Body.String())

			var resp helper.ResponseSuccess
			err := json.Unmarshal(rr.Body.Bytes(), &resp)
			assert.NoError(t, err)

			dataMap, ok := resp.Data.(map[string]interface{})
			assert.True(t, ok, "Expected data to be a map")
			assert.Contains(t, dataMap, "token")

			userMap, ok := dataMap["user"].(map[string]interface{})
			assert.True(t, ok, "Expected user object in response data")
			assert.Contains(t, userMap, "email")

			if avatarURL, ok := userMap["avatar"].(string); ok && avatarURL != "" {
				parts := strings.Split(avatarURL, "/")
				fileName := parts[len(parts)-1]
				_, b, _, _ := runtime.Caller(0)
				testDir := filepath.Dir(b)
				physicalPath := filepath.Join(testDir, testConfig.StorageProfile, fileName)
				assert.FileExists(t, physicalPath, "Profile picture file should be created")
			} else {
				t.Log("No avatar URL returned, skipping file check")
			}
		})

		t.Run("Login Existing User", func(t *testing.T) {
			rr := makeRequest()
			assert.Equal(t, http.StatusOK, rr.Code, "Response body: %s", rr.Body.String())

			var resp helper.ResponseSuccess
			err := json.Unmarshal(rr.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.NotNil(t, resp.Data)
		})
	})
}
