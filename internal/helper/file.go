package helper

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func GenerateUniqueFileName(originalName string) string {
	ext := filepath.Ext(originalName)
	if ext == "" {
		ext = ".jpg"
	}

	ext = strings.ToLower(ext)

	uniqueName := fmt.Sprintf("%d-%s%s", time.Now().UTC().UnixNano(), uuid.New().String(), ext)

	return uniqueName
}

func BuildImageURL(storageMode, appURL, storageCDNURL, folderPathFromConfig, fileName string) string {
	if fileName == "" {
		return ""
	}

	var baseURL string
	if storageMode == "local" {
		baseURL = appURL
	} else {
		baseURL = storageCDNURL
	}

	cleanInput := strings.TrimLeft(folderPathFromConfig, "/\\.")

	cleanPath := filepath.ToSlash(filepath.Join(".", cleanInput))

	return fmt.Sprintf("%s/%s/%s", baseURL, cleanPath, fileName)
}

func DetectFileContentType(file multipart.File) (string, error) {
	buffer := make([]byte, 512)
	_, err := file.Read(buffer)
	if err != nil {
		return "", err
	}

	contentType := http.DetectContentType(buffer)

	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}

	return contentType, nil
}
