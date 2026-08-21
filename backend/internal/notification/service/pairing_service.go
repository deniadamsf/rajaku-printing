package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rajaku-printing/backend/internal/notification/workerclient"
)

// WorkerClient — narrowed contract this service actually calls. Declared as
// an interface (rather than depending on *workerclient.Client directly) so
// tests can inject a fake — *workerclient.Client satisfies this implicitly.
type WorkerClient interface {
	PairingStatus(ctx context.Context) (*workerclient.PairingStatus, error)
	Logout(ctx context.Context) error
}

// PairingStatusView — hasil GetPairingStatus. Reachable=false berarti worker
// tidak bisa dihubungi (atau menolak kredensial internal kita) — field
// lainnya kosong/nil dalam kondisi itu. Dipisah dari
// workerclient.PairingStatus supaya kontrak field JSON admin panel
// (worker_reachable/connected) tidak terikat langsung ke bentuk balasan
// worker.
type PairingStatusView struct {
	Reachable  bool
	Connected  bool
	SelfNumber *string
	QRDataURL  *string
	Quota      json.RawMessage
	Circuit    json.RawMessage
	Pacing     json.RawMessage
}

// PairingService — orkestrasi pairing WhatsApp (QR) untuk admin panel.
// Terpisah dari Service (job dispatch) karena tanggung jawabnya beda (§22
// satu struct satu tanggung jawab): Service urus enqueue/klaim job,
// PairingService urus komunikasi langsung ke notification-worker untuk
// status/putus pairing.
type PairingService struct {
	worker WorkerClient
}

func NewPairingService(worker WorkerClient) *PairingService {
	return &PairingService{worker: worker}
}

// GetPairingStatus SELALU mengembalikan PairingStatusView non-nil, bahkan
// kalau worker tidak bisa dihubungi (Reachable=false). Error return value
// hanya untuk keperluan LOGGING sisi pemanggil (handler) — bukan untuk
// menentukan status HTTP; kontrak endpoint admin selalu 200 di kasus ini
// (operator perlu membedakan "worker mati" dari "halaman rusak").
func (s *PairingService) GetPairingStatus(ctx context.Context) (PairingStatusView, error) {
	st, err := s.worker.PairingStatus(ctx)
	if err != nil {
		return PairingStatusView{Reachable: false}, fmt.Errorf("ambil status pairing whatsapp: %w", err)
	}
	return PairingStatusView{
		Reachable:  true,
		Connected:  st.WAReady,
		SelfNumber: st.SelfNumber,
		QRDataURL:  st.QRDataURL,
		Quota:      st.Quota,
		Circuit:    st.Circuit,
		Pacing:     st.Pacing,
	}, nil
}

// Unlink memutus pairing whatsapp. Berbeda dari GetPairingStatus: kalau
// worker tidak bisa dihubungi, ini WAJIB gagal dengan error (bukan sukses
// palsu) — operator perlu tahu pemutusan tidak benar-benar terjadi.
func (s *PairingService) Unlink(ctx context.Context) error {
	if err := s.worker.Logout(ctx); err != nil {
		return fmt.Errorf("putus pairing whatsapp: %w", err)
	}
	return nil
}
