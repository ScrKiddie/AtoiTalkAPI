package controller

import (
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/middleware"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/service"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type MessageController struct {
	messageService *service.MessageService
}

func NewMessageController(messageService *service.MessageService) *MessageController {
	return &MessageController{
		messageService: messageService,
	}
}

// SendMessage godoc
// @Summary      Send Message
// @Description  Send a message to a chat (private or group). Supports text and attachments (via IDs).
// @Tags         message
// @Accept       json
// @Produce      json
// @Param        request body model.SendMessageRequest true "Send Message Request"
// @Success      200  {object}  helper.ResponseSuccess{data=model.MessageResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/messages [post]
func (c *MessageController) SendMessage(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	var req model.SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Invalid request body", "error", err)
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	resp, err := c.messageService.SendMessage(r.Context(), userContext.ID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}

// EditMessage godoc
// @Summary      Edit Message
// @Description  Edit a message's content and/or attachments.
// @Tags         message
// @Accept       json
// @Produce      json
// @Param        messageID path string true "Message ID (UUID)"
// @Param        request body model.EditMessageRequest true "Edit Message Request"
// @Success      200  {object}  helper.ResponseSuccess{data=model.MessageResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/messages/{messageID} [put]
func (c *MessageController) EditMessage(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	messageIDStr := chi.URLParam(r, "messageID")
	messageID, err := uuid.Parse(messageIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid Message ID"))
		return
	}

	var req model.EditMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Invalid request body", "error", err)
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	resp, err := c.messageService.EditMessage(r.Context(), userContext.ID, messageID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}

// GetMessages godoc
// @Summary      Get Messages
// @Description  Get a paginated list of messages from a chat.
// @Tags         message
// @Accept       json
// @Produce      json
// @Param        chatID path string true "Chat ID (UUID)"
// @Param        cursor query string false "Pagination cursor (Base64 encoded message ID)"
// @Param        limit query int false "Number of messages to fetch (default 20, max 50)"
// @Success      200  {object}  helper.ResponseWithPagination{data=[]model.MessageResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/chats/{chatID}/messages [get]
func (c *MessageController) GetMessages(w http.ResponseWriter, r *http.Request) {
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

	cursor := r.URL.Query().Get("cursor")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	req := model.GetMessagesRequest{
		ChatID: chatID,
		Cursor: cursor,
		Limit:  limit,
	}

	messages, nextCursor, hasNext, err := c.messageService.GetMessages(r.Context(), userContext.ID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccessWithPagination(w, messages, nextCursor, hasNext)
}

// DeleteMessage godoc
// @Summary      Delete Message
// @Description  Soft delete a message. Only the sender can delete their own message.
// @Tags         message
// @Accept       json
// @Produce      json
// @Param        messageID path string true "Message ID (UUID)"
// @Success      200  {object}  helper.ResponseSuccess
// @Failure      400  {object}  helper.ResponseError
// @Failure      401  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/messages/{messageID} [delete]
func (c *MessageController) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	messageIDStr := chi.URLParam(r, "messageID")
	messageID, err := uuid.Parse(messageIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid Message ID"))
		return
	}

	err = c.messageService.DeleteMessage(r.Context(), userContext.ID, messageID)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, nil)
}
