package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	paymodel "github.com/rajaku-printing/backend/internal/payment/model"
)

// TestToCustomerProofResponse_DoesNotLeakReviewedBy is a regression lock: the
// customer-facing DTO must NEVER include reviewed_by (the staff UUID who
// verified the proof) — that's internal-only data. It must ALWAYS include
// reject_reason, since that's the one signal a customer has for why their
// upload was rejected and needs re-uploading (§4/§7).
func TestToCustomerProofResponse_DoesNotLeakReviewedBy(t *testing.T) {
	reviewer := uuid.New()
	reason := "foto buram, nominal tidak terbaca"
	now := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	proof := &paymodel.PaymentProof{
		ID:               uuid.New(),
		OrderID:          uuid.New(), // must not leak either — redundant with :resi path
		MetodeBayar:      paymodel.MetodeBayarTransfer,
		FileOriginalName: "bukti.jpg",
		FileSizeBytes:    12345,
		FileMimeType:     "image/jpeg",
		UploadedAt:       now,
		Status:           paymodel.ProofRejected,
		ReviewedAt:       &now,
		ReviewedBy:       &reviewer,
		RejectReason:     &reason,
	}

	out := toCustomerProofResponse(proof)
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(raw)

	if strings.Contains(body, "reviewed_by") {
		t.Fatalf("customer proof response leaks reviewed_by: %s", body)
	}
	if strings.Contains(body, reviewer.String()) {
		t.Fatalf("customer proof response leaks staff UUID value: %s", body)
	}
	if strings.Contains(body, "order_id") {
		t.Fatalf("customer proof response leaks order_id (redundant w/ :resi path): %s", body)
	}
	if !strings.Contains(body, `"reject_reason":"foto buram, nominal tidak terbaca"`) {
		t.Fatalf("customer proof response must carry reject_reason so customer knows why: %s", body)
	}
	if out.Status != "rejected" {
		t.Errorf("status mismatch: %s", out.Status)
	}
}

func TestToCustomerProofResponse_PendingHasNoReviewFields(t *testing.T) {
	now := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	proof := &paymodel.PaymentProof{
		ID:               uuid.New(),
		OrderID:          uuid.New(),
		MetodeBayar:      paymodel.MetodeBayarQRIS,
		FileOriginalName: "bukti.png",
		FileSizeBytes:    999,
		FileMimeType:     "image/png",
		UploadedAt:       now,
		Status:           paymodel.ProofPending,
	}

	out := toCustomerProofResponse(proof)
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(raw)

	if strings.Contains(body, "reviewed_at") || strings.Contains(body, "reject_reason") {
		t.Fatalf("pending proof must omit empty review fields (omitempty), got: %s", body)
	}
}
