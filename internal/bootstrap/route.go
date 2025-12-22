package bootstrap

import (
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/controller"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func SetupRoutes(cfg *config.AppConfig, authController *controller.AuthController) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Welcome to AtoiTalkAPI"))
	})

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/google", authController.GoogleExchange)
	})

	return r
}
