package bootstrap

import (
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/controller"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

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

	if cfg.StorageMode == "local" {
		serveStatic := func(pathFromConfig string) {
			if pathFromConfig == "" {
				return
			}
			cleanInput := strings.TrimLeft(pathFromConfig, "/\\.")

			physicalPath := filepath.Join(".", cleanInput)

			urlPath := filepath.ToSlash(physicalPath)

			routePattern := fmt.Sprintf("/%s/*", urlPath)
			prefix := fmt.Sprintf("/%s", urlPath)

			r.Handle(routePattern, http.StripPrefix(prefix, http.FileServer(http.Dir(physicalPath))))
		}

		serveStatic(cfg.StorageAttachment)
		serveStatic(cfg.StorageProfile)
	}

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/google", authController.GoogleExchange)
	})

	return r
}
