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
	"github.com/google/uuid"
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
// @Param        member_ids formData string true "JSON Array of Member IDs (UUIDs)"
// @Param        avatar formData file false "Group Avatar Image"
// @Param        is_public formData boolean false "Is Public Group"
// @Success      200  {object}  helper.ResponseSuccess{data=model.ChatListResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      429  {object}  helper.ResponseError
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
	isPublicStr := r.FormValue("is_public")

	var memberIDs []uuid.UUID
	if memberIDsStr != "" {
		var idStrings []string
		if err := json.Unmarshal([]byte(memberIDsStr), &idStrings); err == nil {
			for _, s := range idStrings {
				if id, err := uuid.Parse(s); err == nil {
					memberIDs = append(memberIDs, id)
				}
			}
		} else {
			parts := strings.Split(memberIDsStr, ",")
			for _, p := range parts {
				id, err := uuid.Parse(strings.TrimSpace(p))
				if err == nil {
					memberIDs = append(memberIDs, id)
				}
			}
		}
	}

	isPublic := false
	if isPublicStr != "" {
		isPublic, _ = strconv.ParseBool(isPublicStr)
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
		IsPublic:    isPublic,
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
// @Description  Update group name, description, avatar, or visibility. Only owners or admins can perform this action.
// @Tags         chat
// @Accept       multipart/form-data
// @Produce      json
// @Param        chatID path string true "Group Chat ID (UUID)"
// @Param        name formData string false "Group Name"
// @Param        description formData string false "Group Description"
// @Param        avatar formData file false "Group Avatar Image"
// @Param        delete_avatar formData boolean false "Delete Avatar (Set to true to delete current avatar)"
// @Param        is_public formData boolean false "Is Public Group"
// @Success      200  {object}  helper.ResponseSuccess{data=model.ChatListResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      429  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{chatID} [put]
func (c *GroupChatController) UpdateGroupChat(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	chatIDStr := chi.URLParam(r, "chatID")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid Chat ID"))
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
	if _, ok := r.MultipartForm.Value["is_public"]; ok {
		isPublic, _ := strconv.ParseBool(r.FormValue("is_public"))
		req.IsPublic = &isPublic
	}
	if _, ok := r.MultipartForm.Value["delete_avatar"]; ok {
		deleteAvatar, _ := strconv.ParseBool(r.FormValue("delete_avatar"))
		req.DeleteAvatar = deleteAvatar
	}

	_, header, err := r.FormFile("avatar")
	if err == nil {
		req.Avatar = header
	} else if err != http.ErrMissingFile {
		helper.WriteError(w, helper.NewBadRequestError("Failed to process avatar file"))
		return
	}

	resp, err := c.groupChatService.UpdateGroupChat(r.Context(), userContext.ID, chatID, req, false)
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
// @Param        chatID path string true "Group Chat ID (UUID)"
// @Param        query query string false "Search query"
// @Param        cursor query string false "Pagination cursor"
// @Param        limit query int false "Number of items per page (default 20, max 50)"
// @Success      200  {object}  helper.ResponseWithPagination{data=[]model.GroupMemberDTO}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      429  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{chatID}/members [get]
func (c *GroupChatController) SearchGroupMembers(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	chatIDStr := chi.URLParam(r, "chatID")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid Chat ID"))
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
		GroupID: chatID,
		Query:   query,
		Cursor:  cursor,
		Limit:   limit,
	}

	members, nextCursor, hasNext, err := c.groupChatService.SearchGroupMembers(r.Context(), userContext.ID, req, false)
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
// @Param        chatID path string true "Group Chat ID (UUID)"
// @Param        request body model.AddGroupMemberRequest true "Add Member Request"
// @Success      200  {object}  helper.ResponseSuccess{data=[]model.MessageResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      409  {object}  helper.ResponseError
// @Failure      429  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{chatID}/members [post]
func (c *GroupChatController) AddMember(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	chatIDStr := chi.URLParam(r, "chatID")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid Chat ID"))
		return
	}

	var req model.AddGroupMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	resp, err := c.groupChatService.AddMember(r.Context(), userContext.ID, chatID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}

// LeaveGroup godoc
// @Summary      Leave Group
// @Description  Leave a group chat. Owner cannot leave without transferring ownership first.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        chatID path string true "Group Chat ID (UUID)"
// @Success      200  {object}  helper.ResponseSuccess{data=model.MessageResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      429  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{chatID}/leave [post]
func (c *GroupChatController) LeaveGroup(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	chatIDStr := chi.URLParam(r, "chatID")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid Chat ID"))
		return
	}

	resp, err := c.groupChatService.LeaveGroup(r.Context(), userContext.ID, chatID)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}

// KickMember godoc
// @Summary      Kick Member from Group
// @Description  Kick a member from a group chat. Only owners or admins can perform this action.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        chatID path string true "Group Chat ID (UUID)"
// @Param        userID path string true "Target User ID (UUID)"
// @Success      200  {object}  helper.ResponseSuccess{data=model.MessageResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      429  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{chatID}/members/{userID}/kick [post]
func (c *GroupChatController) KickMember(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	chatIDStr := chi.URLParam(r, "chatID")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid Chat ID"))
		return
	}

	targetUserIDStr := chi.URLParam(r, "userID")
	targetUserID, err := uuid.Parse(targetUserIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid User ID"))
		return
	}

	resp, err := c.groupChatService.KickMember(r.Context(), userContext.ID, chatID, targetUserID)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}

