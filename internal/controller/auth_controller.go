package controller

import (
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/service"
	"encoding/json"
	"log/slog"
	"net/http"
)

type AuthController struct {
	authService *service.AuthService
}

func NewAuthController(authService *service.AuthService) *AuthController {
	return &AuthController{
		authService: authService,
	}
}

// GoogleExchange godoc
// @Summary      Google Exchange
// @Description  Exchange Google ID Token for App Token and User Info
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request body model.GoogleLoginRequest true "Google Login Request"
// @Success      200  {object}  helper.ResponseSuccess{data=model.AuthResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Router       /api/auth/google [post]
func (c *AuthController) GoogleExchange(w http.ResponseWriter, r *http.Request) {
	var req model.GoogleLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Invalid request body", "error", err)
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	resp, err := c.authService.GoogleExchange(r.Context(), req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}

// Register godoc
// @Summary      Register User
// @Description  Register a new user with email, password, and OTP verification.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request body model.RegisterUserRequest true "Register User Request"
// @Success      200  {object}  helper.ResponseSuccess{data=model.AuthResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      409  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Router       /api/auth/register [post]
func (c *AuthController) Register(w http.ResponseWriter, r *http.Request) {
	var req model.RegisterUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Invalid request body", "error", err)
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	resp, err := c.authService.Register(r.Context(), req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, resp)
}
