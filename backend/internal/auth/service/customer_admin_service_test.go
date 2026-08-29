package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/model"
	"github.com/rajaku-printing/backend/internal/auth/repository"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
)

// ---------------------------------------------------------------------------
// fakeCustomerAdminUserStore / fakeCustomerAdminLogStore / fakeOrderReader —
// in-memory stand-ins for customerAdminUserStore/customerAdminLogStore/
// orderapi.CustomerOrderReader, pola sama fakeCustomerUserStore di
// customer_service_test.go.
// ---------------------------------------------------------------------------

type fakeCustomerAdminUserStore struct {
	byID map[uuid.UUID]*model.User

	emailConflict bool
	phoneConflict bool

	updateCalls               int
	lastUpdateName            string
	lastUpdateEmail           *string
	lastUpdatePhone           *string
	lastUpdatePhoneVerifiedAt *time.Time
	updateErr                 error

	setActiveCalls int
	setActiveErr   error

	listTotal int64
	listErr   error

	streamBatches [][]model.User
	streamErr     error
}

func (f *fakeCustomerAdminUserStore) ListCustomers(_ context.Context, _ repository.ListCustomerFilter) (*repository.ListCustomerResult, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &repository.ListCustomerResult{Total: f.listTotal, Page: 1, PageSize: 1}, nil
}