// UpdateMemberRole godoc
// @Summary      Update Member Role
// @Description  Promote or demote a group member. Only owner can perform this action.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        chatID path string true "Group Chat ID (UUID)"
// @Param        userID path string true "Target User ID (UUID)"
// @Param        request body model.UpdateGroupMemberRoleRequest true "Update Role Request"
// @Success      200  {object}  helper.ResponseSuccess{data=model.MessageResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      429  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{chatID}/members/{userID}/role [put]
func (c *GroupChatController) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	chatIDStr := chi.URLParam(r, "chatID")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid Chat ID"))
		return
	}

	targetUserIDStr := chi.URLParam(r, "userID")
	targetUserID, err := uuid.Parse(targetUserIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid User ID"))
		return
	}

	var req model.UpdateGroupMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	resp, err := c.groupChatService.UpdateMemberRole(r.Context(), userContext.ID, chatID, targetUserID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}

// TransferOwnership godoc
// @Summary      Transfer Ownership
// @Description  Transfer group ownership to another member. Only owner can perform this action.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        chatID path string true "Group Chat ID (UUID)"
// @Param        request body model.TransferGroupOwnershipRequest true "Transfer Ownership Request"
// @Success      200  {object}  helper.ResponseSuccess{data=model.MessageResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      429  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{chatID}/transfer [post]
func (c *GroupChatController) TransferOwnership(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	chatIDStr := chi.URLParam(r, "chatID")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid Chat ID"))
		return
	}

	var req model.TransferGroupOwnershipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	resp, err := c.groupChatService.TransferOwnership(r.Context(), userContext.ID, chatID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}

// DeleteGroup godoc
// @Summary      Delete Group
// @Description  Soft delete a group chat. Only owner can perform this action.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        chatID path string true "Group Chat ID (UUID)"
// @Success      200  {object}  helper.ResponseSuccess
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      429  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{chatID} [delete]
func (c *GroupChatController) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	chatIDStr := chi.URLParam(r, "chatID")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid Chat ID"))
		return
	}

	err = c.groupChatService.DeleteGroup(r.Context(), userContext.ID, chatID, false)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, nil)
}

// SearchPublicGroups godoc
// @Summary      Search Public Groups
// @Description  Search for public groups by name or description.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        query query string false "Search query"
// @Param        cursor query string false "Pagination cursor"
// @Param        limit query int false "Number of items per page (default 20, max 50)"
// @Success      200  {object}  helper.ResponseWithPagination{data=[]model.PublicGroupDTO}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      429  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/public [get]
func (c *GroupChatController) SearchPublicGroups(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
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

	req := model.SearchPublicGroupsRequest{
		Query:  query,
		Cursor: cursor,
		Limit:  limit,
	}

	groups, nextCursor, hasNext, err := c.groupChatService.SearchPublicGroups(r.Context(), userContext.ID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccessWithPagination(w, groups, nextCursor, hasNext)
}

// JoinPublicGroup godoc
// @Summary      Join Public Group
// @Description  Join a public group chat.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        chatID path string true "Group Chat ID (UUID)"
// @Success      200  {object}  helper.ResponseSuccess{data=model.ChatListResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      409  {object}  helper.ResponseError
// @Failure      429  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{chatID}/join [post]
func (c *GroupChatController) JoinPublicGroup(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	chatIDStr := chi.URLParam(r, "chatID")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid Chat ID"))
		return
	}

	resp, err := c.groupChatService.JoinPublicGroup(r.Context(), userContext.ID, chatID)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}

// JoinGroupByInvite godoc
// @Summary      Join Group by Invite Code
// @Description  Join a private or public group using an invite code.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        request body model.JoinGroupByInviteRequest true "Join Request"
// @Success      200  {object}  helper.ResponseSuccess{data=model.ChatListResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      429  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/join/invite [post]
func (c *GroupChatController) JoinGroupByInvite(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	var req model.JoinGroupByInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	resp, err := c.groupChatService.JoinGroupByInvite(r.Context(), userContext.ID, req.InviteCode)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}

// GetGroupByInviteCode godoc
// @Summary      Get Group Preview by Invite Code
// @Description  Get basic group info using an invite code. Useful for previewing before joining.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        inviteCode path string true "Invite Code"
// @Success      200  {object}  helper.ResponseSuccess{data=model.GroupPreviewDTO}
// @Failure      400  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      429  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Router       /api/chats/group/invite/{inviteCode} [get]
func (c *GroupChatController) GetGroupByInviteCode(w http.ResponseWriter, r *http.Request) {
	inviteCode := chi.URLParam(r, "inviteCode")
	if inviteCode == "" {
		helper.WriteError(w, helper.NewBadRequestError("Invite code is required"))
		return
	}

	resp, err := c.groupChatService.GetGroupByInviteCode(r.Context(), inviteCode)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}

// ResetInviteCode godoc
// @Summary      Reset Group Invite Code
// @Description  Reset the invite code for a group. Only admins or owners can perform this action.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        chatID path string true "Group Chat ID (UUID)"
// @Success      200  {object}  helper.ResponseSuccess{data=model.GroupInviteResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      429  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/group/{chatID}/invite [put]
func (c *GroupChatController) ResetInviteCode(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	chatIDStr := chi.URLParam(r, "chatID")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid Chat ID"))
		return
	}

	resp, err := c.groupChatService.ResetInviteCode(r.Context(), userContext.ID, chatID)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}
