// Command seedadmin creates the initial super-admin user. Meant to be run
// once, manually, after applying migrations. Idempotent: if a user with the
// given email already exists, the command exits without changing anything
// (won't silently overwrite passwords).
//
// Usage:
//
//	go run ./cmd/seedadmin --email you@toko.id --phone 081234567890 --name "Owner" --password "strong-pass"
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/model"
	"github.com/rajaku-printing/backend/internal/auth/password"
	"github.com/rajaku-printing/backend/internal/auth/repository"
	"github.com/rajaku-printing/backend/internal/config"
	"github.com/rajaku-printing/backend/internal/database"
	"github.com/rajaku-printing/backend/internal/logger"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

func main() {
	var (
		email    = flag.String("email", "", "email super admin (wajib)")
		phoneRaw = flag.String("phone", "", "nomor WA super admin (wajib)")
		name     = flag.String("name", "", "nama tampilan (wajib)")
		pass     = flag.String("password", "", "password (min 8 karakter, wajib)")
	)
	flag.Parse()

	if err := run(*email, *phoneRaw, *name, *pass); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run(email, phoneRaw, name, pass string) error {
	if email == "" || phoneRaw == "" || name == "" || pass == "" {
		flag.Usage()
		return errors.New("email, phone, name, dan password wajib diisi")
	}
	if err := password.ValidateStrength(pass); err != nil {
		return err
	}
	normalizedPhone, err := phone.Normalize(phoneRaw)
	if err != nil {
		return fmt.Errorf("phone invalid: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger.Init(cfg.App.LogLevel, cfg.App.Env == config.EnvDevelopment)

	ctx := context.Background()
	db, err := database.Open(ctx, cfg.DB, cfg.App.Env == config.EnvDevelopment)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	users := repository.NewUserRepository(db)
	roles := repository.NewRoleRepository(db)

	// Idempotency check — jangan overwrite user existing.
	if _, err := users.FindByEmail(ctx, email); err == nil {
		log.Info().Str("email", email).Msg("user with this email already exists — nothing to do")
		return nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return fmt.Errorf("check existing: %w", err)
	}

	superRole, err := roles.FindByName(ctx, "super_admin")
	if err != nil {
		return fmt.Errorf("super_admin role not found — apakah migration 000001 sudah dijalankan? cause: %w", err)
	}

	hash, err := password.Hash(pass)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	u := &model.User{
		Email:        &email,
		Phone:        &normalizedPhone,
		Name:         name,
		PasswordHash: &hash,
		UserType:     model.UserTypeStaff,
		IsActive:     true,
	}
	if err := users.Create(ctx, u); err != nil {
		return fmt.Errorf("create super admin: %w", err)
	}
	if err := users.AssignRole(ctx, u.ID, superRole.ID, nil); err != nil {
		return fmt.Errorf("assign super_admin role: %w", err)
	}

	log.Info().
		Str("user_id", u.ID.String()).
		Str("email", email).
		Str("phone", normalizedPhone).
		Msg("super admin created")
	return nil
}
