package config

import (
	"AtoiTalkAPI/internal/helper"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	slogchi "github.com/samber/slog-chi"
)

func NewChi(cfg *AppConfig) *chi.Mux {
	r := chi.NewRouter()

	r.Use(slogchi.New(slog.Default()))
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.AppCorsAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		helper.WriteError(w, helper.NewNotFoundError(""))
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		helper.WriteError(w, helper.NewMethodNotAllowedError(""))
	})

	return r
}
