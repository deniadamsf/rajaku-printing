package service

import (
	"context"
	"errors"
	"testing"

	"github.com/rajaku-printing/backend/internal/notification/workerclient"
)

type fakeWorkerClient struct {
	status    *workerclient.PairingStatus
	statusErr error

	logoutErr    error
	logoutCalled bool
}

func (f *fakeWorkerClient) PairingStatus(_ context.Context) (*workerclient.PairingStatus, error) {
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return f.status, nil
}

func (f *fakeWorkerClient) Logout(_ context.Context) error {
	f.logoutCalled = true
	return f.logoutErr
}

func strPtr(s string) *string { return &s }

func TestPairingService_GetPairingStatus_HappyPath(t *testing.T) {
	fake := &fakeWorkerClient{
		status: &workerclient.PairingStatus{
			WAReady:    true,
			SelfNumber: strPtr("628123456789"),
			QRDataURL:  nil,
			Quota:      []byte(`{"used":1}`),
			Circuit:    []byte(`{"state":"closed"}`),
			Pacing:     []byte(`{"delay_ms":100}`),
		},
	}
	svc := NewPairingService(fake)

	view, err := svc.GetPairingStatus(context.Background())
	if err != nil {
		t.Fatalf("GetPairingStatus() error = %v", err)
	}
	if !view.Reachable {
		t.Fatal("Reachable = false, want true")
	}
	if !view.Connected {
		t.Error("Connected = false, want true")
	}
	if view.SelfNumber == nil || *view.SelfNumber != "628123456789" {
		t.Errorf("SelfNumber = %v", view.SelfNumber)
	}
	if len(view.Quota) == 0 {
		t.Error("Quota should be passed through, got empty")
	}
}

func TestPairingService_GetPairingStatus_WorkerUnreachable(t *testing.T) {
	fake := &fakeWorkerClient{statusErr: workerclient.ErrWorkerUnreachable}
	svc := NewPairingService(fake)

	view, err := svc.GetPairingStatus(context.Background())
	if err == nil {
		t.Fatal("expected error for logging purposes, got nil")
	}
	if !errors.Is(err, workerclient.ErrWorkerUnreachable) {
		t.Errorf("errors.Is(err, ErrWorkerUnreachable) = false, err = %v", err)
	}
	if view.Reachable {
		t.Error("Reachable = true, want false when worker unreachable")
	}
	if view.SelfNumber != nil || view.QRDataURL != nil {
		t.Error("expected empty view fields when worker unreachable")
	}
}

func TestPairingService_Unlink_HappyPath(t *testing.T) {
	fake := &fakeWorkerClient{}
	svc := NewPairingService(fake)

	if err := svc.Unlink(context.Background()); err != nil {
		t.Fatalf("Unlink() error = %v", err)
	}
	if !fake.logoutCalled {
		t.Error("expected Logout to be called on worker client")
	}
}

func TestPairingService_Unlink_WorkerUnreachable(t *testing.T) {
	fake := &fakeWorkerClient{logoutErr: workerclient.ErrWorkerUnreachable}
	svc := NewPairingService(fake)

	err := svc.Unlink(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil — unlink must fail loudly when worker is unreachable, not silently succeed")
	}
	if !errors.Is(err, workerclient.ErrWorkerUnreachable) {
		t.Errorf("errors.Is(err, ErrWorkerUnreachable) = false, err = %v", err)
	}
}
