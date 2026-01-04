package middleware

import (
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/service"
	"context"
	"net/http"
	"strings"
)

type contextKey string

const UserContextKey contextKey = "userContext"

type AuthMiddleware struct {
	authService *service.AuthService
}

func NewAuthMiddleware(authService *service.AuthService) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
	}
}

func (m *AuthMiddleware) VerifyToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			helper.WriteError(w, helper.NewUnauthorizedError(""))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			helper.WriteError(w, helper.NewUnauthorizedError(""))
			return
		}

		tokenString := parts[1]

		userContext, err := m.authService.VerifyUser(r.Context(), tokenString)
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, userContext)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *AuthMiddleware) VerifyWSToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString := r.URL.Query().Get("token")
		if tokenString == "" {
			helper.WriteError(w, helper.NewUnauthorizedError(""))
			return
		}

		userContext, err := m.authService.VerifyUser(r.Context(), tokenString)
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, userContext)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
