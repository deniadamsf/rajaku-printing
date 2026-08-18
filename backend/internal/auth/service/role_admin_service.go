// role_admin_service.go — endpoint super admin untuk kelola role & permission
// toggle-able (§10). Semua operasi assume caller sudah lewat permission
// "role.manage".
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/model"
	"github.com/rajaku-printing/backend/internal/auth/repository"
)

type RoleAdminService struct {
	roles *repository.RoleRepository
}

func NewRoleAdminService(roles *repository.RoleRepository) *RoleAdminService {
	return &RoleAdminService{roles: roles}
}

// ListRoles — semua role + permissions (untuk admin UI).
func (s *RoleAdminService) ListRoles(ctx context.Context) ([]model.Role, error) {
	return s.roles.ListAll(ctx)
}

// GetRole — single role by ID.
func (s *RoleAdminService) GetRole(ctx context.Context, id uuid.UUID) (*model.Role, error) {
	r, err := s.roles.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, authapi.ErrRoleNotFound
		}
		return nil, fmt.Errorf("find role: %w", err)
	}
	return r, nil
}

// ListPermissions — semua permission (grouped by category di client).
func (s *RoleAdminService) ListPermissions(ctx context.Context) ([]model.MenuPermission, error) {
	return s.roles.ListPermissions(ctx)
}

// CreateRoleInput — payload create role custom (non-system).
type CreateRoleInput struct {
	Name            string // unique; snake_case biasanya
	DisplayName     string
	Description     string
	PermissionCodes []string // optional; kosong = role tanpa permission (still valid)
}

func (s *RoleAdminService) CreateRole(ctx context.Context, in CreateRoleInput) (*model.Role, error) {
	name := strings.TrimSpace(in.Name)
	display := strings.TrimSpace(in.DisplayName)
	if name == "" || display == "" {
		return nil, fmt.Errorf("name dan display_name wajib diisi")
	}

	// Validasi permission codes dulu (biar tidak insert role lalu gagal
	// attach permission dgn state inkonsisten).
	var permIDs []uuid.UUID
	if len(in.PermissionCodes) > 0 {
		perms, err := s.roles.FindPermissionsByCodes(ctx, in.PermissionCodes)
		if err != nil {
			return nil, fmt.Errorf("validate permissions: %w", err)
		}
		if len(perms) != len(in.PermissionCodes) {
			return nil, authapi.ErrInvalidPermissionSet
		}
		permIDs = make([]uuid.UUID, len(perms))
		for i, p := range perms {
			permIDs[i] = p.ID
		}
	}

	role := &model.Role{
		Name:        name,
		DisplayName: display,
		IsSystem:    false,
	}
	if in.Description != "" {
		d := in.Description
		role.Description = &d
	}
	if err := s.roles.Create(ctx, role); err != nil {
		if errors.Is(err, repository.ErrDuplicateName) {
			return nil, authapi.ErrRoleDuplicateName
		}
		return nil, fmt.Errorf("create role: %w", err)
	}
	if len(permIDs) > 0 {
		if err := s.roles.ReplacePermissions(ctx, role.ID, permIDs); err != nil {
			return nil, fmt.Errorf("attach permissions: %w", err)
		}
	}
	// Re-load role dgn permissions untuk return.
	return s.roles.FindByID(ctx, role.ID)
}

// UpdateRoleBasic — hanya display_name + description. Name & is_system tidak
// boleh diubah. Return ErrRoleSystemLocked untuk role system.
func (s *RoleAdminService) UpdateRoleBasic(ctx context.Context, id uuid.UUID, displayName, description string) error {
	display := strings.TrimSpace(displayName)
	if display == "" {
		return fmt.Errorf("display_name wajib diisi")
	}
	var descPtr *string
	if description != "" {
		descPtr = &description
	}
	err := s.roles.UpdateBasic(ctx, id, display, descPtr)
	if errors.Is(err, repository.ErrNotFound) {
		return authapi.ErrRoleNotFound
	}
	if errors.Is(err, repository.ErrSystemRoleLocked) {
		return authapi.ErrRoleSystemLocked
	}
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	return nil
}

// DeleteRole — hard delete. Cuma non-system.
func (s *RoleAdminService) DeleteRole(ctx context.Context, id uuid.UUID) error {
	err := s.roles.Delete(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return authapi.ErrRoleNotFound
	}
	if errors.Is(err, repository.ErrSystemRoleLocked) {
		return authapi.ErrRoleSystemLocked
	}
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	return nil
}

// SetPermissions — replace permission set untuk role (bisa role system atau
// custom — §10 explicit "toggle-able").
func (s *RoleAdminService) SetPermissions(ctx context.Context, roleID uuid.UUID, permissionCodes []string) error {
	// Pastikan role exist.
	if _, err := s.roles.FindByID(ctx, roleID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return authapi.ErrRoleNotFound
		}
		return fmt.Errorf("load role: %w", err)
	}
	var permIDs []uuid.UUID
	if len(permissionCodes) > 0 {
		perms, err := s.roles.FindPermissionsByCodes(ctx, permissionCodes)
		if err != nil {
			return fmt.Errorf("validate permissions: %w", err)
		}
		if len(perms) != len(permissionCodes) {
			return authapi.ErrInvalidPermissionSet
		}
		permIDs = make([]uuid.UUID, len(perms))
		for i, p := range perms {
			permIDs[i] = p.ID
		}
	}
	if err := s.roles.ReplacePermissions(ctx, roleID, permIDs); err != nil {
		return fmt.Errorf("replace permissions: %w", err)
	}
	return nil
}
