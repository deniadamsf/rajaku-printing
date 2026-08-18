// Handlers untuk /admin/staff, /admin/roles, /admin/permissions, /invites/*.
// Semua kecuali /invites/* butuh permission staff.manage atau role.manage.
package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/password"
	"github.com/rajaku-printing/backend/internal/auth/service"
	"github.com/rajaku-printing/backend/internal/httpx"
)

// AdminHandler — HTTP-facing bundle untuk staff + role + invite admin endpoints.
type AdminHandler struct {
	staff   *service.StaffAdminService
	roles   *service.RoleAdminService
	invites *service.InviteService
}

func NewAdminHandler(staff *service.StaffAdminService, roles *service.RoleAdminService, invites *service.InviteService) *AdminHandler {
	return &AdminHandler{staff: staff, roles: roles, invites: invites}
}

// ---- error mapping ----

func adminMapErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, authapi.ErrStaffNotFound), errors.Is(err, authapi.ErrRoleNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, authapi.ErrEmailAlreadyUsed), errors.Is(err, authapi.ErrPhoneAlreadyUsed),
		errors.Is(err, authapi.ErrRoleDuplicateName):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, authapi.ErrRoleSystemLocked):
		httpx.Error(c, http.StatusForbidden, httpx.CodeForbidden, err.Error())
	case errors.Is(err, authapi.ErrInvalidRoleAssignment),
		errors.Is(err, authapi.ErrInvalidPermissionSet):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	case errors.Is(err, authapi.ErrInviteInvalid), errors.Is(err, authapi.ErrInviteAlreadyUsed):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
	case errors.Is(err, authapi.ErrInviteExpired):
		httpx.Error(c, http.StatusGone, httpx.CodeBadRequest, err.Error())
	case errors.Is(err, password.ErrTooShort), errors.Is(err, password.ErrTooLong):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("admin handler: unmapped error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
	}
}

func callerID(c *gin.Context) (uuid.UUID, bool) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return uuid.Nil, false
	}
	return id.UserID, true
}

// ---- Staff endpoints ----

type createStaffBody struct {
	Name     string      `json:"name"`
	Email    string      `json:"email"`
	Phone    string      `json:"phone"`
	RoleIDs  []string    `json:"role_ids"`
}

// POST /admin/staff
func (h *AdminHandler) CreateStaff(c *gin.Context) {
	inviter, ok := callerID(c)
	if !ok {
		return
	}
	var body createStaffBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid json body")
		return
	}
	roleIDs, err := parseUUIDs(body.RoleIDs)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "role_ids: "+err.Error())
		return
	}
	res, err := h.staff.CreateStaff(c.Request.Context(), service.CreateStaffInput{
		Name: body.Name, Email: body.Email, Phone: body.Phone,
		RoleIDs: roleIDs, InviterID: inviter,
	})
	if err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.Created(c, res)
}

// GET /admin/staff?q=&active=&page=&page_size=
func (h *AdminHandler) ListStaff(c *gin.Context) {
	q := c.Query("q")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	var activePtr *bool
	if v := c.Query("active"); v != "" {
		b := v == "true" || v == "1"
		activePtr = &b
	}
	res, err := h.staff.ListStaff(c.Request.Context(), service.ListStaffInput{
		Q: q, IsActive: activePtr, Page: page, PageSize: pageSize,
	})
	if err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.OK(c, res)
}

// GET /admin/staff/:id
func (h *AdminHandler) GetStaff(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	u, err := h.staff.GetStaff(c.Request.Context(), id)
	if err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.OK(c, u)
}

type updateStaffBody struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// PATCH /admin/staff/:id
func (h *AdminHandler) UpdateStaff(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	var body updateStaffBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid json body")
		return
	}
	if err := h.staff.UpdateStaff(c.Request.Context(), id, service.UpdateStaffInput{
		Name: body.Name, Email: body.Email, Phone: body.Phone,
	}); err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// POST /admin/staff/:id/deactivate
