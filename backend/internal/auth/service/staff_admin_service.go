// staff_admin_service.go — endpoint super admin untuk kelola staff (§10).
// Semua operasi assume caller sudah lewat middleware RequirePermission
// "staff.manage" (handler yg wire-in).
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
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

// CreateStaffInput — payload dari handler.
type CreateStaffInput struct {
	Name    string
	Email   string
	Phone   string   // raw, service normalize
	RoleIDs []uuid.UUID
	InviterID uuid.UUID
}

// CreateStaffResult — response ke super admin. Include raw invite token +
// URL supaya admin bisa forward manual ke staff (email/WA sender akan menyusul).
type CreateStaffResult struct {
	Staff      *model.User `json:"staff"`
	InviteURL  string      `json:"invite_url"`
	InviteToken string     `json:"invite_token"` // dicetak sekali, tidak bisa dilihat lagi
}

// UpdateStaffInput — patch shape untuk update staff. Kosong = tidak diubah.
type UpdateStaffInput struct {
	Name  string
	Email string
	Phone string // raw
}

// StaffAdminService — orkestrator kelola staff.
type StaffAdminService struct {
	users   *repository.UserRepository
	roles   *repository.RoleRepository
	invites *InviteService
	baseURL string // untuk invite URL — dari config.App.BaseURL
}

func NewStaffAdminService(users *repository.UserRepository, roles *repository.RoleRepository, invites *InviteService, baseURL string) *StaffAdminService {
	return &StaffAdminService{
		users:   users,
		roles:   roles,
		invites: invites,
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

// CreateStaff — bikin akun staff inactive + invite token. Super admin dapat
// URL untuk di-forward.
func (s *StaffAdminService) CreateStaff(ctx context.Context, in CreateStaffInput) (*CreateStaffResult, error) {
	name := strings.TrimSpace(in.Name)
	email := strings.TrimSpace(in.Email)
	if name == "" || email == "" || in.Phone == "" {
		return nil, fmt.Errorf("nama, email, phone wajib diisi")
	}
	phoneNorm, err := phone.Normalize(in.Phone)
	if err != nil {
		return nil, fmt.Errorf("phone: %w", err)
	}
	if in.InviterID == uuid.Nil {
		return nil, fmt.Errorf("inviter_id required")
	}

	// Dedup checks (application-level cheap probe; DB unique index tetap authoritative).
	if used, err := s.users.ExistsByEmail(ctx, email); err != nil {
		return nil, err
	} else if used {
		return nil, authapi.ErrEmailAlreadyUsed
	}
	if used, err := s.users.ExistsByPhone(ctx, phoneNorm); err != nil {
		return nil, err
	} else if used {
		return nil, authapi.ErrPhoneAlreadyUsed
	}

	// Validate role IDs sebelum insert.
	if len(in.RoleIDs) > 0 {
		found, err := s.roles.FindByIDs(ctx, in.RoleIDs)
		if err != nil {
			return nil, fmt.Errorf("validate role ids: %w", err)
		}
		if len(found) != len(in.RoleIDs) {
			return nil, authapi.ErrInvalidRoleAssignment
		}
	}

	u := &model.User{
		Email:    &email,
		Phone:    phoneNorm,
		Name:     name,
		UserType: model.UserTypeStaff,
		IsActive: false, // baru aktif setelah accept invite
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("create staff: %w", err)
	}

	// Assign roles (single txn kalau ada).
	if len(in.RoleIDs) > 0 {
		if err := s.users.ReplaceRoles(ctx, u.ID, in.RoleIDs, &in.InviterID); err != nil {
			return nil, fmt.Errorf("assign roles: %w", err)
		}
	}

	// Buat invite.
	rawToken, _, err := s.invites.CreateInvite(ctx, u.ID, in.InviterID)
	if err != nil {
		return nil, err
	}

	return &CreateStaffResult{
		Staff:       u,
		InviteToken: rawToken,
		InviteURL:   fmt.Sprintf("%s/invites/accept?token=%s", s.baseURL, rawToken),
	}, nil
}

// ListStaff paginated.
type ListStaffInput struct {
	Q        string
	IsActive *bool
	Page     int
	PageSize int
}

func (s *StaffAdminService) ListStaff(ctx context.Context, in ListStaffInput) (*repository.ListStaffResult, error) {
	res, err := s.users.ListStaff(ctx, repository.ListStaffFilter{
		Q: in.Q, IsActive: in.IsActive, Page: in.Page, PageSize: in.PageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("list staff: %w", err)
	}
	return res, nil
}

// GetStaff — return single staff w/ full roles + permissions.
func (s *StaffAdminService) GetStaff(ctx context.Context, id uuid.UUID) (*model.User, error) {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, authapi.ErrStaffNotFound
		}
		return nil, fmt.Errorf("find staff: %w", err)
	}
	if u.UserType != model.UserTypeStaff {
		return nil, authapi.ErrStaffNotFound
	}
	return u, nil
}

// UpdateStaff — patch basic fields. Password diatur via invite ulang (belum
// diimplementasi) atau flow reset password (juga menyusul).
func (s *StaffAdminService) UpdateStaff(ctx context.Context, id uuid.UUID, in UpdateStaffInput) error {
	name := strings.TrimSpace(in.Name)
	email := strings.TrimSpace(in.Email)
	if name == "" {
		return fmt.Errorf("nama wajib diisi")
	}
	phoneNorm := ""
	if in.Phone != "" {
		p, err := phone.Normalize(in.Phone)
		if err != nil {
			return fmt.Errorf("phone: %w", err)
		}
		phoneNorm = p
	}
	if err := s.users.UpdateStaffBasic(ctx, id, name, email, phoneNorm); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return authapi.ErrStaffNotFound
		}
		return fmt.Errorf("update staff: %w", err)
	}
	return nil
}

// DeactivateStaff — set is_active=false. Idempotent.
func (s *StaffAdminService) DeactivateStaff(ctx context.Context, id uuid.UUID) error {
	if err := s.users.SetActive(ctx, id, false); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return authapi.ErrStaffNotFound
		}
		return fmt.Errorf("deactivate: %w", err)
	}
	return nil
}

// ReactivateStaff — set is_active=true. Idempotent. Tidak reset password;
// password sudah tersimpan dari sebelum deactivate.
func (s *StaffAdminService) ReactivateStaff(ctx context.Context, id uuid.UUID) error {
	if err := s.users.SetActive(ctx, id, true); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return authapi.ErrStaffNotFound
		}
		return fmt.Errorf("reactivate: %w", err)
	}
	return nil
}

// AssignRoles — replace role set staff. Validasi semua ID exist dulu.
func (s *StaffAdminService) AssignRoles(ctx context.Context, staffID uuid.UUID, roleIDs []uuid.UUID, assignedBy uuid.UUID) error {
	// Validasi target staff exist + tipe.
	u, err := s.users.FindByID(ctx, staffID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return authapi.ErrStaffNotFound
		}
		return fmt.Errorf("load staff: %w", err)
	}
	if u.UserType != model.UserTypeStaff {
		return authapi.ErrStaffNotFound
	}
	// Validasi role IDs.
	if len(roleIDs) > 0 {
		found, err := s.roles.FindByIDs(ctx, roleIDs)
		if err != nil {
			return fmt.Errorf("validate roles: %w", err)
		}
		if len(found) != len(roleIDs) {
			return authapi.ErrInvalidRoleAssignment
		}
	}
	if err := s.users.ReplaceRoles(ctx, staffID, roleIDs, &assignedBy); err != nil {
		return fmt.Errorf("replace roles: %w", err)
	}
	return nil
}
