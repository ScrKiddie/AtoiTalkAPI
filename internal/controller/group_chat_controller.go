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

	"github.com/go-chi/chi/v5"
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
// @Param        member_usernames formData string true "JSON Array of Member Usernames (e.g. [\"user1\", \"user2\"])"
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
	memberUsernamesStr := r.FormValue("member_usernames")

	var memberUsernames []string
	if memberUsernamesStr != "" {
		if err := json.Unmarshal([]byte(memberUsernamesStr), &memberUsernames); err != nil {
			parts := strings.Split(memberUsernamesStr, ",")
			for _, p := range parts {
				username := strings.TrimSpace(p)
				if username != "" {
					memberUsernames = append(memberUsernames, username)
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
		Name:            name,
		Description:     description,
		MemberUsernames: memberUsernames,
		Avatar:          header,
	}

	resp, err := c.groupChatService.CreateGroupChat(r.Context(), userContext.ID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}

// UpdateGroupChat godoc
// @Summary      Update Group Chat Info
// @Description  Update group name, description, or avatar. Only owners or admins can perform this action.
// @Tags         chat
// @Accept       multipart/form-data
// @Produce      json
// @Param        groupID path int true "Group Chat ID"
// @Param        name formData string false "Group Name"
// @Param        description formData string false "Group Description"
// @Param        avatar formData file false "Group Avatar Image"
// @Success      200  {object}  helper.ResponseSuccess{data=model.ChatListResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{groupID} [put]
func (c *GroupChatController) UpdateGroupChat(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	groupID, err := strconv.Atoi(chi.URLParam(r, "groupID"))
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Failed to parse form data"))
		return
	}

	var req model.UpdateGroupChatRequest

	if _, ok := r.MultipartForm.Value["name"]; ok {
		name := r.FormValue("name")
		req.Name = &name
	}
	if _, ok := r.MultipartForm.Value["description"]; ok {
		desc := r.FormValue("description")
		req.Description = &desc
	}

	_, header, err := r.FormFile("avatar")
	if err == nil {
		req.Avatar = header
	} else if err != http.ErrMissingFile {
		helper.WriteError(w, helper.NewBadRequestError("Failed to process avatar file"))
		return
	}

	resp, err := c.groupChatService.UpdateGroupChat(r.Context(), userContext.ID, groupID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}

// SearchGroupMembers godoc
// @Summary      Search Group Members
// @Description  Search for members in a group chat by username or full name.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        groupID path int true "Group Chat ID"
// @Param        query query string false "Search query"
// @Param        cursor query string false "Pagination cursor"
// @Param        limit query int false "Number of items per page (default 20, max 50)"
// @Success      200  {object}  helper.ResponseWithPagination{data=[]model.GroupMemberDTO}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{groupID}/members [get]
func (c *GroupChatController) SearchGroupMembers(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	groupID, err := strconv.Atoi(chi.URLParam(r, "groupID"))
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	query := r.URL.Query().Get("query")
	cursor := r.URL.Query().Get("cursor")
	limitStr := r.URL.Query().Get("limit")

	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	req := model.SearchGroupMembersRequest{
		GroupID: groupID,
		Query:   query,
		Cursor:  cursor,
		Limit:   limit,
	}

	members, nextCursor, hasNext, err := c.groupChatService.SearchGroupMembers(r.Context(), userContext.ID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccessWithPagination(w, members, nextCursor, hasNext)
}

// AddMember godoc
// @Summary      Add Member to Group
// @Description  Add new members to a group chat. Only owners or admins can perform this action.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        groupID path int true "Group Chat ID"
// @Param        request body model.AddGroupMemberRequest true "Add Member Request"
// @Success      200  {object}  helper.ResponseSuccess
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      409  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{groupID}/members [post]
func (c *GroupChatController) AddMember(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	groupID, err := strconv.Atoi(chi.URLParam(r, "groupID"))
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	var req model.AddGroupMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	err = c.groupChatService.AddMember(r.Context(), userContext.ID, groupID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, nil)
}

// LeaveGroup godoc
// @Summary      Leave Group
// @Description  Leave a group chat. Owner cannot leave without transferring ownership first.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        groupID path int true "Group Chat ID"
// @Success      200  {object}  helper.ResponseSuccess
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{groupID}/leave [post]
func (c *GroupChatController) LeaveGroup(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	groupID, err := strconv.Atoi(chi.URLParam(r, "groupID"))
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	err = c.groupChatService.LeaveGroup(r.Context(), userContext.ID, groupID)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, nil)
}

// UpdateMemberRole godoc
// @Summary      Update Member Role
// @Description  Promote or demote a group member. Only owner can perform this action.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        groupID path int true "Group Chat ID"
// @Param        userID path int true "Target User ID"
// @Param        request body model.UpdateGroupMemberRoleRequest true "Update Role Request"
// @Success      200  {object}  helper.ResponseSuccess
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{groupID}/members/{userID}/role [put]
func (c *GroupChatController) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	groupID, err := strconv.Atoi(chi.URLParam(r, "groupID"))
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	targetUserID, err := strconv.Atoi(chi.URLParam(r, "userID"))
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	var req model.UpdateGroupMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	err = c.groupChatService.UpdateMemberRole(r.Context(), userContext.ID, groupID, targetUserID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, nil)
}

// TransferOwnership godoc
// @Summary      Transfer Ownership
// @Description  Transfer group ownership to another member. Only owner can perform this action.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        groupID path int true "Group Chat ID"
// @Param        request body model.TransferGroupOwnershipRequest true "Transfer Ownership Request"
// @Success      200  {object}  helper.ResponseSuccess
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{groupID}/transfer [post]
func (c *GroupChatController) TransferOwnership(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	groupID, err := strconv.Atoi(chi.URLParam(r, "groupID"))
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	var req model.TransferGroupOwnershipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	err = c.groupChatService.TransferOwnership(r.Context(), userContext.ID, groupID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, nil)
}
