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

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	authhandler "github.com/rajaku-printing/backend/internal/auth/handler"
	"github.com/rajaku-printing/backend/internal/auth/oauth"
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
	"github.com/rajaku-printing/backend/internal/notification/workerclient"
	orderhandler "github.com/rajaku-printing/backend/internal/order/handler"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
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
	sitemediahandler "github.com/rajaku-printing/backend/internal/sitemedia/handler"
	sitemediarepo "github.com/rajaku-printing/backend/internal/sitemedia/repository"
	sitemediaservice "github.com/rajaku-printing/backend/internal/sitemedia/service"

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

	// --- Google OAuth login/registration ---
	// Route is ALWAYS mounted (even when disabled) so the frontend gets an
	// explicit 503 OAUTH_NOT_CONFIGURED instead of a bare 404 when
	// GOOGLE_OAUTH_* env vars are empty (dev without Google credentials).
	oauthCodeRepo := authrepo.NewOAuthLoginCodeRepository(d.DB)
	phoneVerificationRepo := authrepo.NewPhoneVerificationRepository(d.DB)
	googleClient := oauth.NewGoogleClient(
		d.Config.GoogleOAuth.ClientID, d.Config.GoogleOAuth.ClientSecret, d.Config.GoogleOAuth.RedirectURL)
	googleOAuthSvc := authservice.NewGoogleOAuthService(
		userRepo, oauthCodeRepo, phoneVerificationRepo, googleClient, jwtIssuer,
		authservice.OTPConfig{
			TTL:                   d.Config.OTP.TTL,
			MaxAttempts:           d.Config.OTP.MaxAttempts,
			ResendCooldown:        d.Config.OTP.ResendCooldown,
			CodeLength:            d.Config.OTP.CodeLength,
			MaxPerPhoneHour:       d.Config.OTP.MaxPerPhoneHour,
			MaxFailedPerPhoneHour: d.Config.OTP.MaxFailedPerPhoneHour,
		},
		d.DB,
	)
	// googleOAuthSvc.SetOTPSender(notifSvc) — wired further below, after
	// notifSvc is built (notification module wired after auth in this
	// composition root, same setter-injection pattern as SetNotifier
	// elsewhere in this file).
	googleOAuthH := authhandler.NewGoogleOAuthHandler(
		googleOAuthSvc, d.Config.GoogleOAuth.Enabled(), d.Config.App.FrontendURL, d.Config.App.Env != config.EnvDevelopment)
	if d.Config.GoogleOAuth.Enabled() {
		log.Info().Msg("Google OAuth aktif")
	} else {
		log.Info().Msg("Google OAuth nonaktif (kredensial kosong)")
	}

	// --- Phone claim: authenticated user adds/changes their own WA number ---
	// Same OTP policy/config as Google OAuth registration (§ business rule —
	// OTP only for contested numbers). otpSender wired below, same
	// setter-injection reason as googleOAuthSvc above.
	//
	// newOrderCustomerMerger: composition-root factory (§22 — only router.go
	// may import both auth and order internals) that binds a fresh
	// orderservice.CustomerMerger to whatever *gorm.DB transaction the phone
	// claim's absorption path is running inside, so a guest's order history
	// moves atomically with releasing its phone (§11). Doesn't depend on
	// orderSvc — built below out of order on purpose, no reordering needed.
	newOrderCustomerMerger := func(tx *gorm.DB) orderapi.CustomerMerger {
		return orderservice.NewCustomerMerger(tx)
	}
	phoneClaimSvc := authservice.NewPhoneClaimService(
		userRepo, phoneVerificationRepo,
		authservice.OTPConfig{
			TTL:                   d.Config.OTP.TTL,
			MaxAttempts:           d.Config.OTP.MaxAttempts,
			ResendCooldown:        d.Config.OTP.ResendCooldown,
			CodeLength:            d.Config.OTP.CodeLength,
			MaxPerPhoneHour:       d.Config.OTP.MaxPerPhoneHour,
			MaxFailedPerPhoneHour: d.Config.OTP.MaxFailedPerPhoneHour,
		},
		d.DB,
		newOrderCustomerMerger,
	)
	phoneClaimH := authhandler.NewPhoneClaimHandler(phoneClaimSvc)

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
	posSvc.SetSettingsReader(settingsSvc)
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
		BaseURL:             d.Config.App.FrontendURL,
		MaxAttempts:         d.Config.Notification.MaxAttempts,
		InitialBackoff:      d.Config.Notification.InitialBackoff,
		InternalAlertPhone:  d.Config.Notification.InternalPhone,
		SensitiveMessageTTL: d.Config.Notification.SensitiveMessageTTL,
	})
	orderSvc.SetNotifier(notifSvc)
	paymentSvc.SetNotifier(notifSvc)
	designSvc.SetNotifier(notifSvc)
	productionSvc.SetNotifier(notifSvc)
	invoiceSvc.SetNotifier(notifSvc)
	posSvc.SetNotifier(notifSvc)
	// Google OAuth registration OTP (WA number ownership proof) — best-effort
	// enqueue via the same job-queue mechanism as every other WA trigger.
	googleOAuthSvc.SetOTPSender(notifSvc)
	// Authenticated phone-claim OTP (same job-queue mechanism).
	phoneClaimSvc.SetOTPSender(notifSvc)
	// Alert internal (ke nomor ops, bukan customer) — dipakai reminder retensi.
	designSvc.SetInternalAlerter(notifSvc)
	notifH := notifhandler.New(notifSvc, d.Config.Notification.InternalSecret)

	// --- Wiring pairing WhatsApp admin panel (§13, permission notification.manage,
	// migration 000021) ---
	// workerclient adalah CALLER ke notification-worker (arah kebalikan dari
	// /internal/notifications/* di atas, di mana worker yang memanggil kita).
	// Timeout sengaja pendek — halaman admin tidak boleh menggantung kalau
	// worker mati.
	pairingWorkerClient := workerclient.New(
		d.Config.Notification.WorkerURL,
		d.Config.Notification.InternalSecret,
		5*time.Second,
	)
	pairingSvc := notifservice.NewPairingService(pairingWorkerClient)
	pairingH := notifhandler.NewPairingHandler(pairingSvc)

	// --- Wiring modul CMS (§14) ---
	// Share filestore (root sama, subdir cms_images).
	cmsRepo := cmsrepo.NewRepository(d.DB)
	cmsSvc := cmsservice.New(cmsRepo, fs, cmsservice.Config{
		MaxImageUploadMB: d.Config.CMS.MaxImageUploadMB,
	})
	cmsH := cmshandler.New(cmsSvc)

	// --- Wiring modul sitemedia ---
	// Gambar landing page yang bisa diganti admin panel tanpa deploy ulang
	// (bukan artikel CMS). Share filestore (root sama, subdir site_media).
	// URL file publik dibangun dari BaseURL (§2 satu sumber base URL).
	sitemediaRepo := sitemediarepo.NewRepository(d.DB)
	sitemediaSvc := sitemediaservice.New(sitemediaRepo, fs, sitemediaservice.Config{
		BaseURL: d.Config.App.BaseURL,
	})
	sitemediaH := sitemediahandler.New(sitemediaSvc)

	// v1 API group.
	v1 := r.Group("/api/v1")
	{
		// Modul auth (register/login publik + /me protected — handler yg atur).
		authH.RegisterRoutes(v1, authSvc)

		// Phone claim — POST /auth/phone/{request-otp,} — authenticated user
		// menambah/mengganti nomor WA sendiri (§ business rule, counterpart
		// dari GoogleOAuthHandler untuk registrasi). RequireAuth WAJIB (bukan
		// endpoint publik) DAN limiter OTP ketat (RATE_LIMIT_OTP_*) — endpoint
		// ini bisa mengungkap status keterpakaian nomor WA sembarang, sama
		// seperti POST /auth/google/request-otp.
		phoneClaimGroup := v1.Group("/auth/phone")
		phoneClaimGroup.Use(authapi.RequireAuth(authSvc))
		phoneClaimGroup.Use(middleware.RateLimitPublic(d.Config.RateLimit.OTPRPS, d.Config.RateLimit.OTPBurst))
		phoneClaimH.RegisterRoutes(phoneClaimGroup)

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

			// Google OAuth login/registration — /auth/google/{start,callback,
			// exchange} on the general public limiter, /request-otp and
			// /complete on their OWN much stricter limiter (RATE_LIMIT_OTP_*) —
			// both accept an arbitrary WhatsApp number / OTP guess in an
			// unauthenticated POST body, same brute-force shape as the
			// guestVerify group above.
			googleOTPLimited := public.Group("")
			googleOTPLimited.Use(middleware.RateLimitPublic(
				d.Config.RateLimit.OTPRPS, d.Config.RateLimit.OTPBurst))
			googleOAuthH.RegisterRoutes(public, googleOTPLimited)

			public.GET("/ping", func(c *gin.Context) {
				httpx.OK(c, gin.H{"pong": true})
			})

			// Modul CMS — public endpoints (list + detail slug + serve image).
			// Admin routes di-mount di v1 (bukan public) supaya rate-limit
			// publik tidak mengganggu admin CRUD.
			cmsH.RegisterRoutes(v1, public, authSvc)

			// Modul sitemedia — GET /site-media + /site-media/file/:slot
			// public (dipanggil landing SSR). Admin routes di-mount di v1.
			sitemediaH.RegisterRoutes(v1, public, authSvc)
		}

		// Modul payment — POST /orders/:resi/payment-proof, GET
		// /orders/:resi/payment-proofs, GET /payment-proofs/:id/file (semua
		// terima sesi penuh ATAU token guest_order ber-scope, lihat
		// payment/handler/routes.go) + admin group. Mount di v1 (bukan
		// public) supaya rate-limit publik tidak mengganggu multipart upload.
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

		// Admin panel — /admin/whatsapp/pairing + /admin/whatsapp/unlink.
		// Staff-only + permission notification.manage (§13, §ket keamanan:
		// QR pairing setara kredensial, JANGAN pernah dimount publik).
		pairingH.RegisterRoutes(v1, authSvc)

		// Modul settings — GET /payment-info publik (§7, rate limited via
		// `public` group declared above); sisanya admin (super-admin-only)
		// ubah kebijakan global mis. retensi file desain (§19).
		settingsH.RegisterRoutes(v1, public, authSvc)
	}

	bg, err := buildBackground(d.Config, designSvc, notifSvc)
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

