// Package jobs adalah scheduler tipis di atas robfig/cron untuk pekerjaan
// periodik in-process (§19 job cleanup retention).
//
// Kenapa in-process, bukan cron VPS yang panggil endpoint internal: satu
// deployment unit lebih sedikit yang bisa lupa dipasang saat pindah/redeploy
// server, dan job-nya butuh akses service layer yang sudah ter-wire (settings,
// filestore, notifikasi) — bukan sekadar HTTP call.
//
// Konsekuensi yang harus disadari: kalau nanti backend di-scale > 1 instance,
// tiap instance akan menjalankan job yang sama. Job retention sendiri idempoten
// (guard `is_purged = false` / `retention_reminder_at IS NULL` di query), jadi
// duplikasi tidak merusak data — tapi kalau nanti ada job yang tidak idempoten,
// wajib tambah locking (advisory lock Postgres / Redis, lihat catatan §13).
package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

// Job — satu pekerjaan terjadwal.
type Job struct {
	// Name dipakai di log; bikin deskriptif, ini yang dibaca saat debugging
	// "kenapa file desain nggak kehapus".
	Name string
	// Spec — cron expression 5 field (menit jam tanggal bulan hari),
	// mis. "0 3 * * *" = tiap hari jam 03:00 waktu server.
	Spec string
	// Run — eksekusi job. Menerima context yang sudah dibatasi Timeout runner.
	Run func(ctx context.Context) error
}

// Runner membungkus cron scheduler + timeout + recover per-eksekusi.
type Runner struct {
	cron    *cron.Cron
	timeout time.Duration

	mu      sync.Mutex
	started bool
}

// New membuat Runner. timeout <= 0 dianggap 15 menit — cukup longgar untuk
// batch retention, tapi tetap ada batas supaya job yang nyangkut (mis. disk
// hang) tidak menahan koneksi DB selamanya.
func New(timeout time.Duration) *Runner {
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	return &Runner{
		// SkipIfStillRunning: kalau run sebelumnya belum kelar (backlog besar),
		// jadwal berikutnya dilewati alih-alih menumpuk eksekusi paralel yang
		// saling rebutan row yang sama.
		cron:    cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DiscardLogger))),
		timeout: timeout,
	}
}

// Register menambah job. Spec cron divalidasi di sini supaya salah ketik di
// env var ketahuan saat startup (fail-fast §22), bukan saat job seharusnya
// jalan tengah malam.
func (r *Runner) Register(j Job) error {
	if j.Name == "" {
		return fmt.Errorf("jobs: name kosong")
	}
	if j.Run == nil {
		return fmt.Errorf("jobs: job %q tidak punya fungsi Run", j.Name)
	}
	if _, err := r.cron.AddFunc(j.Spec, r.wrap(j)); err != nil {
		return fmt.Errorf("jobs: register %q dengan spec %q: %w", j.Name, j.Spec, err)
	}
	return nil
}

// wrap memberi tiap eksekusi: context bertimeout, recover (satu job panic
// tidak boleh menjatuhkan proses API), dan log durasi.
func (r *Runner) wrap(j Job) func() {
	return func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error().
					Str("job", j.Name).
					Interface("panic", rec).
					Msg("scheduled job panic — recovered")
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
		defer cancel()

		start := time.Now()
		if err := j.Run(ctx); err != nil {
			log.Error().Err(err).
				Str("job", j.Name).
				Dur("duration", time.Since(start)).
				Msg("scheduled job failed")
			return
		}
		log.Info().
			Str("job", j.Name).
			Dur("duration", time.Since(start)).
			Msg("scheduled job done")
	}
}

// Start menjalankan scheduler di goroutine terpisah (non-blocking).
func (r *Runner) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return
	}
	r.cron.Start()
	r.started = true
	log.Info().Int("jobs", len(r.cron.Entries())).Msg("job scheduler started")
}

// Stop menghentikan penjadwalan dan menunggu job yang sedang jalan selesai,
// atau sampai ctx habis (mana yang duluan). Dipanggil saat graceful shutdown.
func (r *Runner) Stop(ctx context.Context) {
	r.mu.Lock()
	started := r.started
	r.started = false
	r.mu.Unlock()
	if !started {
		return
	}

	done := r.cron.Stop().Done()
	select {
	case <-done:
		log.Info().Msg("job scheduler stopped cleanly")
	case <-ctx.Done():
		log.Warn().Msg("job scheduler stop timed out — ada job yang masih jalan")
	}
}
