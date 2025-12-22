package bootstrap

import (
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/controller"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

type Route struct {
	cfg            *config.AppConfig
	chi            *chi.Mux
	authController *controller.AuthController
}

func NewRoute(cfg *config.AppConfig, chi *chi.Mux, authController *controller.AuthController) *Route {
	return &Route{
		cfg:            cfg,
		chi:            chi,
		authController: authController,
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

	route.chi.Route("/api", func(r chi.Router) {
		r.Post("/auth/google", route.authController.GoogleExchange)
	})
}
