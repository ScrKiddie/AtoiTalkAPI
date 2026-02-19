package controller

import (
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/middleware"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/websocket"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	ws "github.com/gorilla/websocket"
)

type WebSocketController struct {
	hub            *websocket.Hub
	allowAllOrigin bool
	allowedOrigins map[string]struct{}
}

func NewWebSocketController(hub *websocket.Hub, cfg *config.AppConfig) *WebSocketController {
	allowAllOrigin := false
	allowedOrigins := make(map[string]struct{})

	for _, rawOrigin := range cfg.AppCorsAllowedOrigins {
		trimmed := strings.TrimSpace(rawOrigin)
		if trimmed == "" {
			continue
		}

		if trimmed == "*" {
			allowAllOrigin = true
			continue
		}

		normalized := normalizeOrigin(trimmed)
		if normalized != "" {
			allowedOrigins[normalized] = struct{}{}
		}
	}

	return &WebSocketController{
		hub:            hub,
		allowAllOrigin: allowAllOrigin,
		allowedOrigins: allowedOrigins,
	}
}

func normalizeOrigin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}

	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

func (c *WebSocketController) checkOrigin(r *http.Request) bool {
	if c.allowAllOrigin {
		return true
	}

	origin := r.Header.Get("Origin")
	if strings.TrimSpace(origin) == "" {
		return true
	}

	normalized := normalizeOrigin(origin)
	if normalized == "" {
		return false
	}

	_, ok := c.allowedOrigins[normalized]
	return ok
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

	upgrader := ws.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     c.checkOrigin,
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
