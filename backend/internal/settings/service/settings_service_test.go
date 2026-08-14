package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/settings/model"
	settingsrepo "github.com/rajaku-printing/backend/internal/settings/repository"
	"github.com/rajaku-printing/backend/internal/settings/settingsapi"
)

type fakeStore struct {
	row        *model.AppSetting
	getErr     error
	updateErr  error
	updated    []updateCall
	listResult []model.AppSetting
	listErr    error
}

type updateCall struct {
	key   string
	value string
	actor *uuid.UUID
}

func (f *fakeStore) Get(_ context.Context, _ string) (*model.AppSetting, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.row, nil
}

func (f *fakeStore) List(_ context.Context) ([]model.AppSetting, error) {
	return f.listResult, f.listErr
}

func (f *fakeStore) Update(_ context.Context, key, value string, actor *uuid.UUID, _ time.Time) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = append(f.updated, updateCall{key: key, value: value, actor: actor})
	if f.row != nil {
		f.row.Value = value
	}
	return nil
}

func newTestSvc(store *fakeStore) *Service {
	s := New(store)
	s.nowFn = func() time.Time { return time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC) }
	return s
}

// ---------- happy path ----------

func TestGetInt_ReturnsStoredValue(t *testing.T) {
	svc := newTestSvc(&fakeStore{row: &model.AppSetting{
		Key:   settingsapi.KeyDesignRetentionDays,
		Value: "45",
	}})

	got, err := svc.GetInt(context.Background(), settingsapi.KeyDesignRetentionDays)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 45 {
		t.Errorf("want 45, got %d", got)
	}
}

func TestUpdate_PersistsValidValue(t *testing.T) {
	store := &fakeStore{row: &model.AppSetting{
		Key:   settingsapi.KeyDesignRetentionDays,
		Value: "30",
	}}
	svc := newTestSvc(store)
	actor := uuid.New()

	row, err := svc.Update(context.Background(), settingsapi.KeyDesignRetentionDays, " 14 ", actor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if row.Value != "14" {
		t.Errorf("value want 14, got %q", row.Value)
	}
	if len(store.updated) != 1 {
		t.Fatalf("want 1 update call, got %d", len(store.updated))
	}
	if store.updated[0].actor == nil || *store.updated[0].actor != actor {
		t.Error("actor harus tercatat untuk audit siapa yang ubah kebijakan")
	}
}

// ---------- failure path ----------

func TestUpdate_RejectsNonInteger(t *testing.T) {
	store := &fakeStore{row: &model.AppSetting{Key: settingsapi.KeyDesignRetentionDays, Value: "30"}}
	svc := newTestSvc(store)

	_, err := svc.Update(context.Background(), settingsapi.KeyDesignRetentionDays, "tigapuluh", uuid.New())
	if !errors.Is(err, settingsapi.ErrInvalidValue) {
		t.Fatalf("want ErrInvalidValue, got %v", err)
	}
	if len(store.updated) != 0 {
		t.Error("nilai invalid tidak boleh sampai ke store")
	}
}

func TestUpdate_RejectsOutOfRange(t *testing.T) {
	store := &fakeStore{row: &model.AppSetting{Key: settingsapi.KeyDesignRetentionDays, Value: "30"}}
	svc := newTestSvc(store)

	// 0 hari = file kehapus seketika setelah upload; harus ditolak.
	if _, err := svc.Update(context.Background(), settingsapi.KeyDesignRetentionDays, "0", uuid.New()); !errors.Is(err, settingsapi.ErrInvalidValue) {
		t.Errorf("retensi 0 hari harus ditolak, got %v", err)
	}
	if _, err := svc.Update(context.Background(), settingsapi.KeyDesignRetentionDays, "9999", uuid.New()); !errors.Is(err, settingsapi.ErrInvalidValue) {
		t.Errorf("retensi 9999 hari harus ditolak, got %v", err)
	}
	if len(store.updated) != 0 {
		t.Error("nilai di luar rentang tidak boleh sampai ke store")
	}
}

func TestUpdate_RejectsUnknownKey(t *testing.T) {
	store := &fakeStore{}
	svc := newTestSvc(store)

	_, err := svc.Update(context.Background(), "retensi_karangan", "10", uuid.New())
	if !errors.Is(err, settingsapi.ErrUnknownKey) {
		t.Fatalf("want ErrUnknownKey, got %v", err)
	}
}

func TestGetInt_MissingRowSurfacesError(t *testing.T) {
	svc := newTestSvc(&fakeStore{getErr: settingsrepo.ErrNotFound})

	_, err := svc.GetInt(context.Background(), settingsapi.KeyDesignRetentionDays)
	if !errors.Is(err, settingsapi.ErrSettingNotFound) {
		t.Fatalf("want ErrSettingNotFound, got %v", err)
	}
}

func TestGetInt_CorruptValueSurfacesError(t *testing.T) {
	svc := newTestSvc(&fakeStore{row: &model.AppSetting{
		Key:   settingsapi.KeyDesignRetentionDays,
		Value: "dua puluh",
	}})

	_, err := svc.GetInt(context.Background(), settingsapi.KeyDesignRetentionDays)
	if !errors.Is(err, settingsapi.ErrInvalidValue) {
		t.Fatalf("want ErrInvalidValue, got %v", err)
	}
}
