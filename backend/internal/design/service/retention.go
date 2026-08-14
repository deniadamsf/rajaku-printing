package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/pkg/filestore"
	"github.com/rajaku-printing/backend/internal/settings/settingsapi"
)

// Retensi file desain (§19).
//
// Kebijakan: blob file desain dihapus dari disk N hari setelah upload (default
// 30, configurable dari admin panel lewat setting `design_retention_days`).
// Row DB TIDAK ikut terhapus — nama file asli, ukuran, tanggal upload, order,
// dan siapa yang upload tetap tersimpan untuk rekap; yang dikosongkan cuma
// file_path + flag is_purged/purged_at.
//
// Kenapa ini penting: storage-nya disk lokal VPS tunggal (§19), tidak ada
// auto-scaling. Tanpa sweep, disk penuh diam-diam dan SEMUA upload (termasuk
// bukti bayar) mulai gagal.
//
// Dua job yang saling melengkapi, dijalankan scheduler (internal/pkg/jobs):
//
//	RunRetentionReminder — H-3 sebelum purge, kirim WA internal ke ops kalau
//	                       order terkait belum selesai (staff sempat download
//	                       manual untuk reprint).
//	RunRetentionSweep    — hapus blob yang sudah lewat masa retensi.
//
// Keduanya idempoten & batched: aman dijalankan berkali-kali, dan satu run
// dibatasi BatchLimit row supaya tidak menahan DB lama saat ada backlog.

// Status order yang dianggap "tidak perlu diingatkan lagi" — kerjaan sudah
// tuntas atau dibatalkan, jadi kalaupun filenya hilang tidak ada reprint.
//
// Sengaja string literal, bukan import internal/order/state: modul design
// tidak boleh import internal package modul order (§22). Konvensi yang sama
// sudah dipakai guard status di design_service.go.
const (
	orderStatusSelesai    = "selesai"
	orderStatusDibatalkan = "dibatalkan"
)

// SweepResult — ringkasan satu run sweep, dipakai untuk logging & monitoring
// (§19 "pastikan job ini benar-benar jalan reliable").
type SweepResult struct {
	RetentionDays int   `json:"retention_days"`
	Scanned       int   `json:"scanned"`
	Purged        int   `json:"purged"`
	Failed        int   `json:"failed"`
	FreedBytes    int64 `json:"freed_bytes"`
}

// ReminderResult — ringkasan satu run reminder H-3.
type ReminderResult struct {
	RetentionDays int `json:"retention_days"`
	ReminderDays  int `json:"reminder_days"`
	Scanned       int `json:"scanned"`
	Notified      int `json:"notified"`
	Skipped       int `json:"skipped"` // order sudah selesai/dibatalkan
	Failed        int `json:"failed"`
}

// SetSettingsReader meng-inject sumber setting retensi. Dipanggil composition
// root (server.NewRouter) setelah modul settings dibuat. Kalau nil, service
// pakai RetentionDefaultDays dari env.
func (s *Service) SetSettingsReader(r settingsapi.Reader) { s.settings = r }

// SetInternalAlerter meng-inject channel notifikasi internal (ops), terpisah
// dari SetNotifier yang tujuannya customer. Kalau nil, reminder H-3 dilewati.
func (s *Service) SetInternalAlerter(a notificationapi.InternalAlerter) { s.alerter = a }

// resolveRetentionDays membaca setting dari DB; fallback ke nilai env kalau
// setting hilang/rusak. Fallback SELALU disertai log warn — row yang hilang
// artinya seed migration bermasalah dan harus kelihatan di log, bukan lewat
// begitu saja (§22 "jangan gagal diam-diam").
func (s *Service) resolveRetentionDays(ctx context.Context) int {
	fallback := s.retentionDefaultDays
	if fallback <= 0 {
		fallback = 30
	}
	if s.settings == nil {
		return fallback
	}
	days, err := s.settings.GetInt(ctx, settingsapi.KeyDesignRetentionDays)
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).
			Int("fallback_days", fallback).
			Msg("retention: gagal baca setting design_retention_days, pakai fallback env")
		return fallback
	}
	if days <= 0 {
		log.Ctx(ctx).Warn().
			Int("value", days).
			Int("fallback_days", fallback).
			Msg("retention: setting design_retention_days <= 0, pakai fallback env")
		return fallback
	}
	return days
}

func (s *Service) batchLimit() int {
	if s.retentionBatchLimit > 0 {
		return s.retentionBatchLimit
	}
	return 200
}

func (s *Service) reminderDays() int {
	if s.retentionReminderDays > 0 {
		return s.retentionReminderDays
	}
	return 3
}

