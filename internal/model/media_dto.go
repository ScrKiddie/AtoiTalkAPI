package model

import "mime/multipart"

type MediaDTO struct {
	ID           int    `json:"id"`
	FileName     string `json:"file_name"`
	OriginalName string `json:"original_name"`
	FileSize     int64  `json:"file_size"`
	MimeType     string `json:"mime_type"`
	URL          string `json:"url"`
}

type UploadMediaRequest struct {
	File *multipart.FileHeader `form:"file" validate:"required,filesize=50"`
}
