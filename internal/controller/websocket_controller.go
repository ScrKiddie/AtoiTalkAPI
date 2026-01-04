package controller

import (
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/middleware"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/websocket"
	"log/slog"
	"net/http"

	ws "github.com/gorilla/websocket"
)

type WebSocketController struct {
	hub *websocket.Hub
}

func NewWebSocketController(hub *websocket.Hub) *WebSocketController {
	return &WebSocketController{
		hub: hub,
	}
}

var upgrader = ws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ServeWS godoc
// @Summary      WebSocket Connection
// @Description  Upgrade HTTP connection to WebSocket. Requires Bearer token in Authorization header or 'token' query param.
// @Tags         websocket
// @Success      101  {string}  string  "Switching Protocols"
// @Failure      401  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /ws [get]
func (c *WebSocketController) ServeWS(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("Failed to upgrade websocket", "error", err)
		return
	}

	client := &websocket.Client{
		Hub:    c.hub,
		Conn:   conn,
		Send:   make(chan []byte, 256),
		UserID: userContext.ID,
	}

	client.Hub.Register <- client

	go client.WritePump()
	go client.ReadPump()
}