// RunRetentionSweep menghapus blob file desain yang sudah lewat masa retensi.
//
// Error handling per file sengaja "log & lanjut": satu file bermasalah (mis.
// permission disk) tidak boleh menghentikan pembersihan file lain. Error yang
// dikembalikan hanya untuk kegagalan level-batch (query kandidat gagal).
func (s *Service) RunRetentionSweep(ctx context.Context) (SweepResult, error) {
	days := s.resolveRetentionDays(ctx)
	res := SweepResult{RetentionDays: days}

	now := s.nowFn().UTC()
	cutoff := now.AddDate(0, 0, -days)

	files, err := s.files.ListPurgeCandidates(ctx, cutoff, s.batchLimit())
	if err != nil {
		return res, fmt.Errorf("retention sweep: list candidates: %w", err)
	}
	res.Scanned = len(files)

	for i := range files {
		f := files[i]
		subpath := strings.TrimSpace(f.FilePath)

		// file_path kosong = row lama yang blob-nya sudah hilang entah bagaimana.
		// Tidak ada yang perlu dihapus, tapi flag-nya tetap harus dirapikan
		// supaya tidak ikut ke-scan lagi tiap run.
		if subpath != "" {
			if err := s.blobs.Delete(ctx, subpath); err != nil && !errors.Is(err, filestore.ErrNotFound) {
				res.Failed++
				log.Ctx(ctx).Error().Err(err).
					Str("file_id", f.ID.String()).
					Str("subpath", subpath).
					Msg("retention sweep: gagal hapus blob, row tidak ditandai purged (akan dicoba lagi)")
				continue
			}
		}

		if err := s.files.MarkPurged(ctx, f.ID, now); err != nil {
			res.Failed++
			log.Ctx(ctx).Error().Err(err).
				Str("file_id", f.ID.String()).
				Msg("retention sweep: blob terhapus tapi gagal update row — file jadi yatim di DB")
			continue
		}

		res.Purged++
		res.FreedBytes += f.FileSizeBytes
	}

	return res, nil
}

// RunRetentionReminder mengirim peringatan H-N ke nomor ops untuk file yang
// sebentar lagi dihapus, TAPI order-nya belum selesai — kasus di mana reprint
// masih mungkin dibutuhkan (§19).
//
// File yang order-nya sudah `selesai`/`dibatalkan` sengaja TIDAK ditandai
// sebagai "sudah diingatkan": kalau nanti order dibuka lagi (mis. komplain),
// run berikutnya masih bisa mengingatkan selama file belum kehapus.
func (s *Service) RunRetentionReminder(ctx context.Context) (ReminderResult, error) {
	days := s.resolveRetentionDays(ctx)
	remind := s.reminderDays()
	res := ReminderResult{RetentionDays: days, ReminderDays: remind}

	if s.alerter == nil {
		log.Ctx(ctx).Debug().Msg("retention reminder: internal alerter tidak di-set, dilewati")
		return res, nil
	}

	now := s.nowFn().UTC()
	purgeCutoff := now.AddDate(0, 0, -days)               // lebih tua dari ini → urusan sweep
	reminderCutoff := now.AddDate(0, 0, -(days - remind)) // lebih tua dari ini → masuk jendela H-N

	files, err := s.files.ListReminderCandidates(ctx, purgeCutoff, reminderCutoff, s.batchLimit())
	if err != nil {
		return res, fmt.Errorf("retention reminder: list candidates: %w", err)
	}
	res.Scanned = len(files)

	for i := range files {
		f := files[i]

		order, err := s.orderCmd.FindSummaryByID(ctx, f.OrderID)
		if err != nil {
			res.Failed++
			log.Ctx(ctx).Error().Err(err).
				Str("file_id", f.ID.String()).
				Str("order_id", f.OrderID.String()).
				Msg("retention reminder: gagal resolve order")
			continue
		}
		if order.Status == orderStatusSelesai || order.Status == orderStatusDibatalkan {
			res.Skipped++
			continue
		}

		// Dibulatkan ke ATAS: sisa 1,9 hari lebih jujur ditulis "2 hari"
		// daripada "1 hari" (truncate) yang bikin staff kira waktunya lebih
		// mepet dari kenyataan.
		purgeAt := f.UploadedAt.AddDate(0, 0, days)
		daysLeft := int(math.Ceil(purgeAt.Sub(now).Hours() / 24))
		if daysLeft < 0 {
			daysLeft = 0
		}

		dedup := fmt.Sprintf("%s:%s", notificationapi.KindDesignRetentionWarning, f.ID)
		err = s.alerter.EnqueueInternalAlert(ctx,
			notificationapi.KindDesignRetentionWarning,
			&f.OrderID,
			map[string]any{
				"days_left":      daysLeft,
				"retention_days": days,
				"file_name":      f.FileOriginalName,
				"file_id":        f.ID.String(),
				"order_status":   order.Status,
				"purge_at":       purgeAt.Format(time.RFC3339),
			},
			dedup,
		)
		if err != nil {
			// Nomor ops belum dikonfigurasi → sisa batch pasti gagal dengan
			// alasan yang sama. Berhenti, jangan spam log per file.
			if errors.Is(err, notificationapi.ErrInternalRecipientMissing) {
				log.Ctx(ctx).Warn().
					Msg("retention reminder: NOTIFICATION_INTERNAL_PHONE belum di-set, reminder dilewati")
				return res, nil
			}
			res.Failed++
			log.Ctx(ctx).Error().Err(err).
				Str("file_id", f.ID.String()).
				Msg("retention reminder: gagal enqueue alert internal")
			continue
		}

		if err := s.files.MarkReminderSent(ctx, f.ID, now); err != nil {
			// Alert sudah terkirim; gagal menandai berarti run berikutnya bisa
			// kirim ulang. Dedup key di notification jadi jaring pengaman
			// (insert kedua jadi no-op), jadi tidak fatal — tapi tetap dicatat.
			res.Failed++
			log.Ctx(ctx).Error().Err(err).
				Str("file_id", f.ID.String()).
				Msg("retention reminder: alert terkirim tapi gagal tandai reminder_at")
			continue
		}

		res.Notified++
	}

	return res, nil
}
