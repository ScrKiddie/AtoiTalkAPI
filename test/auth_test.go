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
)

func TestGoogleExchange(t *testing.T) {

	clearDatabase(context.Background())

	t.Run("Validation Error", func(t *testing.T) {
		reqBody := model.GoogleLoginRequest{
			Code: "",
		}
		body, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest("POST", "/api/auth/google", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rr.Code)
		}

		var resp helper.Response
		json.Unmarshal(rr.Body.Bytes(), &resp)
		if resp.Status {
			t.Errorf("Expected status false, got true")
		}
	})

	t.Run("Invalid Token", func(t *testing.T) {
		reqBody := model.GoogleLoginRequest{
			Code: "invalid-token-string",
		}
		body, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest("POST", "/api/auth/google", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := executeRequest(req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
		}

		var resp helper.Response
		json.Unmarshal(rr.Body.Bytes(), &resp)
		if resp.Status {
			t.Errorf("Expected status false, got true")
		}
	})

	t.Run("Valid Token", func(t *testing.T) {
		validToken := os.Getenv("TEST_GOOGLE_ID_TOKEN")
		if validToken == "" {
			t.Skip("Skipping Valid Token test: TEST_GOOGLE_ID_TOKEN not set")
		}

		makeRequest := func() *httptest.ResponseRecorder {
			reqBody := model.GoogleLoginRequest{
				Code: validToken,
			}
			body, _ := json.Marshal(reqBody)

			req, _ := http.NewRequest("POST", "/api/auth/google", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			return executeRequest(req)
		}

		t.Run("Register", func(t *testing.T) {
			rr := makeRequest()

			if rr.Code != http.StatusOK {
				t.Errorf("Expected status code %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
			}

			var resp helper.Response
			json.Unmarshal(rr.Body.Bytes(), &resp)

			if !resp.Status {
				t.Errorf("Expected status true, got false")
			}

			dataMap, ok := resp.Data.(map[string]interface{})
			if !ok {
				t.Errorf("Expected data to be a map")
				return
			}

			if _, ok := dataMap["token"]; !ok {
				t.Errorf("Expected token in response data")
			}

			userMap, ok := dataMap["user"].(map[string]interface{})
			if !ok {
				t.Errorf("Expected user object in response data")
				return
			}

			if _, ok := userMap["email"]; !ok {
				t.Errorf("Expected email in user data")
			}

			avatarURL, ok := userMap["avatar"].(string)
			if ok && avatarURL != "" {

				parts := strings.Split(avatarURL, "/")
				fileName := parts[len(parts)-1]

				_, b, _, _ := runtime.Caller(0)
				testDir := filepath.Dir(b)
				physicalPath := filepath.Join(testDir, testConfig.StorageProfile, fileName)

				if _, err := os.Stat(physicalPath); os.IsNotExist(err) {
					t.Errorf("Profile picture file was not created at %s", physicalPath)
				} else {
					t.Logf("Profile picture successfully saved at %s", physicalPath)
				}
			} else {
				t.Log("No avatar URL returned, skipping file check (maybe token has no picture)")
			}
		})

		t.Run("Login Existing User", func(t *testing.T) {
			rr := makeRequest()

			if rr.Code != http.StatusOK {
				t.Errorf("Expected status code %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
			}

			var resp helper.Response
			json.Unmarshal(rr.Body.Bytes(), &resp)

			if !resp.Status {
				t.Errorf("Expected status true, got false")
			}
		})
	})
}
