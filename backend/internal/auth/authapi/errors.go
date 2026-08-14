package authapi

import "errors"

// Sentinel errors exposed by the auth Service. Map to HTTP 401 at handler.
var (
	ErrInvalidToken = errors.New("authapi: invalid token")
	ErrExpiredToken = errors.New("authapi: expired token")

	// Login-specific
	ErrUserNotFound    = errors.New("authapi: user not found")
	ErrInvalidPassword = errors.New("authapi: invalid password")
	ErrUserInactive    = errors.New("authapi: user is inactive")

	// Registration
	ErrEmailAlreadyUsed = errors.New("authapi: email already registered")
	ErrPhoneAlreadyUsed = errors.New("authapi: phone already registered")

	// Lookup
	ErrCustomerNotFound = errors.New("authapi: customer not found")

	// Guest order ownership verification (POST /lacak/:resi/verify). Deliberately
	// generic — resi-not-found and phone-mismatch map to THIS SAME sentinel so
	// the HTTP response is identical for both cases (no resi enumeration).
	ErrGuestVerificationFailed = errors.New("authapi: order/phone verification failed")

	// Staff / role admin (§10)
	ErrStaffNotFound         = errors.New("authapi: staff not found")
	ErrRoleNotFound          = errors.New("authapi: role not found")
	ErrRoleDuplicateName     = errors.New("authapi: role name already exists")
	ErrRoleSystemLocked      = errors.New("authapi: system role cannot be modified/deleted")
	ErrInviteInvalid         = errors.New("authapi: invite token invalid")
	ErrInviteExpired         = errors.New("authapi: invite token expired")
	ErrInviteAlreadyUsed     = errors.New("authapi: invite token already used")
	ErrInvalidRoleAssignment = errors.New("authapi: role id list contains invalid entries")
	ErrInvalidPermissionSet  = errors.New("authapi: permission codes contain invalid entries")
)