// sensitiveMessageRedactor — kontrak minimal dari notification service untuk
// sweep job keamanan (review finding #3: redaksi paksa `message` job
// is_sensitive yang tidak kunjung sampai status terminal). Interface lokal,
// sama alasannya dengan retentionRunner di atas.
type sensitiveMessageRedactor interface {
	RedactStaleSensitiveMessages(ctx context.Context) (int64, error)
}

// buildBackground mendaftarkan job terjadwal. Spec cron divalidasi di sini —
// salah ketik = startup gagal (§22 fail-fast), bukan job diam-diam tidak jalan.
func buildBackground(cfg *config.Config, retention retentionRunner, notif sensitiveMessageRedactor) (*Background, error) {
	// Timeout dipakai bersama oleh SEMUA job terjadwal (bukan cuma retention
	// desain) — cukup satu Runner untuk seluruh proses, tidak perlu goroutine
	// scheduler terpisah per fitur.
	runner := jobs.New(cfg.Retention.JobTimeout)

	if cfg.Retention.Enabled {
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
	} else {
		log.Warn().Msg("retention job DISABLED lewat RETENTION_JOB_ENABLED — file desain tidak akan dibersihkan otomatis")
	}

	// Notification sensitive-message sweep — INDEPENDEN dari
	// RETENTION_JOB_ENABLED di atas: ini kebersihan keamanan (jangan simpan
	// kode OTP plaintext lebih lama dari perlu, §3 review keamanan), bukan
	// soal kapasitas disk. Spec cron hardcode (bukan env var) karena ini
	// jaring pengaman internal, bukan kebijakan bisnis yang perlu admin
	// atur — satu-satunya knob yang relevan adalah ambang umurnya
	// (NOTIFICATION_SENSITIVE_MESSAGE_TTL, dibaca di dalam service).
	if err := runner.Register(jobs.Job{
		Name: "notification-sensitive-message-sweep",
		Spec: "0 * * * *",
		Run: func(ctx context.Context) error {
			n, err := notif.RedactStaleSensitiveMessages(ctx)
			if err != nil {
				return err
			}
			log.Info().Int64("redacted", n).Msg("notification sensitive-message sweep selesai")
			return nil
		},
	}); err != nil {
		return nil, err
	}

	return &Background{Jobs: runner}, nil
}
