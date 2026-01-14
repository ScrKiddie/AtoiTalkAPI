package bootstrap

import (
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/controller"
	"AtoiTalkAPI/internal/middleware"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type Route struct {
	cfg                   *config.AppConfig
	chi                   *chi.Mux
	authController        *controller.AuthController
	otpController         *controller.OTPController
	userController        *controller.UserController
	accountController     *controller.AccountController
	chatController        *controller.ChatController
	privateChatController *controller.PrivateChatController
	groupChatController   *controller.GroupChatController
	messageController     *controller.MessageController
	mediaController       *controller.MediaController
	wsController          *controller.WebSocketController
	reportController      *controller.ReportController
	adminController       *controller.AdminController
	authMiddleware        *middleware.AuthMiddleware
	rateLimitMiddleware   *middleware.RateLimitMiddleware
}

func NewRoute(cfg *config.AppConfig, chi *chi.Mux, authController *controller.AuthController, otpController *controller.OTPController, userController *controller.UserController, accountController *controller.AccountController, chatController *controller.ChatController, privateChatController *controller.PrivateChatController, groupChatController *controller.GroupChatController, messageController *controller.MessageController, mediaController *controller.MediaController, wsController *controller.WebSocketController, reportController *controller.ReportController, adminController *controller.AdminController, authMiddleware *middleware.AuthMiddleware, rateLimitMiddleware *middleware.RateLimitMiddleware) *Route {
	return &Route{
		cfg:                   cfg,
		chi:                   chi,
		authController:        authController,
		otpController:         otpController,
		userController:        userController,
		accountController:     accountController,
		chatController:        chatController,
		privateChatController: privateChatController,
		groupChatController:   groupChatController,
		messageController:     messageController,
		mediaController:       mediaController,
		wsController:          wsController,
		reportController:      reportController,
		adminController:       adminController,
		authMiddleware:        authMiddleware,
		rateLimitMiddleware:   rateLimitMiddleware,
	}
}

func (route *Route) Register() {
	route.chi.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Welcome to AtoiTalkAPI"))
	})

	route.chi.With(route.authMiddleware.VerifyWSToken).Get("/ws", route.wsController.ServeWS)

	route.chi.Route("/api", func(r chi.Router) {

		r.Group(func(r chi.Router) {
			r.Use(middleware.MaxBodySize(100 * 1024))
			r.Use(route.rateLimitMiddleware.Limit("auth_public", 10, time.Minute))

			r.Post("/auth/login", route.authController.Login)
			r.Post("/auth/google", route.authController.GoogleExchange)
			r.Post("/auth/register", route.authController.Register)
			r.Post("/auth/reset-password", route.authController.ResetPassword)
			r.Post("/otp/send", route.otpController.SendOTP)

			r.Get("/chats/group/invite/{inviteCode}", route.groupChatController.GetGroupByInviteCode)
		})

		r.Group(func(r chi.Router) {
			r.Use(route.authMiddleware.VerifyToken)

			r.Group(func(r chi.Router) {
				r.Use(middleware.MaxBodySize(100 * 1024))
				r.Use(route.rateLimitMiddleware.Limit("auth_logout", 20, time.Minute))
				r.Post("/auth/logout", route.authController.Logout)
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.MaxBodySize(20 * 1024 * 1024))
				r.Use(route.rateLimitMiddleware.Limit("media_upload", 20, time.Minute))
				r.Post("/media/upload", route.mediaController.UploadMedia)
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.MaxBodySize(5 * 1024 * 1024))
				r.Use(route.rateLimitMiddleware.Limit("user_management", 30, time.Minute))

				r.Put("/user/profile", route.userController.UpdateProfile)
				r.Post("/chats/group", route.groupChatController.CreateGroupChat)
				r.Put("/chats/group/{groupID}", route.groupChatController.UpdateGroupChat)
			})

			r.Group(func(r chi.Router) {
				r.Use(route.authMiddleware.AdminOnly)
				r.Use(route.rateLimitMiddleware.Limit("admin_action", 1000, time.Minute))

				r.Post("/admin/users/ban", route.adminController.BanUser)
				r.Post("/admin/users/{userID}/unban", route.adminController.UnbanUser)
				r.Get("/admin/reports", route.adminController.GetReports)
				r.Get("/admin/reports/{reportID}", route.adminController.GetReportDetail)
				r.Put("/admin/reports/{reportID}/resolve", route.adminController.ResolveReport)
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.MaxBodySize(100 * 1024))
				r.Use(route.rateLimitMiddleware.Limit("general_read", 200, time.Minute))

				r.Get("/user/current", route.userController.GetCurrentUser)
				r.Get("/users/blocked", route.userController.GetBlockedUsers)
				r.Get("/users/{id}", route.userController.GetUserProfile)
				r.Get("/users", route.userController.SearchUsers)
				r.Post("/users/{id}/block", route.userController.BlockUser)
				r.Post("/users/{id}/unblock", route.userController.UnblockUser)

				r.Put("/account/password", route.accountController.ChangePassword)
				r.Put("/account/email", route.accountController.ChangeEmail)
				r.Delete("/account", route.accountController.DeleteAccount)

				r.Get("/chats", route.chatController.GetChats)
				r.Get("/chats/{id}", route.chatController.GetChat)
				r.Post("/chats/{id}/read", route.chatController.MarkAsRead)
				r.Post("/chats/{id}/hide", route.chatController.HideChat)
				r.Get("/chats/{chatID}/messages", route.messageController.GetMessages)

				r.Post("/chats/private", route.privateChatController.CreatePrivateChat)

				r.Get("/chats/group/public", route.groupChatController.SearchPublicGroups)
				r.Get("/chats/group/{groupID}/members", route.groupChatController.SearchGroupMembers)
				r.Post("/chats/group/{groupID}/members", route.groupChatController.AddMember)
				r.Post("/chats/group/{groupID}/leave", route.groupChatController.LeaveGroup)
				r.Post("/chats/group/{groupID}/join", route.groupChatController.JoinPublicGroup)
				r.Post("/chats/group/join/invite", route.groupChatController.JoinGroupByInvite)
				r.Get("/chats/group/{groupID}/invite", route.groupChatController.GetInviteCode)
				r.Put("/chats/group/{groupID}/invite", route.groupChatController.ResetInviteCode)
				r.Post("/chats/group/{groupID}/members/{userID}/kick", route.groupChatController.KickMember)
				r.Put("/chats/group/{groupID}/members/{userID}/role", route.groupChatController.UpdateMemberRole)
				r.Post("/chats/group/{groupID}/transfer", route.groupChatController.TransferOwnership)
				r.Delete("/chats/group/{groupID}", route.groupChatController.DeleteGroup)

				r.Post("/messages", route.messageController.SendMessage)
				r.Put("/messages/{messageID}", route.messageController.EditMessage)
				r.Delete("/messages/{messageID}", route.messageController.DeleteMessage)

				r.Post("/reports", route.reportController.CreateReport)

				r.Get("/media/{mediaID}/url", route.mediaController.GetMediaURL)
			})
		})
	})
}
