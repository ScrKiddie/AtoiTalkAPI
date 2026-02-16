package model

import (
	"mime/multipart"

	"github.com/google/uuid"
)

type UploadMediaRequest struct {
	File         *multipart.FileHeader `form:"file" validate:"required"`
	CaptchaToken string                `form:"captcha_token" validate:"required"`
}

type MediaDTO struct {
	ID           uuid.UUID `json:"id"`
	FileName     string    `json:"file_name"`
	OriginalName string    `json:"original_name"`
	FileSize     int64     `json:"file_size"`
	MimeType     string    `json:"mime_type"`
	URL          string    `json:"url"`
}

type MediaURLResponse struct {
	URL string `json:"url"`
}
