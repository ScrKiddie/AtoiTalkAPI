package test

import (
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/media"
	"AtoiTalkAPI/ent/message"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"testing"

	"AtoiTalkAPI/internal/helper"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
)

func TestUploadMedia(t *testing.T) {
	clearDatabase(context.Background())

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

		_ = writer.WriteField("captcha_token", "dummy-token")

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

		fileName := dataMap["file_name"].(string)
		_, err := s3Client.HeadObject(context.Background(), &s3.HeadObjectInput{
			Bucket: aws.String(testConfig.S3BucketPrivate),
			Key:    aws.String(fileName),
		})
		assert.NoError(t, err, "Uploaded file should exist in S3")
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
		_ = writer.WriteField("captcha_token", "dummy-token")
		_ = writer.Close()

		req, _ := http.NewRequest("POST", "/api/media/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Missing Captcha", func(t *testing.T) {
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
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Fail - Unauthorized", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/api/media/upload", nil)
		rr := executeRequest(req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestGetMediaURL(t *testing.T) {
	clearDatabase(context.Background())

	u1 := createTestUser(t, "user1")
	u2 := createTestUser(t, "user2")
	u3 := createTestUser(t, "user3")

	token1, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u1.ID)
	token3, _ := helper.GenerateJWT(testConfig.JWTSecret, testConfig.JWTExp, u3.ID)

	chatPrivate, _ := testClient.Chat.Create().SetType(chat.TypePrivate).Save(context.Background())
	testClient.PrivateChat.Create().SetChat(chatPrivate).SetUser1(u1).SetUser2(u2).Save(context.Background())

	chatGroup, _ := testClient.Chat.Create().SetType(chat.TypeGroup).Save(context.Background())
	gc, _ := testClient.GroupChat.Create().SetChat(chatGroup).SetCreator(u1).SetName("Test Group").SetInviteCode("test").Save(context.Background())
	testClient.GroupMember.Create().SetGroupChat(gc).SetUser(u1).SetRole(groupmember.RoleOwner).Save(context.Background())

	mediaPrivate, _ := testClient.Media.Create().
		SetFileName("private.jpg").SetOriginalName("private.jpg").SetFileSize(100).SetMimeType("image/jpeg").
		SetStatus(media.StatusActive).SetUploader(u1).Save(context.Background())
	testClient.Message.Create().SetChat(chatPrivate).SetSender(u1).SetType(message.TypeRegular).AddAttachments(mediaPrivate).Save(context.Background())

	mediaGroup, _ := testClient.Media.Create().
		SetFileName("group.jpg").SetOriginalName("group.jpg").SetFileSize(100).SetMimeType("image/jpeg").
		SetStatus(media.StatusActive).SetUploader(u1).Save(context.Background())
	testClient.Message.Create().SetChat(chatGroup).SetSender(u1).SetType(message.TypeRegular).AddAttachments(mediaGroup).Save(context.Background())

	s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(testConfig.S3BucketPrivate),
		Key:    aws.String("private.jpg"),
		Body:   bytes.NewReader([]byte("content")),
	})
	s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(testConfig.S3BucketPrivate),
		Key:    aws.String("group.jpg"),
		Body:   bytes.NewReader([]byte("content")),
	})

	t.Run("Success - Refresh URL (Private Chat)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/media/%s/url", mediaPrivate.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataMap := resp.Data.(map[string]interface{})
		assert.NotEmpty(t, dataMap["url"])
	})

	t.Run("Success - Refresh URL (Group Chat)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/media/%s/url", mediaGroup.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp helper.ResponseSuccess
		json.Unmarshal(rr.Body.Bytes(), &resp)
		dataMap := resp.Data.(map[string]interface{})
		assert.NotEmpty(t, dataMap["url"])
	})

	t.Run("Fail - Not Member (Private Chat)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/media/%s/url", mediaPrivate.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token3)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Fail - Not Member (Group Chat)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/media/%s/url", mediaGroup.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token3)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Fail - Media Not Found", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/media/%s/url", "00000000-0000-0000-0000-000000000000"), nil)
		req.Header.Set("Authorization", "Bearer "+token1)

		rr := executeRequest(req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}
