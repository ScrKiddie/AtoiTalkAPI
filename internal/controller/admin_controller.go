package controller

import (
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/middleware"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/service"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AdminController struct {
	adminService *service.AdminService
}

func NewAdminController(adminService *service.AdminService) *AdminController {
	return &AdminController{
		adminService: adminService,
	}
}

// BanUser godoc
// @Summary      Ban User
// @Description  Ban a user permanently or temporarily. Requires Admin privileges.
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        request body model.BanUserRequest true "Ban Request"
// @Success      200  {object}  helper.ResponseSuccess
// @Failure      400  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/admin/users/ban [post]
func (c *AdminController) BanUser(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	var req model.BanUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	if err := c.adminService.BanUser(r.Context(), userContext.ID, req); err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, nil)
}

// UnbanUser godoc
// @Summary      Unban User
// @Description  Lift a ban from a user. Requires Admin privileges.
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        userID path string true "User ID (UUID)"
// @Success      200  {object}  helper.ResponseSuccess
// @Failure      400  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/admin/users/{userID}/unban [post]
func (c *AdminController) UnbanUser(w http.ResponseWriter, r *http.Request) {
	userContext, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	userIDStr := chi.URLParam(r, "userID")
	targetUserID, err := uuid.Parse(userIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid User ID"))
		return
	}

	if err := c.adminService.UnbanUser(r.Context(), userContext.ID, targetUserID); err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, nil)
}

// GetReports godoc
// @Summary      Get Reports
// @Description  Get a list of reports. Requires Admin privileges.
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        status query string false "Filter by status (pending, reviewed, resolved, rejected)"
// @Param        limit query int false "Limit (default 20)"
// @Param        cursor query string false "Cursor for pagination"
// @Success      200  {object}  helper.ResponseWithPagination{data=[]model.ReportListResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/admin/reports [get]
func (c *AdminController) GetReports(w http.ResponseWriter, r *http.Request) {
	_, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	status := r.URL.Query().Get("status")
	cursor := r.URL.Query().Get("cursor")
	limitStr := r.URL.Query().Get("limit")

	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	req := model.GetReportsRequest{
		Status: status,
		Cursor: cursor,
		Limit:  limit,
	}

	reports, nextCursor, hasNext, err := c.adminService.GetReports(r.Context(), req)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccessWithPagination(w, reports, nextCursor, hasNext)
}

// GetReportDetail godoc
// @Summary      Get Report Detail
// @Description  Get detailed information about a report including evidence snapshot. Requires Admin privileges.
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        reportID path string true "Report ID (UUID)"
// @Success      200  {object}  helper.ResponseSuccess{data=model.ReportDetailResponse}
// @Failure      400  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/admin/reports/{reportID} [get]
func (c *AdminController) GetReportDetail(w http.ResponseWriter, r *http.Request) {
	_, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	reportIDStr := chi.URLParam(r, "reportID")
	reportID, err := uuid.Parse(reportIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid Report ID"))
		return
	}

	report, err := c.adminService.GetReportDetail(r.Context(), reportID)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, report)
}

// ResolveReport godoc
// @Summary      Resolve Report
// @Description  Update the status of a report (e.g., to resolved or rejected). Requires Admin privileges.
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        reportID path string true "Report ID (UUID)"
// @Param        request body model.ResolveReportRequest true "Resolve Request"
// @Success      200  {object}  helper.ResponseSuccess
// @Failure      400  {object}  helper.ResponseError
// @Failure      403  {object}  helper.ResponseError
// @Failure      404  {object}  helper.ResponseError
// @Failure      500  {object}  helper.ResponseError
// @Security     BearerAuth
// @Router       /api/admin/reports/{reportID}/resolve [put]
func (c *AdminController) ResolveReport(w http.ResponseWriter, r *http.Request) {
	_, ok := r.Context().Value(middleware.UserContextKey).(*model.UserDTO)
	if !ok {
		helper.WriteError(w, helper.NewUnauthorizedError(""))
		return
	}

	reportIDStr := chi.URLParam(r, "reportID")
	reportID, err := uuid.Parse(reportIDStr)
	if err != nil {
		helper.WriteError(w, helper.NewBadRequestError("Invalid Report ID"))
		return
	}

	var req model.ResolveReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		helper.WriteError(w, helper.NewBadRequestError(""))
		return
	}

	if err := c.adminService.ResolveReport(r.Context(), reportID, req); err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteSuccess(w, nil)
}
