// Package server wires the HTTP router. Module routes will be registered here
// as they are built (auth, catalog, order, ...). Keep this file thin — the
// per-module route registration lives inside each module's handler package.
package server

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	authhandler "github.com/rajaku-printing/backend/internal/auth/handler"
	authrepo "github.com/rajaku-printing/backend/internal/auth/repository"
	authservice "github.com/rajaku-printing/backend/internal/auth/service"
	"github.com/rajaku-printing/backend/internal/auth/token"
	cataloghandler "github.com/rajaku-printing/backend/internal/catalog/handler"
	catalogrepo "github.com/rajaku-printing/backend/internal/catalog/repository"
	catalogservice "github.com/rajaku-printing/backend/internal/catalog/service"
	cmshandler "github.com/rajaku-printing/backend/internal/cms/handler"
	cmsrepo "github.com/rajaku-printing/backend/internal/cms/repository"
	cmsservice "github.com/rajaku-printing/backend/internal/cms/service"
	"github.com/rajaku-printing/backend/internal/config"
	designhandler "github.com/rajaku-printing/backend/internal/design/handler"
	designrepo "github.com/rajaku-printing/backend/internal/design/repository"
	designservice "github.com/rajaku-printing/backend/internal/design/service"
	invoicehandler "github.com/rajaku-printing/backend/internal/invoice/handler"
	"github.com/rajaku-printing/backend/internal/invoice/pdfrender"
	invoicerepo "github.com/rajaku-printing/backend/internal/invoice/repository"
	invoiceservice "github.com/rajaku-printing/backend/internal/invoice/service"
	notifhandler "github.com/rajaku-printing/backend/internal/notification/handler"
	notifrepo "github.com/rajaku-printing/backend/internal/notification/repository"
	notifservice "github.com/rajaku-printing/backend/internal/notification/service"
	orderhandler "github.com/rajaku-printing/backend/internal/order/handler"
	orderrepo "github.com/rajaku-printing/backend/internal/order/repository"
	orderservice "github.com/rajaku-printing/backend/internal/order/service"
	paymenthandler "github.com/rajaku-printing/backend/internal/payment/handler"
	paymentrepo "github.com/rajaku-printing/backend/internal/payment/repository"
	paymentservice "github.com/rajaku-printing/backend/internal/payment/service"
	poshandler "github.com/rajaku-printing/backend/internal/pos/handler"
	posservice "github.com/rajaku-printing/backend/internal/pos/service"
	productionhandler "github.com/rajaku-printing/backend/internal/production/handler"
	productionservice "github.com/rajaku-printing/backend/internal/production/service"
	settingshandler "github.com/rajaku-printing/backend/internal/settings/handler"
	settingsrepo "github.com/rajaku-printing/backend/internal/settings/repository"
	settingsservice "github.com/rajaku-printing/backend/internal/settings/service"

	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/middleware"
	"github.com/rajaku-printing/backend/internal/pkg/filestore"
	"github.com/rajaku-printing/backend/internal/pkg/jobs"
)

type Deps struct {
	Config *config.Config
	DB     *gorm.DB
}

// Background membungkus pekerjaan terjadwal yang hidup selama proses jalan
// (§19 retention). Dikembalikan terpisah dari router supaya main.go bisa
// mengatur lifecycle-nya (Start setelah server siap, Stop saat shutdown).
// Nil-safe: kalau job dimatikan lewat config, field-nya nil.
type Background struct {
	Jobs *jobs.Runner
}

