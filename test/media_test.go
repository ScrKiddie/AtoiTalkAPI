package test

import (
	"AtoiTalkAPI/ent/media"
	"AtoiTalkAPI/internal/helper"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUploadMedia(t *testing.T) {
	if testConfig.StorageMode != "local" {
		t.Skip("Skipping Upload Media test: Storage mode is not local")
	}

	clearDatabase(context.Background())
	cleanupStorage(true)

	u := createTestUser(t, "uploader")
	token, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u.ID)

	t.Run("Success - Upload Image", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", `form-data; name="file"; filename="test_image.jpg"`)
		h.Set("Content-Type", "image/jpeg")
		part, _ := writer.CreatePart(h)

		imgData := createTestImage(t, 100, 100)
		_, _ = io.Copy(part, bytes.NewReader(imgData))
		_ = writer.Close()

		req, _ := http.NewRequest("POST", "/api/media/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if !assert.Equal(t, http.StatusOK, rr.Code) {
			printBody(t, rr)
		}

		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataMap := resp.Data.(map[string]interface{})

		assert.NotEmpty(t, dataMap["id"])
		assert.Equal(t, "test_image.jpg", dataMap["original_name"])
		assert.Equal(t, "image/jpeg", dataMap["mime_type"])
		assert.NotEmpty(t, dataMap["url"])

		mediaIDStr := dataMap["id"].(string)

		m, err := testClient.Media.Query().Where(media.FileName(dataMap["file_name"].(string))).Only(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, mediaIDStr, m.ID.String())
		assert.Equal(t, media.StatusActive, m.Status)
		assert.Equal(t, u.ID, m.UploadedByID, "Media uploader should be set to the user")

		_, b, _, _ := runtime.Caller(0)
		testDir := filepath.Dir(b)
		physicalPath := filepath.Join(testDir, testConfig.StorageAttachment, m.FileName)
		assert.FileExists(t, physicalPath)
	})

	t.Run("Fail - Invalid Form Data", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/api/media/upload", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Missing File", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.Close()

		req, _ := http.NewRequest("POST", "/api/media/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Unauthorized", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/api/media/upload", nil)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Fail - Upload Fake Image (MIME Spoofing)", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", `form-data; name="file"; filename="virus.jpg"`)
		h.Set("Content-Type", "image/jpeg")
		part, _ := writer.CreatePart(h)

		_, _ = io.WriteString(part, "<?php echo 'hacked'; ?>")
		_ = writer.Close()

		req, _ := http.NewRequest("POST", "/api/media/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)

		if assert.Equal(t, http.StatusOK, rr.Code) {
			var resp helper.ResponseSuccess
			json.Unmarshal(rr.Body.Bytes(), &resp)
			dataMap := resp.Data.(map[string]interface{})

			assert.Equal(t, "application/octet-stream", dataMap["mime_type"])
		}
	})
}
