package controller

import (
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/middleware"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/service"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type GroupChatController struct {
	groupChatService *service.GroupChatService
}

func NewGroupChatController(groupChatService *service.GroupChatService) *GroupChatController {
	return &GroupChatController{
		groupChatService: groupChatService,
	}
}

// CreateGroupChat godoc
// @Summary      Create Group Chat
// @Description  Create a new group chat with multiple members and an optional avatar.
// @Tags         chat
// @Accept       multipart/form-data
// @Produce      json
// @Param        name formData string true "Group Name"
// @Param        description formData string false "Group Description"
// @Param        member_ids formData string true "JSON Array of Member IDs (e.g. [1, 2, 3])"
// @Param        avatar formData file false "Group Avatar Image"
// @Success      200  {object}  helper.ResponseSuccess{data=model.ChatResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group [post]
func (c *GroupChatController) CreateGroupChat(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	memberIDsStr := r.FormValue("member_ids")

	var memberIDs []int
	if memberIDsStr != "" {
		if err := json.Unmarshal([]byte(memberIDsStr), &memberIDs); err != nil {
			parts := strings.Split(memberIDsStr, ",")
			for _, p := range parts {
				id, err := strconv.Atoi(strings.TrimSpace(p))
				if err == nil {
					memberIDs = append(memberIDs, id)
				}
			}
		}
	}

	_, header, err := r.FormFile("avatar")
	if err != nil && err != http.ErrMissingFile {
		helper.WriteError(w, helper.NewBadRequestError("Failed to process avatar file"))
		return
	}

	req := model.CreateGroupChatRequest{
		Name:        name,
		Description: description,
		MemberIDs:   memberIDs,
		Avatar:      header,
	}

	resp, err := c.groupChatService.CreateGroupChat(r.Context(), userContext.ID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}