func (h *AdminHandler) DeactivateStaff(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	if err := h.staff.DeactivateStaff(c.Request.Context(), id); err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// POST /admin/staff/:id/activate
func (h *AdminHandler) ActivateStaff(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	if err := h.staff.ReactivateStaff(c.Request.Context(), id); err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

type assignRolesBody struct {
	RoleIDs []string `json:"role_ids"`
}

// POST /admin/staff/:id/roles — replace all
func (h *AdminHandler) AssignRoles(c *gin.Context) {
	assignedBy, ok := callerID(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	var body assignRolesBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid json body")
		return
	}
	roleIDs, err := parseUUIDs(body.RoleIDs)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "role_ids: "+err.Error())
		return
	}
	if err := h.staff.AssignRoles(c.Request.Context(), id, roleIDs, assignedBy); err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// ---- Role endpoints ----

// GET /admin/roles
func (h *AdminHandler) ListRoles(c *gin.Context) {
	roles, err := h.roles.ListRoles(c.Request.Context())
	if err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": roles})
}

// GET /admin/roles/:id
func (h *AdminHandler) GetRole(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	r, err := h.roles.GetRole(c.Request.Context(), id)
	if err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.OK(c, r)
}

// GET /admin/permissions
func (h *AdminHandler) ListPermissions(c *gin.Context) {
	perms, err := h.roles.ListPermissions(c.Request.Context())
	if err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": perms})
}

type createRoleBody struct {
	Name            string   `json:"name"`
	DisplayName     string   `json:"display_name"`
	Description     string   `json:"description"`
	PermissionCodes []string `json:"permission_codes"`
}

// POST /admin/roles
func (h *AdminHandler) CreateRole(c *gin.Context) {
	var body createRoleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid json body")
		return
	}
	r, err := h.roles.CreateRole(c.Request.Context(), service.CreateRoleInput{
		Name: body.Name, DisplayName: body.DisplayName, Description: body.Description,
		PermissionCodes: body.PermissionCodes,
	})
	if err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.Created(c, r)
}

type updateRoleBody struct {
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

// PATCH /admin/roles/:id
func (h *AdminHandler) UpdateRole(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	var body updateRoleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid json body")
		return
	}
	if err := h.roles.UpdateRoleBasic(c.Request.Context(), id, body.DisplayName, body.Description); err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// DELETE /admin/roles/:id
func (h *AdminHandler) DeleteRole(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	if err := h.roles.DeleteRole(c.Request.Context(), id); err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

type setPermsBody struct {
	PermissionCodes []string `json:"permission_codes"`
}

// PUT /admin/roles/:id/permissions — replace all
func (h *AdminHandler) SetRolePermissions(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	var body setPermsBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid json body")
		return
	}
	if err := h.roles.SetPermissions(c.Request.Context(), id, body.PermissionCodes); err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// ---- Invite endpoints (public — untuk staff accept URL dari email) ----

// GET /invites/verify?token=xxx
func (h *AdminHandler) VerifyInvite(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "token wajib")
		return
	}
	v, err := h.invites.VerifyInvite(c.Request.Context(), token)
	if err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.OK(c, v)
}

type acceptInviteBody struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// POST /invites/accept
func (h *AdminHandler) AcceptInvite(c *gin.Context) {
	var body acceptInviteBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid json body")
		return
	}
	if body.Token == "" || body.Password == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "token & password wajib")
		return
	}
	if err := h.invites.AcceptInvite(c.Request.Context(), body.Token, body.Password); err != nil {
		adminMapErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// ---- helpers ----

func parseUUIDs(list []string) ([]uuid.UUID, error) {
	if len(list) == 0 {
		return nil, nil
	}
	out := make([]uuid.UUID, 0, len(list))
	for _, s := range list {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, errors.New("invalid uuid: " + s)
		}
		out = append(out, id)
	}
	return out, nil
}
