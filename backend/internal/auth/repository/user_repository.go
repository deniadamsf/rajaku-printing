// Package repository handles ONLY database access for auth module. No business
// logic — that lives in service. Handler MUST NOT import this package
// (spec section 22).
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/rajaku-printing/backend/internal/auth/model"
)

// gormForUpdate — row-level lock clause (`SELECT ... FOR UPDATE`), same
// pattern as internal/order/repository/order_repository.go's helper of the
// same name.
func gormForUpdate() clause.Locking { return clause.Locking{Strength: "UPDATE"} }

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

// FindByPhoneForUpdate is FindByPhone's row-locking counterpart
// (`SELECT ... FOR UPDATE`) — MUST be called inside an open transaction.
// Used by PhoneClaimService.claimOther's authoritative in-transaction
// re-check (§ phone-claim review finding #1): without the lock, the owner
// row could change type (guest → registered, e.g. via a concurrent
// UpgradeGuestToRegistered) between this read and the absorption writes that
// follow it in the same transaction, letting a since-upgraded account get
// silently merged into and tombstoned. Deliberately skips the
// Roles.Permissions preload FindByPhone carries — this caller only ever
// inspects UserType/CustomerType, and preloading roles here would be a
// second round-trip inside an already-locked row's transaction for no
// benefit.
func (r *UserRepository) FindByPhoneForUpdate(ctx context.Context, phone string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).
		Clauses(gormForUpdate()).
		Where("phone = ?", phone).
		First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find user by phone (for update) %q: %w", phone, err)
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

// FindByOAuth returns the user linked to the given provider+subject, or
// ErrNotFound.
func (r *UserRepository) FindByOAuth(ctx context.Context, provider, subject string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).
		Preload("Roles.Permissions").
		Where("oauth_provider = ? AND oauth_subject = ?", provider, subject).
		First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find user by oauth %s/%s: %w", provider, subject, err)
	}
	return &u, nil
}

// LinkOAuth attaches provider+subject to an existing user — used when a
// customer's verified Google email matches an account that doesn't have a
// linked Google identity yet. Return ErrNotFound kalau row tidak ada.
func (r *UserRepository) LinkOAuth(ctx context.Context, id uuid.UUID, provider, subject string) error {
	res := r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", id).
		Updates(map[string]any{"oauth_provider": provider, "oauth_subject": subject})
	if res.Error != nil {
		return fmt.Errorf("link oauth for user %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpgradeGuestToRegistered promotes a guest customer (created e.g. via POS,
// §11) to a registered customer with a linked OAuth identity — §11: nomor WA
// adalah matching key tunggal, jadi upgrade ini menyambung riwayat transaksi
// yang sudah ada ke akun baru, bukan membuat identitas terpisah.
//
// `email` and `name` are only written when the existing column is empty
// (email IS NULL / name = ”) — the phone number is the durable matching
// key; Google login must never clobber data that's already on file. Pass
// email=nil to leave the email column untouched entirely (caller already
// determined the Google email is taken by a different user).
//
// Always stamps phone_verified_at = NOW() — this method is ONLY ever called
// after the caller has already proven ownership of the guest's phone number
// via a verified OTP (§ phone-claim review; a guest's phone is unverified by
// construction since POS never runs an OTP round-trip), so the upgrade is
// exactly the moment that number becomes provably owned.
func (r *UserRepository) UpgradeGuestToRegistered(ctx context.Context, id uuid.UUID, email *string, name, provider, subject string) error {
	var emailArg any
	if email != nil {
		emailArg = *email
	}
	res := r.db.WithContext(ctx).Exec(`
		UPDATE users SET
			customer_type     = 'registered',
			oauth_provider    = ?,
			oauth_subject     = ?,
			email             = CASE WHEN email IS NULL THEN ? ELSE email END,
			name              = CASE WHEN name = ''    THEN ? ELSE name  END,
			phone_verified_at = NOW(),
			updated_at        = NOW()
		WHERE id = ?
	`, provider, subject, emailArg, name, id)
	if res.Error != nil {
		return fmt.Errorf("upgrade guest to registered for user %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPhone overwrites a user's phone number and its verification stamp
// directly. `phone` nil clears it to NULL (used to release a phone from a
// guest row being absorbed into an authenticated user's account — §ownership
// review, PhoneClaimService.Claim). `verifiedAt` nil means "claimed but not
// proven"; non-nil records when OTP ownership was proven. Return ErrNotFound
// kalau row tidak ada.
func (r *UserRepository) SetPhone(ctx context.Context, id uuid.UUID, phone *string, verifiedAt *time.Time) error {
	res := r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", id).
		Updates(map[string]any{"phone": phone, "phone_verified_at": verifiedAt})
	if res.Error != nil {
		return fmt.Errorf("set phone for user %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
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
// phone). `phone` nil = clear to NULL (mirrors the pre-existing "not
// provided in patch" behavior — previously stored as a bogus empty string,
// now correctly NULL since the column is nullable, migration 000017).
// Password diatur lewat SetPassword. is_active lewat SetActive. Return
// ErrNotFound kalau row tidak ada.
func (r *UserRepository) UpdateStaffBasic(ctx context.Context, id uuid.UUID, name, email string, phone *string) error {
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