func NewRouter(d Deps) (*gin.Engine, *Background, error) {
	if d.Config.App.Env == config.EnvProduction {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// SetTrustedProxies — security review finding: gin.New()'s DEFAULT trusts
	// every proxy, so without this call, Context.ClientIP() (used by every
	// RateLimitPublic-guarded route, mis. the guest-checkout verify endpoint
	// below) would read whatever X-Forwarded-For the caller sends. An
	// attacker can rotate that header per-request to get a fresh limiter
	// bucket every time, making the rate limit meaningless. Empty config
	// (dev default) → SetTrustedProxies(nil) → ClientIP() ignores
	// X-Forwarded-For entirely and uses the raw TCP RemoteAddr. Only
	// non-empty when TRUSTED_PROXIES names a specific, known reverse proxy.
	var trustedProxies []string
	if len(d.Config.TrustedProxies) > 0 {
		trustedProxies = d.Config.TrustedProxies
	}
	if err := r.SetTrustedProxies(trustedProxies); err != nil {
		return nil, nil, fmt.Errorf("set trusted proxies: %w", err)
	}

	r.Use(middleware.Recover())
	r.Use(middleware.RequestID())
	r.Use(middleware.CORS(d.Config.CORS.AllowedOrigins))

	// Health check — no auth, no rate limit (for container/orchestrator probes).
	r.GET("/healthz", func(c *gin.Context) {
		httpx.OK(c, gin.H{
			"status": "ok",
			"time":   time.Now().UTC().Format(time.RFC3339),
			"env":    string(d.Config.App.Env),
		})
	})

	// --- Wiring modul auth ---
	// Semua modul di-wire di sini (composition root). Handler menerima service,
	// service menerima repository — layer separation section 22.
	jwtIssuer, err := token.NewIssuer(d.Config.JWT.Secret, d.Config.JWT.AccessTTL, d.Config.JWT.Issuer)
	if err != nil {
		return nil, nil, err
	}
	// guestOrderIssuer — same secret/issuer name as jwtIssuer (so authSvc.VerifyToken
	// can verify tokens it signs) but a much shorter TTL. Deliberately a SEPARATE
	// *token.Issuer instance — never reuse jwtIssuer's 24h TTL for guest tokens.
	guestOrderIssuer, err := token.NewIssuer(d.Config.JWT.Secret, d.Config.JWT.GuestOrderTTL, d.Config.JWT.Issuer)
	if err != nil {
		return nil, nil, err
	}
	userRepo := authrepo.NewUserRepository(d.DB)
	roleRepo := authrepo.NewRoleRepository(d.DB)
	inviteRepo := authrepo.NewStaffInviteRepository(d.DB)
	authSvc := authservice.New(userRepo, roleRepo, jwtIssuer)
	authH := authhandler.New(authSvc)
	// authapi.CustomerService — dipakai order module.
	customerSvc := authservice.NewCustomerService(userRepo)
	// Guest-checkout ownership proof (POST /lacak/:resi/verify). orderCmd
	// wired below via SetOrderCommandService — order module is built AFTER
	// auth (order needs authapi.CustomerService first), same setter-injection
	// pattern as SetNotifier elsewhere in this file.
	guestOrderSvc := authservice.NewGuestOrderService(userRepo, guestOrderIssuer)
	guestOrderH := authhandler.NewGuestOrderHandler(guestOrderSvc)

	// --- Admin panel: staff & role management (§10) ---
	inviteSvc := authservice.NewInviteService(inviteRepo, userRepo, authservice.InviteConfig{})
	// Invite URL yg di-forward ke calon staff HARUS mengarah ke halaman Nuxt
	// (frontend), bukan API backend — di dev split-domain URL-nya beda port.
	staffAdminSvc := authservice.NewStaffAdminService(userRepo, roleRepo, inviteSvc, d.Config.App.FrontendURL)
	roleAdminSvc := authservice.NewRoleAdminService(roleRepo)
	adminH := authhandler.NewAdminHandler(staffAdminSvc, roleAdminSvc, inviteSvc)

	// --- Wiring modul catalog ---
	catalogProductRepo := catalogrepo.NewProductRepository(d.DB)
	catalogMaterialRepo := catalogrepo.NewMaterialRepository(d.DB)
	catalogPricingRepo := catalogrepo.NewPricingRepository(d.DB)
	catalogSvc := catalogservice.New(catalogProductRepo, catalogMaterialRepo, catalogPricingRepo)
	catalogH := cataloghandler.New(catalogSvc)

	// --- Wiring modul order ---
	orderRepo := orderrepo.NewOrderRepository(d.DB)
	orderSvc := orderservice.New(orderRepo, catalogSvc, customerSvc)
	orderH := orderhandler.New(orderSvc)
	// orderSvc satisfies orderapi.OrderCommandService — needed by guest order
	// verification (resi → order → CustomerID).
	guestOrderSvc.SetOrderCommandService(orderSvc)

	// --- Wiring modul payment ---
	// filestore backing customer-uploaded proofs; disk local per spec §19.
	fs := filestore.New(d.Config.Storage.LocalRoot)
	paymentProofRepo := paymentrepo.NewProofRepository(d.DB)
	// orderSvc satisfies orderapi.OrderCommandService — payment triggers order
	// state transitions via that interface (spec §22: no cross-module internal imports).
	paymentSvc := paymentservice.New(paymentProofRepo, fs, orderSvc, paymentservice.Config{
		MaxUploadMB: d.Config.Payment.MaxUploadMB,
	})
	paymentH := paymenthandler.New(paymentSvc)

	// --- Wiring modul design ---
	// Share filestore instance dgn payment (root sama, subdir beda per modul).
	designFilesRepo := designrepo.NewRepository(d.DB)
	designSvc := designservice.New(designFilesRepo, fs, orderSvc, designservice.Config{
		MaxUploadMB:           d.Config.Design.MaxUploadMB,
		RetentionDefaultDays:  d.Config.Retention.DefaultDays,
		RetentionReminderDays: d.Config.Retention.ReminderDays,
		RetentionBatchLimit:   d.Config.Retention.BatchLimit,
	})
	designH := designhandler.New(designSvc)

	// --- Wiring modul settings (§19) ---
	// Sumber masa retensi runtime. Di-inject ke design service supaya job
	// retention baca nilai terbaru tiap run (admin ubah setting → langsung
	// berlaku di run berikutnya, tanpa restart).
	settingsRepo := settingsrepo.NewRepository(d.DB)
	settingsSvc := settingsservice.New(settingsRepo)
	settingsH := settingshandler.New(settingsSvc)
	designSvc.SetSettingsReader(settingsSvc)

	// --- Wiring modul production ---
	// Thin — hanya orkestrasi state advance via orderCmd + trigger notif.
	// Tidak punya storage sendiri.
	productionSvc := productionservice.New(orderSvc)
	productionH := productionhandler.New(productionSvc)

	// --- Wiring modul invoice (§12) ---
	invoiceRepo := invoicerepo.NewRepository(d.DB)
	invoiceSvc := invoiceservice.New(invoiceRepo, fs, orderSvc, customerSvc, invoiceservice.Config{
		BaseURL: d.Config.App.BaseURL,
		Company: pdfrender.CompanyInfo{
			Name:    d.Config.Invoice.CompanyName,
			Address: d.Config.Invoice.CompanyAddress,
			Phone:   d.Config.Invoice.CompanyPhone,
			Email:   d.Config.Invoice.CompanyEmail,
			Website: d.Config.App.BaseURL,
		},
	})
	invoiceH := invoicehandler.New(invoiceSvc)
	// Payment auto-trigger invoice generation after approval.
	paymentSvc.SetInvoiceGenerator(invoiceSvc)

	// --- Wiring modul POS (§11) ---
	// Thin — orchestrator: resolve customer → orderapi.CreatePOSOrder → notif + invoice.
	// Tracking URL POS ("/lacak/:resi") diakses customer via browser → pakai
	// FrontendURL supaya di dev-split-domain tetap valid.
	posSvc := posservice.New(orderSvc, customerSvc, posservice.Config{
		BaseURL: d.Config.App.FrontendURL,
	})
	posSvc.SetInvoiceGenerator(invoiceSvc)
	posH := poshandler.New(posSvc)

	// --- Wiring modul notification ---
	// notification.Service butuh orderCmd (=orderSvc) + customerSvc. Ini yang
	// mendorong urutan wiring: order dulu, lalu notification. Setelah dibuat,
	// notif di-inject BALIK ke order.Service, payment.Service, design.Service,
	// dan production.Service lewat setter — best-effort enqueue.
	notifRepo := notifrepo.NewRepository(d.DB)
	// Tracking URL di template WA (mis. "Lacak di: BASE/lacak/:resi") →
	// FrontendURL karena URL dibuka pelanggan lewat browser.
	notifSvc := notifservice.New(notifRepo, orderSvc, customerSvc, notifservice.Config{
		BaseURL:            d.Config.App.FrontendURL,
		MaxAttempts:        d.Config.Notification.MaxAttempts,
		InitialBackoff:     d.Config.Notification.InitialBackoff,
		InternalAlertPhone: d.Config.Notification.InternalPhone,
	})
	orderSvc.SetNotifier(notifSvc)
	paymentSvc.SetNotifier(notifSvc)
	designSvc.SetNotifier(notifSvc)
	productionSvc.SetNotifier(notifSvc)
	invoiceSvc.SetNotifier(notifSvc)
	posSvc.SetNotifier(notifSvc)
	// Alert internal (ke nomor ops, bukan customer) — dipakai reminder retensi.
	designSvc.SetInternalAlerter(notifSvc)
	notifH := notifhandler.New(notifSvc, d.Config.Notification.InternalSecret)

	// --- Wiring modul CMS (§14) ---
	// Share filestore (root sama, subdir cms_images).
	cmsRepo := cmsrepo.NewRepository(d.DB)
	cmsSvc := cmsservice.New(cmsRepo, fs, cmsservice.Config{
		MaxImageUploadMB: d.Config.CMS.MaxImageUploadMB,
	})
	cmsH := cmshandler.New(cmsSvc)

	// v1 API group.
	v1 := r.Group("/api/v1")
	{
		// Modul auth (register/login publik + /me protected — handler yg atur).
		authH.RegisterRoutes(v1, authSvc)
		// Admin: kelola staff, role, permissions + invite accept (§10).
		adminH.RegisterRoutes(v1, authSvc)
		// Admin catalog: kelola bahan/produk/pricing (§9/§10).
		catalogH.RegisterAdminRoutes(v1, authSvc)

		// Public endpoints (no auth) — rate limited to prevent scraping/bruteforce.
		public := v1.Group("")
		public.Use(middleware.RateLimitPublic(d.Config.RateLimit.PublicRPS, d.Config.RateLimit.PublicBurst))
		{
			// Modul catalog — semua endpoint publik (tidak butuh auth).
			catalogH.RegisterRoutes(public)

			// Modul order — POST /orders (auth opsional; guest allowed),
			// GET /lacak/:resi (public tracking tercensor). GET /orders/:resi
			// (auth) di-mount di dalam v1 group non-public.
			orderH.RegisterRoutes(v1, public, authSvc)

			// POST /lacak/:resi/verify — guest checkout ownership proof (resi +
			// no. WA → token guest_order, dipakai upload/approve desain tanpa
			// akun). Endpoint milik modul auth (urusan sesi/identitas), tapi
			// path bersarang di bawah /lacak/:resi milik order module — aman,
			// gin memisah pohon routing per HTTP method (GET vs POST) sehingga
			// tidak konflik dengan GET /lacak/:resi di atas.
			//
			// Limiter TAMBAHAN (jauh lebih ketat) di atas limiter publik grup
			// yang sudah berlaku (public.Use di atas) — endpoint ini vektor
			// brute-force nomor WA (coba banyak nomor untuk satu resi).
			guestVerify := public.Group("/lacak/:resi")
			guestVerify.Use(middleware.RateLimitPublic(
				d.Config.RateLimit.GuestVerifyRPS, d.Config.RateLimit.GuestVerifyBurst))
			guestVerify.POST("/verify", guestOrderH.VerifyOwnership)

			public.GET("/ping", func(c *gin.Context) {
				httpx.OK(c, gin.H{"pong": true})
			})

			// Modul CMS — public endpoints (list + detail slug + serve image).
			// Admin routes di-mount di v1 (bukan public) supaya rate-limit
			// publik tidak mengganggu admin CRUD.
			cmsH.RegisterRoutes(v1, public, authSvc)
		}

		// Modul payment — POST /orders/:resi/payment-proof (auth req) + admin
		// group. Mount di v1 (bukan public) supaya rate-limit publik tidak
		// mengganggu multipart upload.
		paymentH.RegisterRoutes(v1, authSvc)

		// Modul design — upload/list customer files, staff verify/draft/walkin,
		// customer approve/revision. Mount di v1 (multipart uploads).
		designH.RegisterRoutes(v1, authSvc)

		// Modul production — semua staff-only, advance state cetak/QC/kirim/selesai.
		productionH.RegisterRoutes(v1, authSvc)

		// Modul invoice — public GET /invoices/:id/pdf + admin regenerate.
		invoiceH.RegisterRoutes(v1, authSvc)

		// Modul POS — semua staff-only. Kasir input order walk-in + rekonsiliasi harian.
		posH.RegisterRoutes(v1, authSvc)

		// Modul notification — /internal/notifications/* (worker-facing,
		// shared-secret auth). Bukan public: no rate-limit publik biar
		// worker bisa polling cepat.
		notifH.RegisterRoutes(v1)

		// Modul settings — super-admin-only, ubah kebijakan global mis.
		// retensi file desain (§19).
		settingsH.RegisterRoutes(v1, authSvc)
	}

	bg, err := buildBackground(d.Config, designSvc)
	if err != nil {
		return nil, nil, err
	}

	return r, bg, nil
}

// retentionRunner — kontrak minimal yang dibutuhkan scheduler dari design
// service. Interface lokal (bukan di designapi) karena consumer-nya cuma
// composition root ini, dan bikin unit test wiring gampang.
type retentionRunner interface {
	RunRetentionSweep(ctx context.Context) (designservice.SweepResult, error)
	RunRetentionReminder(ctx context.Context) (designservice.ReminderResult, error)
}

// buildBackground mendaftarkan job terjadwal. Spec cron divalidasi di sini —
// salah ketik = startup gagal (§22 fail-fast), bukan job diam-diam tidak jalan.
func buildBackground(cfg *config.Config, retention retentionRunner) (*Background, error) {
	if !cfg.Retention.Enabled {
		log.Warn().Msg("retention job DISABLED lewat RETENTION_JOB_ENABLED — file desain tidak akan dibersihkan otomatis")
		return &Background{}, nil
	}

	runner := jobs.New(cfg.Retention.JobTimeout)

	if err := runner.Register(jobs.Job{
		Name: "design-retention-reminder",
		Spec: cfg.Retention.ReminderCron,
		Run: func(ctx context.Context) error {
			res, err := retention.RunRetentionReminder(ctx)
			if err != nil {
				return err
			}
			log.Info().
				Int("scanned", res.Scanned).
				Int("notified", res.Notified).
				Int("skipped", res.Skipped).
				Int("failed", res.Failed).
				Int("retention_days", res.RetentionDays).
				Msg("retention reminder selesai")
			return nil
		},
	}); err != nil {
		return nil, err
	}

	if err := runner.Register(jobs.Job{
		Name: "design-retention-sweep",
		Spec: cfg.Retention.SweepCron,
		Run: func(ctx context.Context) error {
			res, err := retention.RunRetentionSweep(ctx)
			if err != nil {
				return err
			}
			log.Info().
				Int("scanned", res.Scanned).
				Int("purged", res.Purged).
				Int("failed", res.Failed).
				Int64("freed_bytes", res.FreedBytes).
				Int("retention_days", res.RetentionDays).
				Msg("retention sweep selesai")
			return nil
		},
	}); err != nil {
		return nil, err
	}

	return &Background{Jobs: runner}, nil
}
