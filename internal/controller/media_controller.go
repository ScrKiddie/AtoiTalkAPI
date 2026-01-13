package controller

import (
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/middleware"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/service"
	"log/slog"
	"net/http"
)

type MediaController struct {
	mediaService *service.MediaService
}

func NewMediaController(mediaService *service.MediaService) *MediaController {
	return &MediaController{
		mediaService: mediaService,
	}
}

// UploadMedia godoc
// @Summary      Upload Media
// @Description  Upload a file (image, video, file, audio) to be used as attachment.
// @Tags         media
// @Accept       multipart/form-data
// @Produce      json
// @Param        file formData file true "File to upload"
// @Success      200  {object}  helper.ResponseSuccess{data=model.MediaDTO}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      429  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/media/upload [post]
func (c *MediaController) UploadMedia(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		slog.Warn("Error retrieving file", "error", err)
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}
	defer file.Close()

	req := model.UploadMediaRequest{
		File: header,
	}

	resp, err := c.mediaService.UploadMedia(r.Context(), user.ID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}
