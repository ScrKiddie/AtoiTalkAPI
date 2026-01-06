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

type ChatController struct {
	chatService *service.ChatService
}

func NewChatController(chatService *service.ChatService) *ChatController {
	return &ChatController{
		chatService: chatService,
	}
}

// CreatePrivateChat godoc
// @Summary      Create Private Chat
// @Description  Create a new private chat with another user. If it already exists, returns the existing chat.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        request body model.CreatePrivateChatRequest true "Create Private Chat Request"
// @Success      200  {object}  helper.ResponseSuccess{data=model.ChatResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/private [post]
func (c *ChatController) CreatePrivateChat(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	var req model.CreatePrivateChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	resp, err := c.chatService.CreatePrivateChat(r.Context(), userContext.ID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
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
func (c *ChatController) CreateGroupChat(w http.ResponseWriter, r *http.Request) {
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

	resp, err := c.chatService.CreateGroupChat(r.Context(), userContext.ID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}

// GetChats godoc
// @Summary      Get Chat List
// @Description  Get a paginated list of user's chats, sorted by last message time. Can be searched.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        query query string false "Search query for chat name"
// @Param        cursor query string false "Pagination cursor"
// @Param        limit query int false "Number of items per page (default 20, max 50)"
// @Success      200  {object}  helper.ResponseWithPagination{data=[]model.ChatListResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats [get]
func (c *ChatController) GetChats(w http.ResponseWriter, r *http.Request) {
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

	req := model.GetChatsRequest{
		Query:  query,
		Cursor: cursor,
		Limit:  limit,
	}

	chats, nextCursor, hasNext, err := c.chatService.GetChats(r.Context(), userContext.ID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccessWithPagination(w, chats, nextCursor, hasNext)
}

// GetChat godoc
// @Summary      Get Chat by ID
// @Description  Get detailed information about a single chat.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        id path int true "Chat ID"
// @Success      200  {object}  helper.ResponseSuccess{data=model.ChatListResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/{id} [get]
func (c *ChatController) GetChat(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	chatIDStr := chi.URLParam(r, "id")
	chatID, err := strconv.Atoi(chatIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	chat, err := c.chatService.GetChatByID(r.Context(), userContext.ID, chatID)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, chat)
}

// MarkAsRead godoc
// @Summary      Mark Chat as Read
// @Description  Mark all messages in a chat as read for the current user.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        id path int true "Chat ID"
// @Success      200  {object}  helper.ResponseSuccess
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/{id}/read [post]
func (c *ChatController) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	chatIDStr := chi.URLParam(r, "id")
	chatID, err := strconv.Atoi(chatIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	err = c.chatService.MarkAsRead(r.Context(), userContext.ID, chatID)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, nil)
}

// HideChat godoc
// @Summary      Hide Chat
// @Description  Hide a private chat from the chat list. It will reappear if a new message is sent or received.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        id path int true "Chat ID"
// @Success      200  {object}  helper.ResponseSuccess
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/{id}/hide [post]
func (c *ChatController) HideChat(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	chatIDStr := chi.URLParam(r, "id")
	chatID, err := strconv.Atoi(chatIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	err = c.chatService.HideChat(r.Context(), userContext.ID, chatID)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, nil)
}
