package controller

import (
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/middleware"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/service"
	"encoding/json"
	"log/slog"
	"net/http"
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
		helper.WriteError(w, helper.NewBadRequestError("Invalid request body"))
		return
	}

	resp, err := c.messageService.SendMessage(r.Context(), userContext.ID, req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}
