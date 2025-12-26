package controller

import (
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/middleware"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/service"
	"log/slog"
	"net/http"
	"strconv"
)

type UserController struct {
	userService *service.UserService
}

func NewUserController(userService *service.UserService) *UserController {
	return &UserController{
		userService: userService,
	}
}

// UpdateProfile godoc
// @Summary      Update User Profile
// @Description  Update user's full name, bio, and avatar.
// @Tags         user
// @Accept       multipart/form-data
// @Produce      json
// @Param        full_name formData string true "Full Name"
// @Param        bio formData string false "Bio"
// @Param        avatar formData file false "Avatar Image"
// @Param        delete_avatar formData boolean false "Delete Avatar"
// @Success      200  {object}  helper.ResponseSuccess{data=model.UserDTO}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/user/profile [put]
func (c *UserController) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		slog.Warn("Failed to parse multipart form", "error", err)
		helper.WriteError(w, helper.NewBadRequestError("Invalid form data"))
		return
	}

	var req model.UpdateProfileRequest
	req.FullName = r.FormValue("full_name")
	req.Bio = r.FormValue("bio")
	req.DeleteAvatar, _ = strconv.ParseBool(r.FormValue("delete_avatar"))

	file, header, err := r.FormFile("avatar")
	if err == nil {
		defer file.Close()
		req.Avatar = header
	} else if err != http.ErrMissingFile {
		slog.Warn("Error retrieving avatar file", "error", err)
		helper.WriteError(w, helper.NewBadRequestError("Invalid avatar file"))
		return
	}

	resp, err := c.userService.UpdateProfile(r.Context(), userContext.ID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}
