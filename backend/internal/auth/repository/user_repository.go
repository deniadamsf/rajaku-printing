// Package repository handles ONLY database access for auth module. No business
// logic — that lives in service. Handler MUST NOT import this package
// (spec section 22).
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/auth/model"
)

// ErrNotFound is a repository-level sentinel — service maps this to a domain
// error (mis. authapi.ErrUserNotFound) before returning to handler.
var ErrNotFound = errors.New("repository: not found")

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// FindByEmail returns the user + roles + permissions, or ErrNotFound.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).
		Preload("Roles.Permissions").
		Where("email = ?", email).
		First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find user by email %q: %w", email, err)
	}
	return &u, nil
}

// FindByPhone returns the user + roles + permissions, or ErrNotFound.
func (r *UserRepository) FindByPhone(ctx context.Context, phone string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).
		Preload("Roles.Permissions").
		Where("phone = ?", phone).
		First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find user by phone %q: %w", phone, err)
	}
	return &u, nil
}

// FindByID returns the user + roles + permissions, or ErrNotFound.
func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).
		Preload("Roles.Permissions").
		First(&u, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find user by id %s: %w", id, err)
	}
	return &u, nil
}

// Create inserts a new user. `u.ID` is populated by DB default (gen_random_uuid).
func (r *UserRepository) Create(ctx context.Context, u *model.User) error {
	if err := r.db.WithContext(ctx).Create(u).Error; err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// UpdateLastLogin sets last_login_at = now() for the given user.
func (r *UserRepository) UpdateLastLogin(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", id).
		UpdateColumn("last_login_at", gorm.Expr("NOW()"))
	if res.Error != nil {
		return fmt.Errorf("update last_login for %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// AssignRole links a user to a role. Idempotent — duplicate insert is silently OK
// via ON CONFLICT DO NOTHING (composite PK covers uniqueness).
func (r *UserRepository) AssignRole(ctx context.Context, userID, roleID uuid.UUID, assignedBy *uuid.UUID) error {
	ur := model.UserRole{UserID: userID, RoleID: roleID, AssignedBy: assignedBy}
	err := r.db.WithContext(ctx).
		Session(&gorm.Session{}).
		Exec(`INSERT INTO user_roles (user_id, role_id, assigned_by) VALUES (?, ?, ?)
              ON CONFLICT (user_id, role_id) DO NOTHING`,
			ur.UserID, ur.RoleID, ur.AssignedBy,
		).Error
	if err != nil {
		return fmt.Errorf("assign role %s to user %s: %w", roleID, userID, err)
	}
	return nil
}

// ExistsByEmail returns true if a user with the given email exists.
func (r *UserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("email = ?", email).Count(&n).Error
	if err != nil {
		return false, fmt.Errorf("count user by email: %w", err)
	}
	return n > 0, nil
}

// ExistsByPhone returns true if a user with the given phone exists.
func (r *UserRepository) ExistsByPhone(ctx context.Context, phone string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("phone = ?", phone).Count(&n).Error
	if err != nil {
		return false, fmt.Errorf("count user by phone: %w", err)
	}
	return n > 0, nil
}

// ---- Admin-facing (§10) ----

// ListStaffFilter — pagination + basic filter untuk daftar staff.
type ListStaffFilter struct {
	Q        string // substring on name/email/phone (case-insensitive)
	IsActive *bool  // nil = both; pointer supaya bisa distinguish false
	Page     int
	PageSize int
}

type ListStaffResult struct {
	Items    []model.User
	Total    int64
	Page     int
	PageSize int
}

// ListStaff paginated. Hanya user_type=staff. Preload roles (bukan permissions —
// hindari over-fetch untuk list view).
func (r *UserRepository) ListStaff(ctx context.Context, f ListStaffFilter) (*ListStaffResult, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&model.User{}).
		Where("user_type = ?", model.UserTypeStaff)
	if f.Q != "" {
		like := "%" + f.Q + "%"
		q = q.Where("name ILIKE ? OR email ILIKE ? OR phone ILIKE ?", like, like, like)
	}
	if f.IsActive != nil {
		q = q.Where("is_active = ?", *f.IsActive)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count staff: %w", err)
	}
	var items []model.User
	if err := q.
		Preload("Roles").
		Order("created_at DESC").
		Offset((f.Page - 1) * f.PageSize).
		Limit(f.PageSize).
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list staff: %w", err)
	}
	return &ListStaffResult{Items: items, Total: total, Page: f.Page, PageSize: f.PageSize}, nil
}

// UpdateStaffBasic — update field yg boleh diubah super admin (name, email,
// phone). Password diatur lewat SetPassword. is_active lewat SetActive. Return
// ErrNotFound kalau row tidak ada.
func (r *UserRepository) UpdateStaffBasic(ctx context.Context, id uuid.UUID, name, email, phone string) error {
	updates := map[string]any{"name": name, "phone": phone}
	// Distinguish "" (clear email) dari "tidak diubah" — untuk MVP anggap
	// email selalu diset (staff wajib email untuk login).
	if email != "" {
		updates["email"] = email
	}
	res := r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ? AND user_type = ?", id, model.UserTypeStaff).
		Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("update staff basic: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetActive toggles is_active. Return ErrNotFound kalau row tidak ada.
func (r *UserRepository) SetActive(ctx context.Context, id uuid.UUID, active bool) error {
	res := r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", id).
		UpdateColumn("is_active", active)
	if res.Error != nil {
		return fmt.Errorf("set active: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPassword hashes+writes password + activate user (dipakai flow accept
// invite). Caller wajib hash sendiri sebelum call — kita tidak import bcrypt
// dari repository (separation of concern §22).
func (r *UserRepository) SetPassword(ctx context.Context, id uuid.UUID, hash string) error {
	res := r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"password_hash": hash,
			"is_active":     true,
		})
	if res.Error != nil {
		return fmt.Errorf("set password: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ReplaceRoles atomically replaces user's role set. Cocok untuk super admin
// re-assign role staff. Insert AssignedBy untuk audit trail per row.
func (r *UserRepository) ReplaceRoles(ctx context.Context, userID uuid.UUID, roleIDs []uuid.UUID, assignedBy *uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).
			Delete(&model.UserRole{}).Error; err != nil {
			return fmt.Errorf("clear existing roles: %w", err)
		}
		if len(roleIDs) == 0 {
			return nil
		}
		rows := make([]model.UserRole, len(roleIDs))
		for i, rid := range roleIDs {
			rows[i] = model.UserRole{UserID: userID, RoleID: rid, AssignedBy: assignedBy}
		}
		if err := tx.Create(&rows).Error; err != nil {
			return fmt.Errorf("insert new roles: %w", err)
		}
		return nil
	})
}
