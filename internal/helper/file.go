package helper

import (
	"AtoiTalkAPI/internal/config"
	"fmt"
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

	uniqueName := fmt.Sprintf("%d-%s%s", time.Now().UnixNano(), uuid.New().String(), ext)

	return uniqueName
}

func BuildImageURL(cfg *config.AppConfig, folderPathFromConfig string, fileName string) string {
	if fileName == "" {
		return ""
	}

	var baseURL string
	if cfg.StorageMode == "local" {
		baseURL = cfg.AppURL
	} else {
		baseURL = cfg.StorageCDNURL
	}

	cleanInput := strings.TrimLeft(folderPathFromConfig, "/\\.")

	cleanPath := filepath.ToSlash(filepath.Join(".", cleanInput))

	return fmt.Sprintf("%s/%s/%s", baseURL, cleanPath, fileName)
}
