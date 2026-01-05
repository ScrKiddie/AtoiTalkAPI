package bootstrap

import (
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/controller"
	"AtoiTalkAPI/internal/middleware"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

type Route struct {
	cfg               *config.AppConfig
	chi               *chi.Mux
	authController    *controller.AuthController
	otpController     *controller.OTPController
	userController    *controller.UserController
	accountController *controller.AccountController
	chatController    *controller.ChatController
	messageController *controller.MessageController
	mediaController   *controller.MediaController
	wsController      *controller.WebSocketController
	authMiddleware    *middleware.AuthMiddleware
}

func NewRoute(cfg *config.AppConfig, chi *chi.Mux, authController *controller.AuthController, otpController *controller.OTPController, userController *controller.UserController, accountController *controller.AccountController, chatController *controller.ChatController, messageController *controller.MessageController, mediaController *controller.MediaController, wsController *controller.WebSocketController, authMiddleware *middleware.AuthMiddleware) *Route {
	return &Route{
		cfg:               cfg,
		chi:               chi,
		authController:    authController,
		otpController:     otpController,
		userController:    userController,
		accountController: accountController,
		chatController:    chatController,
		messageController: messageController,
		mediaController:   mediaController,
		wsController:      wsController,
		authMiddleware:    authMiddleware,
	}
}

func (route *Route) Register() {
	route.chi.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Welcome to AtoiTalkAPI"))
	})

	if route.cfg.StorageMode == "local" {
		serveStatic := func(pathFromConfig string) {
			if pathFromConfig == "" {
				return
			}
			cleanInput := strings.TrimLeft(pathFromConfig, "/\\.")

			physicalPath := filepath.Join(".", cleanInput)

			urlPath := filepath.ToSlash(physicalPath)

			routePattern := fmt.Sprintf("/%s/*", urlPath)
			prefix := fmt.Sprintf("/%s", urlPath)

			route.chi.Handle(routePattern, http.StripPrefix(prefix, http.FileServer(http.Dir(physicalPath))))
		}

		serveStatic(route.cfg.StorageAttachment)
		serveStatic(route.cfg.StorageProfile)
	}

	route.chi.With(route.authMiddleware.VerifyWSToken).Get("/ws", route.wsController.ServeWS)

	route.chi.Route("/api", func(r chi.Router) {

		r.Use(middleware.MaxBodySize(100 * 1024))

		r.Post("/auth/login", route.authController.Login)
		r.Post("/auth/google", route.authController.GoogleExchange)
		r.Post("/auth/register", route.authController.Register)
		r.Post("/auth/reset-password", route.authController.ResetPassword)
		r.Post("/otp/send", route.otpController.SendOTP)

		r.Group(func(r chi.Router) {
			r.Use(route.authMiddleware.VerifyToken)
			r.Get("/user/current", route.userController.GetCurrentUser)
			r.Get("/users/blocked", route.userController.GetBlockedUsers)
			r.Get("/users/{id}", route.userController.GetUserProfile)
			r.Get("/users", route.userController.SearchUsers)
			r.Post("/users/{id}/block", route.userController.BlockUser)
			r.Post("/users/{id}/unblock", route.userController.UnblockUser)

			r.With(middleware.MaxBodySize(3*1024*1024)).Put("/user/profile", route.userController.UpdateProfile)

			r.Put("/account/password", route.accountController.ChangePassword)
			r.Put("/account/email", route.accountController.ChangeEmail)

			r.Get("/chats", route.chatController.GetChats)
			r.Get("/chats/{id}", route.chatController.GetChat)
			r.Post("/chats/private", route.chatController.CreatePrivateChat)
			r.Post("/chats/{id}/read", route.chatController.MarkAsRead)
			r.Post("/chats/{id}/hide", route.chatController.HideChat)
			r.Get("/chats/{chatID}/messages", route.messageController.GetMessages)

			r.Post("/messages", route.messageController.SendMessage)
			r.Put("/messages/{messageID}", route.messageController.EditMessage)
			r.Delete("/messages/{messageID}", route.messageController.DeleteMessage)

			r.With(middleware.MaxBodySize(3*1024*1024)).Post("/media/upload", route.mediaController.UploadMedia)
		})
	})
}
