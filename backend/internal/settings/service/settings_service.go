// Package service — business logic modul settings (§19: retensi configurable).
package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/settings/model"
	settingsrepo "github.com/rajaku-printing/backend/internal/settings/repository"
	"github.com/rajaku-printing/backend/internal/settings/settingsapi"
)

// Store — narrowed repository contract untuk testability.
type Store interface {
	Get(ctx context.Context, key string) (*model.AppSetting, error)
	List(ctx context.Context) ([]model.AppSetting, error)
	Update(ctx context.Context, key, value string, actorID *uuid.UUID, at time.Time) error
}

// intRule — batas nilai yang masuk akal per key. Ini yang mencegah admin
// salah ketik "3000" jadi retensi 8 tahun, atau "0" yang bikin file kehapus
// seketika setelah upload.
type intRule struct {
	min, max int
	unit     string
}

// intRules — registry key bertipe integer. Key yang tidak terdaftar di sini
// (dan tidak di rule tipe lain) ditolak Update dengan ErrUnknownKey — setting
// baru WAJIB didaftarkan bareng migration seed-nya (§22 no-silent-stub).
var intRules = map[string]intRule{
	settingsapi.KeyDesignRetentionDays: {min: 1, max: 365, unit: "hari"},
}

type Service struct {
	store Store
	nowFn func() time.Time
}

var _ settingsapi.Reader = (*Service)(nil)

func New(store Store) *Service {
	return &Service{store: store, nowFn: time.Now}
}

// GetInt implements settingsapi.Reader.
//
// Sengaja TIDAK punya fallback diam-diam ke default: kalau row hilang, caller
// yang memutuskan (design retention job pakai nilai env sebagai fallback dan
// log warn). Menelan error di sini bikin misconfigurasi tidak kelihatan.
func (s *Service) GetInt(ctx context.Context, key string) (int, error) {
	row, err := s.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, settingsrepo.ErrNotFound) {
			return 0, settingsapi.ErrSettingNotFound
		}
		return 0, fmt.Errorf("settings: get %q: %w", key, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(row.Value))
	if err != nil {
		return 0, fmt.Errorf("settings: key %q value %q is not an integer: %w",
			key, row.Value, settingsapi.ErrInvalidValue)
	}
	return n, nil
}

// List mengembalikan semua setting untuk admin panel.
func (s *Service) List(ctx context.Context) ([]model.AppSetting, error) {
	items, err := s.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("settings: list: %w", err)
	}
	return items, nil
}

// Update memvalidasi lalu menyimpan nilai baru. Return row hasil update supaya
// handler bisa langsung balikin state terbaru ke UI.
func (s *Service) Update(ctx context.Context, key, rawValue string, actorID uuid.UUID) (*model.AppSetting, error) {
	key = strings.TrimSpace(key)
	value := strings.TrimSpace(rawValue)

	rule, ok := intRules[key]
	if !ok {
		return nil, settingsapi.ErrUnknownKey
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return nil, fmt.Errorf("settings: %q harus berupa angka, dapat %q: %w",
			key, rawValue, settingsapi.ErrInvalidValue)
	}
	if n < rule.min || n > rule.max {
		return nil, fmt.Errorf("settings: %q di luar rentang [%d, %d] %s, dapat %d: %w",
			key, rule.min, rule.max, rule.unit, n, settingsapi.ErrInvalidValue)
	}

	var actor *uuid.UUID
	if actorID != uuid.Nil {
		actor = &actorID
	}
	if err := s.store.Update(ctx, key, strconv.Itoa(n), actor, s.nowFn().UTC()); err != nil {
		if errors.Is(err, settingsrepo.ErrNotFound) {
			return nil, settingsapi.ErrSettingNotFound
		}
		return nil, fmt.Errorf("settings: update %q: %w", key, err)
	}

	row, err := s.store.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("settings: reload %q after update: %w", key, err)
	}
	return row, nil
}
