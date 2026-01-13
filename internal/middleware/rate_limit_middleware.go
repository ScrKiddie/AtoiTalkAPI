package middleware

import (
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/repository"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strings"
	"time"
)

type RateLimitMiddleware struct {
	repo *repository.RateLimitRepository
}

func NewRateLimitMiddleware(repo *repository.RateLimitRepository) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		repo: repo,
	}
}

func (m *RateLimitMiddleware) Limit(keyName string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var identifier string
			var keyPrefix string

			userContext, ok := r.Context().Value(UserContextKey).(*model.UserDTO)
			if ok && userContext != nil {
				identifier = userContext.ID.String()
				keyPrefix = "ratelimit:user"
			} else {

				identifier = getIP(r)
				keyPrefix = "ratelimit:ip"
			}

			key := fmt.Sprintf("%s:%s:%s", keyPrefix, keyName, identifier)

			allowed, ttl, err := m.repo.Allow(r.Context(), key, limit, window)
			if err != nil {

				slog.Error("Rate limit check failed", "error", err)
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", int(ttl.Seconds())))

			if !allowed {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("Retry-After", fmt.Sprintf("%d", int(math.Ceil(ttl.Seconds()))))

				helper.WriteError(w, helper.NewTooManyRequestsError("Rate limit exceeded. Please try again later."))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func getIP(r *http.Request) string {

	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {

		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}

	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