func (f *fakeCustomerAdminUserStore) StreamCustomers(_ context.Context, _ repository.ListCustomerFilter, _ int, fn func([]model.User) error) error {
	if f.streamErr != nil {
		return f.streamErr
	}
	for _, batch := range f.streamBatches {
		if err := fn(batch); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeCustomerAdminUserStore) FindCustomerByID(_ context.Context, id uuid.UUID) (*model.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (f *fakeCustomerAdminUserStore) UpdateCustomerBasic(_ context.Context, id uuid.UUID, name string, email, phoneNum *string, phoneVerifiedAt *time.Time) error {
	f.updateCalls++
	f.lastUpdateName = name
	f.lastUpdateEmail = email
	f.lastUpdatePhone = phoneNum
	f.lastUpdatePhoneVerifiedAt = phoneVerifiedAt
	if f.updateErr != nil {
		return f.updateErr
	}
	u, ok := f.byID[id]
	if !ok {
		return repository.ErrNotFound
	}
	u.Name = name
	u.Email = email
	u.Phone = phoneNum
	u.PhoneVerifiedAt = phoneVerifiedAt
	return nil
}

func (f *fakeCustomerAdminUserStore) SetCustomerActive(_ context.Context, id uuid.UUID, active bool) error {
	f.setActiveCalls++
	if f.setActiveErr != nil {
		return f.setActiveErr
	}
	u, ok := f.byID[id]
	if !ok {
		return repository.ErrNotFound
	}
	u.IsActive = active
	return nil
}

func (f *fakeCustomerAdminUserStore) ExistsByEmailExcluding(_ context.Context, _ string, _ uuid.UUID) (bool, error) {
	return f.emailConflict, nil
}

func (f *fakeCustomerAdminUserStore) ExistsByPhoneExcluding(_ context.Context, _ string, _ uuid.UUID) (bool, error) {
	return f.phoneConflict, nil
}

type fakeCustomerAdminLogStore struct {
	createCalls int
	lastLog     *model.CustomerAdminLog
	createErr   error

	listResult []repository.CustomerAdminLogView
	listErr    error
}

func (f *fakeCustomerAdminLogStore) Create(_ context.Context, l *model.CustomerAdminLog) error {
	f.createCalls++
	f.lastLog = l
	return f.createErr
}

func (f *fakeCustomerAdminLogStore) ListByCustomer(_ context.Context, _ uuid.UUID, _ int) ([]repository.CustomerAdminLogView, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listResult, nil
}

type fakeOrderReader struct {
	overview *orderapi.CustomerOrderOverview
	err      error
}

func (f *fakeOrderReader) CustomerOrderOverview(_ context.Context, _ uuid.UUID, _ int) (*orderapi.CustomerOrderOverview, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.overview != nil {
		return f.overview, nil
	}
	return &orderapi.CustomerOrderOverview{}, nil
}

// fakeCustomerAdminTxRunner meniru semantik transaksi tanpa DB: fn dijalankan
// atas fake store yang SAMA dengan jalur baca, dan kalau fn mengembalikan
// error, seluruh tulisan di dalamnya dibatalkan (rollback ditiru dengan
// mengembalikan snapshot user + membuang baris log yang sempat ditulis).
// Tanpa peniruan rollback ini, test "audit log gagal → status tidak berubah"
// (temuan review #7) akan lolos palsu.
type fakeCustomerAdminTxRunner struct {
	users *fakeCustomerAdminUserStore
	logs  *fakeCustomerAdminLogStore
}

func (r *fakeCustomerAdminTxRunner) RunInTx(ctx context.Context, fn func(tx customerAdminTx) error) error {
	snapshot := make(map[uuid.UUID]model.User, len(r.users.byID))
	for id, u := range r.users.byID {
		snapshot[id] = *u
	}
	logCallsBefore := r.logs.createCalls

	if err := fn(customerAdminTx{Users: r.users, Logs: r.logs}); err != nil {
		for id := range r.users.byID {
			if prev, ok := snapshot[id]; ok {
				restored := prev
				r.users.byID[id] = &restored
			}
		}
		r.logs.createCalls = logCallsBefore
		r.logs.lastLog = nil
		return err
	}
	return nil
}

func newTestCustomerAdminService(users *fakeCustomerAdminUserStore, logs *fakeCustomerAdminLogStore) *CustomerAdminService {
	if users == nil {
		users = &fakeCustomerAdminUserStore{}
	}
	if logs == nil {
		logs = &fakeCustomerAdminLogStore{}
	}
	return &CustomerAdminService{
		users:  users,
		logs:   logs,
		orders: &fakeOrderReader{},
		tx:     &fakeCustomerAdminTxRunner{users: users, logs: logs},
	}
}

func registeredCustomerType() *model.CustomerType {
	t := model.CustomerTypeRegistered
	return &t
}

// --- ListCustomers ---

// (1) Filter customer_type tak dikenal ditolak SEBELUM repository dipanggil
// (typo tidak boleh diam-diam mengembalikan 0 baris).
func TestCustomerAdminService_ListCustomers_UnknownFilter_Rejected(t *testing.T) {
	svc := newTestCustomerAdminService(nil, nil)

	_, err := svc.ListCustomers(context.Background(), ListCustomerInput{CustomerType: "bogus"})
	if !errors.Is(err, ErrInvalidCustomerFilter) {
		t.Fatalf("expected ErrInvalidCustomerFilter, got %v", err)
	}
}

func TestCustomerAdminService_ListCustomers_UnknownMembershipFilter_Rejected(t *testing.T) {
	svc := newTestCustomerAdminService(nil, nil)

	_, err := svc.ListCustomers(context.Background(), ListCustomerInput{MembershipStatus: "banned"})
	if !errors.Is(err, ErrInvalidCustomerFilter) {
		t.Fatalf("expected ErrInvalidCustomerFilter, got %v", err)
	}
}

// --- UpdateCustomer ---

// (2) Happy path: nomor WA lokal ("0812...") dinormalisasi ke 62xxx, dan
// KARENA nomornya berubah, phone_verified_at WAJIB ditulis NULL eksplisit
// (§ aturan keamanan #3) — bukan dibiarkan menempel dari sebelumnya.
func TestCustomerAdminService_UpdateCustomer_HappyPath_NormalizesPhoneAndClearsVerification(t *testing.T) {
	custID := uuid.New()
	oldPhone := "6281200000001"
	verifiedAt := time.Now().Add(-24 * time.Hour)
	users := &fakeCustomerAdminUserStore{byID: map[uuid.UUID]*model.User{
		custID: {
			ID: custID, Name: "Lama", Phone: &oldPhone, PhoneVerifiedAt: &verifiedAt,
			UserType: model.UserTypeCustomer, CustomerType: registeredCustomerType(), IsActive: true,
		},
	}}
	svc := newTestCustomerAdminService(users, nil)

	newName := "Budi Santoso"
	rawPhone := "081234567890"
	out, err := svc.UpdateCustomer(context.Background(), UpdateCustomerInput{
		CustomerID: custID, Name: &newName, Phone: &rawPhone,
	})
	if err != nil {
		t.Fatalf("UpdateCustomer: unexpected err: %v", err)
	}
	if out.Name != newName {
		t.Fatalf("expected name %q, got %q", newName, out.Name)
	}
	if out.Phone == nil || *out.Phone != "6281234567890" {
		t.Fatalf("expected normalized phone 6281234567890, got %v", out.Phone)
	}
	if users.lastUpdatePhoneVerifiedAt != nil {
		t.Fatalf("expected phone_verified_at written NULL when phone changes, got %v", users.lastUpdatePhoneVerifiedAt)
	}
	if out.PhoneVerified {
		t.Fatal("expected phone_verified=false in response after number changed")
	}
}

// (3) Nomor WA baru sudah dipakai pelanggan/akun lain → ErrPhoneTaken, dan
// UpdateCustomerBasic TIDAK PERNAH dipanggil (dedup check duluan).
func TestCustomerAdminService_UpdateCustomer_PhoneTaken_Rejected(t *testing.T) {
	custID := uuid.New()
	users := &fakeCustomerAdminUserStore{
		byID:          map[uuid.UUID]*model.User{custID: {ID: custID, Name: "Budi", UserType: model.UserTypeCustomer, CustomerType: registeredCustomerType(), IsActive: true}},
		phoneConflict: true,
	}
	svc := newTestCustomerAdminService(users, nil)

	rawPhone := "081234567890"
	_, err := svc.UpdateCustomer(context.Background(), UpdateCustomerInput{CustomerID: custID, Phone: &rawPhone})
	if !errors.Is(err, ErrPhoneTaken) {
		t.Fatalf("expected ErrPhoneTaken, got %v", err)
	}
	if users.updateCalls != 0 {
		t.Fatalf("expected UpdateCustomerBasic NOT called when phone dedup fails, got %d calls", users.updateCalls)
	}
}

// Email baru sudah dipakai pelanggan/akun lain → ErrEmailTaken.
func TestCustomerAdminService_UpdateCustomer_EmailTaken_Rejected(t *testing.T) {
	custID := uuid.New()
	users := &fakeCustomerAdminUserStore{
		byID:          map[uuid.UUID]*model.User{custID: {ID: custID, Name: "Budi", UserType: model.UserTypeCustomer, CustomerType: registeredCustomerType(), IsActive: true}},
		emailConflict: true,
	}
	svc := newTestCustomerAdminService(users, nil)

	newEmail := "dipakai@example.com"
	_, err := svc.UpdateCustomer(context.Background(), UpdateCustomerInput{CustomerID: custID, Email: &newEmail})
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
	if users.updateCalls != 0 {
		t.Fatalf("expected UpdateCustomerBasic NOT called when email dedup fails, got %d calls", users.updateCalls)
	}
}

// (3b) Email disimpan lowercase walau admin mengetik kapital campuran, dan
// pengecekan duplikat memakai bentuk lowercase itu (temuan review #4).
func TestCustomerAdminService_UpdateCustomer_Email_LowercasedOnSave(t *testing.T) {
	custID := uuid.New()
	users := &fakeCustomerAdminUserStore{
		byID: map[uuid.UUID]*model.User{custID: {ID: custID, Name: "Budi", UserType: model.UserTypeCustomer, CustomerType: registeredCustomerType(), IsActive: true}},
	}
	svc := newTestCustomerAdminService(users, nil)

	mixedCase := "Budi@Gmail.com"
	out, err := svc.UpdateCustomer(context.Background(), UpdateCustomerInput{CustomerID: custID, Email: &mixedCase})
	if err != nil {
		t.Fatalf("UpdateCustomer: unexpected err: %v", err)
	}
	if out.Email == nil || *out.Email != "budi@gmail.com" {
		t.Fatalf("expected lowercased email budi@gmail.com, got %v", out.Email)
	}
	if users.lastUpdateEmail == nil || *users.lastUpdateEmail != "budi@gmail.com" {
		t.Fatalf("expected UpdateCustomerBasic written with lowercased email, got %v", users.lastUpdateEmail)
	}
}

// (3c) Duplikat terdeteksi walau kapitalisasinya beda — dedup check DIKIRIM
// bentuk lowercase, bukan mentah dari input.
func TestCustomerAdminService_UpdateCustomer_EmailTaken_DetectedRegardlessOfCase(t *testing.T) {
	custID := uuid.New()
	users := &fakeCustomerAdminUserStore{
		byID:          map[uuid.UUID]*model.User{custID: {ID: custID, Name: "Budi", UserType: model.UserTypeCustomer, CustomerType: registeredCustomerType(), IsActive: true}},
		emailConflict: true,
	}
	svc := newTestCustomerAdminService(users, nil)

	mixedCase := "Dipakai@Example.COM"
	_, err := svc.UpdateCustomer(context.Background(), UpdateCustomerInput{CustomerID: custID, Email: &mixedCase})
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

// (3d) Registered customer + email="" ditolak SEBELUM mencapai repository
// (temuan review #5 — CHECK users_email_required).
func TestCustomerAdminService_UpdateCustomer_RegisteredClearEmail_Rejected(t *testing.T) {
	custID := uuid.New()
	users := &fakeCustomerAdminUserStore{
		byID: map[uuid.UUID]*model.User{custID: {ID: custID, Name: "Budi", UserType: model.UserTypeCustomer, CustomerType: registeredCustomerType(), IsActive: true}},
	}
	svc := newTestCustomerAdminService(users, nil)

	empty := ""
	_, err := svc.UpdateCustomer(context.Background(), UpdateCustomerInput{CustomerID: custID, Email: &empty})
	if !errors.Is(err, ErrEmailRequiredForRegistered) {
		t.Fatalf("expected ErrEmailRequiredForRegistered, got %v", err)
	}
	if users.updateCalls != 0 {
		t.Fatalf("expected UpdateCustomerBasic NOT called when clearing email on a registered customer, got %d calls", users.updateCalls)
	}
}

// (3e) Guest customer + phone="" ditolak SEBELUM mencapai repository (temuan
// review #5 — nomor WA adalah satu-satunya matching key identitas guest).
func TestCustomerAdminService_UpdateCustomer_GuestClearPhone_Rejected(t *testing.T) {
	custID := uuid.New()
	guestPhone := "6281200000001"
	guestType := model.CustomerTypeGuest
	users := &fakeCustomerAdminUserStore{
		byID: map[uuid.UUID]*model.User{custID: {
			ID: custID, Name: "Budi", Phone: &guestPhone,
			UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: true,
		}},
	}
	svc := newTestCustomerAdminService(users, nil)

	empty := ""
	_, err := svc.UpdateCustomer(context.Background(), UpdateCustomerInput{CustomerID: custID, Phone: &empty})
	if !errors.Is(err, ErrPhoneRequiredForGuest) {
		t.Fatalf("expected ErrPhoneRequiredForGuest, got %v", err)
	}
	if users.updateCalls != 0 {
		t.Fatalf("expected UpdateCustomerBasic NOT called when clearing phone on a guest customer, got %d calls", users.updateCalls)
	}
}

// (4) SECURITY-RELEVANT: mengedit ID yang bukan pelanggan (mis. staff) HARUS
// gagal dengan not-found — buktinya filter user_type='customer' di
// FindCustomerByID benar-benar bekerja (§ aturan keamanan #1). Simulasi:
// baris "staff" tidak pernah masuk ke peta byID (persis seperti
// FindCustomerByID yang menyaring WHERE user_type='customer' di DB nyata).
func TestCustomerAdminService_UpdateCustomer_TargetIsStaffRow_NotFound(t *testing.T) {
	staffID := uuid.New()
	users := &fakeCustomerAdminUserStore{byID: map[uuid.UUID]*model.User{}}
	svc := newTestCustomerAdminService(users, nil)

	newName := "Percobaan Escalation"
	_, err := svc.UpdateCustomer(context.Background(), UpdateCustomerInput{CustomerID: staffID, Name: &newName})
	if !errors.Is(err, authapi.ErrCustomerNotFound) {
		t.Fatalf("expected authapi.ErrCustomerNotFound, got %v", err)
	}
}

// --- SetCustomerActive ---

// (5) Alasan kurang dari 10 karakter ditolak SEBELUM apa pun tersentuh.
func TestCustomerAdminService_SetCustomerActive_ReasonTooShort_Rejected(t *testing.T) {
	users := &fakeCustomerAdminUserStore{}
	svc := newTestCustomerAdminService(users, nil)

	_, err := svc.SetCustomerActive(context.Background(), SetActiveInput{
		CustomerID: uuid.New(), Active: false, Reason: "pendek", ActorID: uuid.New(),
	})
	if !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("expected ErrReasonRequired, got %v", err)
	}
	if users.setActiveCalls != 0 {
		t.Fatalf("expected SetCustomerActive NOT called when reason too short, got %d calls", users.setActiveCalls)
	}
}

// (6) Happy path: menonaktifkan pelanggan aktif menulis TEPAT satu baris
// customer_admin_logs beraksi "block" dengan ChangedBy=actor.
func TestCustomerAdminService_SetCustomerActive_HappyPath_WritesAuditLog(t *testing.T) {
	custID := uuid.New()
	users := &fakeCustomerAdminUserStore{byID: map[uuid.UUID]*model.User{
		custID: {ID: custID, Name: "Budi", UserType: model.UserTypeCustomer, CustomerType: registeredCustomerType(), IsActive: true},
	}}
	logs := &fakeCustomerAdminLogStore{}
	svc := newTestCustomerAdminService(users, logs)

	actorID := uuid.New()
	out, err := svc.SetCustomerActive(context.Background(), SetActiveInput{
		CustomerID: custID, Active: false, Reason: "Penyalahgunaan berulang oleh pelanggan", ActorID: actorID,
	})
	if err != nil {
		t.Fatalf("SetCustomerActive: unexpected err: %v", err)
	}
	if out.IsActive {
		t.Fatal("expected is_active=false in response")
	}
	if logs.createCalls != 1 {
		t.Fatalf("expected exactly 1 audit log row, got %d", logs.createCalls)
	}
	if logs.lastLog.Action != model.CustomerAdminActionBlock {
		t.Fatalf("expected logged action %q, got %q", model.CustomerAdminActionBlock, logs.lastLog.Action)
	}
	if logs.lastLog.ChangedBy == nil || *logs.lastLog.ChangedBy != actorID {
		t.Fatalf("expected logged ChangedBy=%s, got %v", actorID, logs.lastLog.ChangedBy)
	}
}

// (7) Mengaktifkan pelanggan yang SUDAH aktif ditolak sebagai no-op — audit
// trail tidak boleh terisi baris palsu.
func TestCustomerAdminService_SetCustomerActive_NoOp_Rejected(t *testing.T) {
	custID := uuid.New()
	users := &fakeCustomerAdminUserStore{byID: map[uuid.UUID]*model.User{
		custID: {ID: custID, Name: "Budi", UserType: model.UserTypeCustomer, CustomerType: registeredCustomerType(), IsActive: true},
	}}
	logs := &fakeCustomerAdminLogStore{}
	svc := newTestCustomerAdminService(users, logs)

	_, err := svc.SetCustomerActive(context.Background(), SetActiveInput{
		CustomerID: custID, Active: true, Reason: "Mengaktifkan yang sudah aktif", ActorID: uuid.New(),
	})
	if !errors.Is(err, ErrCustomerStatusUnchanged) {
		t.Fatalf("expected ErrCustomerStatusUnchanged, got %v", err)
	}
	if logs.createCalls != 0 {
		t.Fatalf("expected NO audit log row written for a no-op, got %d", logs.createCalls)
	}
}

// --- ExportCustomers ---

// (8) Hasil filter melebihi maxExportRows ditolak SEBELUM satu byte pun
// ditulis ke writer (§28.5 pola yang sama: batas ditegakkan sebelum baris
// pertama).
func TestCustomerAdminService_ExportCustomers_TooManyRows_Rejected(t *testing.T) {
	users := &fakeCustomerAdminUserStore{listTotal: maxExportRows + 1}
	svc := newTestCustomerAdminService(users, nil)

	var buf bytes.Buffer
	err := svc.ExportCustomers(context.Background(), ListCustomerInput{}, &buf)
	if !errors.Is(err, ErrExportTooLarge) {
		t.Fatalf("expected ErrExportTooLarge, got %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected nothing written before rejecting an oversized export, got %d bytes", buf.Len())
	}
}

// (9) Happy path: filter valid, hasil di bawah batas → header + baris data
// mengalir ke writer.
func TestCustomerAdminService_ExportCustomers_HappyPath_WritesHeaderAndRows(t *testing.T) {
	phoneStr := "6281234567890"
	users := &fakeCustomerAdminUserStore{
		listTotal: 1,
		streamBatches: [][]model.User{
			{{
				Name: "Budi Santoso", Phone: &phoneStr, UserType: model.UserTypeCustomer,
				CustomerType: registeredCustomerType(), IsActive: true, MembershipStatus: model.MembershipStatusActive,
			}},
		},
	}
	svc := newTestCustomerAdminService(users, nil)

	var buf bytes.Buffer
	if err := svc.ExportCustomers(context.Background(), ListCustomerInput{}, &buf); err != nil {
		t.Fatalf("ExportCustomers: unexpected err: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Nama") {
		t.Fatalf("expected CSV header row, got: %q", out)
	}
	if !strings.Contains(out, "Budi Santoso") {
		t.Fatalf("expected exported customer row, got: %q", out)
	}
}
