package adapter

import (
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type StorageAdapter struct {
	mode       string
	client     *s3.Client
	bucket     string
	httpClient *http.Client
}

func NewStorageAdapter(cfg *config.AppConfig, s3Client *s3.Client, httpClient *http.Client) *StorageAdapter {
	return &StorageAdapter{
		mode:       cfg.StorageMode,
		client:     s3Client,
		bucket:     cfg.S3Bucket,
		httpClient: httpClient,
	}
}

func (s *StorageAdapter) Store(file *multipart.FileHeader, path string) error {
	fileOpened, err := file.Open()
	if err != nil {
		return err
	}
	defer fileOpened.Close()

	contentType := file.Header.Get("Content-Type")
	return s.StoreFromReader(fileOpened, contentType, path)
}

func (s *StorageAdapter) StoreFromReader(reader io.Reader, contentType string, path string) error {
	if s.mode == "s3" {
		if s.client == nil {
			return errors.New("s3 client is not initialized")
		}
		s3Key := filepath.ToSlash(path)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		_, err := s.client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket:      aws.String(s.bucket),
			Key:         aws.String(s3Key),
			Body:        reader,
			ContentType: aws.String(contentType),
		})
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	fileStored, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fileStored.Close()

	_, err = io.Copy(fileStored, reader)
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func (s *StorageAdapter) Download(url string) ([]byte, string, error) {
	operation := func() (*http.Response, bool, error) {
		resp, err := s.httpClient.Get(url)
		if helper.ShouldRetryHTTP(resp, err) {
			if resp != nil {
				resp.Body.Close()
			}
			return nil, true, err
		}
		if err != nil {
			return nil, false, err
		}

		if resp.StatusCode != http.StatusOK {
			defer resp.Body.Close()
			return nil, false, fmt.Errorf("failed to download image, status code: %d", resp.StatusCode)
		}

		return resp, false, nil
	}

	resp, err := helper.RetryWithBackoff(operation, 3, 500*time.Millisecond)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	contentType := http.DetectContentType(data)
	return data, contentType, nil
}

func (s *StorageAdapter) Delete(path string) error {
	if s.mode == "s3" {
		if s.client == nil {

			return errors.New("s3 client is not initialized")
		}
		s3Key := filepath.ToSlash(path)
		_, err := s.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(s3Key),
		})
		return err
	}

	err := os.Remove(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Warn("Attempted to delete a non-existent local file", "path", path)
			return nil
		}
		return err
	}

	return nil
}
