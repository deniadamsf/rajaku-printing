package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/auth/model"
)

var (
	ErrDuplicateName    = errors.New("role/repository: role name already exists")
	ErrSystemRoleLocked = errors.New("role/repository: system role cannot be modified/deleted")
)

const pgUniqueViolationCode = "23505"

type RoleRepository struct {
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

// FindByName returns a role (with permissions) by its unique name, or ErrNotFound.
func (r *RoleRepository) FindByName(ctx context.Context, name string) (*model.Role, error) {
	var role model.Role
	err := r.db.WithContext(ctx).
		Preload("Permissions").
		Where("name = ?", name).
		First(&role).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find role by name %q: %w", name, err)
	}
	return &role, nil
}

// FindByID returns a role (with permissions) by ID.
func (r *RoleRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Role, error) {
	var role model.Role
	err := r.db.WithContext(ctx).
		Preload("Permissions").
		First(&role, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find role by id %s: %w", id, err)
	}
	return &role, nil
}

// FindByIDs returns roles matching any of the given IDs (dipakai validasi
// bulk assign — pastikan semua ID valid sebelum insert).
func (r *RoleRepository) FindByIDs(ctx context.Context, ids []uuid.UUID) ([]model.Role, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var roles []model.Role
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("find roles by ids: %w", err)
	}
	return roles, nil
}

// ListAll returns all roles (with permissions), sorted by display_name.
func (r *RoleRepository) ListAll(ctx context.Context) ([]model.Role, error) {
	var roles []model.Role
	if err := r.db.WithContext(ctx).
		Preload("Permissions").
		Order("display_name ASC").
		Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	return roles, nil
}

// ListPermissions returns all menu_permissions (untuk admin UI toggle list).
func (r *RoleRepository) ListPermissions(ctx context.Context) ([]model.MenuPermission, error) {
	var perms []model.MenuPermission
	if err := r.db.WithContext(ctx).
		Order("category ASC, display_name ASC").
		Find(&perms).Error; err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	return perms, nil
}

// FindPermissionsByCodes returns permissions matching codes (validasi input).
func (r *RoleRepository) FindPermissionsByCodes(ctx context.Context, codes []string) ([]model.MenuPermission, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	var perms []model.MenuPermission
	if err := r.db.WithContext(ctx).Where("code IN ?", codes).Find(&perms).Error; err != nil {
		return nil, fmt.Errorf("find permissions by codes: %w", err)
	}
	return perms, nil
}

// Create inserts a new (non-system) role. Return ErrDuplicateName on unique
// violation.
func (r *RoleRepository) Create(ctx context.Context, role *model.Role) error {
	if err := r.db.WithContext(ctx).Create(role).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicateName
		}
		return fmt.Errorf("create role: %w", err)
	}
	return nil
}

// UpdateBasic updates display_name & description. System roles cannot be
// modified — return ErrSystemRoleLocked.
func (r *RoleRepository) UpdateBasic(ctx context.Context, id uuid.UUID, displayName string, description *string) error {
	var role model.Role
	if err := r.db.WithContext(ctx).First(&role, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("load role: %w", err)
	}
	if role.IsSystem {
		return ErrSystemRoleLocked
	}
	updates := map[string]any{"display_name": displayName, "description": description}
	if err := r.db.WithContext(ctx).Model(&model.Role{}).
		Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update role basic: %w", err)
	}
	return nil
}

// Delete removes a non-system role. FK cascade akan hapus role_permissions +
// user_roles otomatis.
func (r *RoleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	var role model.Role
	if err := r.db.WithContext(ctx).First(&role, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("load role: %w", err)
	}
	if role.IsSystem {
		return ErrSystemRoleLocked
	}
	if err := r.db.WithContext(ctx).Delete(&model.Role{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	return nil
}

// ReplacePermissions atomically replaces role's permission set. Boleh untuk
// role system SEKALIPUN — spec §10 explicit "toggle-able per role", termasuk
// role default (mis. super admin bisa cabut permission tertentu dari cashier
// tanpa hapus role-nya).
func (r *RoleRepository) ReplacePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM role_permissions WHERE role_id = ?", roleID).Error; err != nil {
			return fmt.Errorf("clear permissions: %w", err)
		}
		if len(permissionIDs) == 0 {
			return nil
		}
		// Bulk insert via raw values.
		values := make([]any, 0, len(permissionIDs)*2)
		placeholders := ""
		for i, pid := range permissionIDs {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "(?, ?)"
			values = append(values, roleID, pid)
		}
		sql := "INSERT INTO role_permissions (role_id, permission_id) VALUES " + placeholders
		if err := tx.Exec(sql, values...).Error; err != nil {
			return fmt.Errorf("insert permissions: %w", err)
		}
		return nil
	})
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolationCode
	}
	return false
}
