// Package config loads and validates application configuration from environment
// variables exactly ONCE at startup (fail-fast). Downstream code receives an
// immutable *Config via dependency injection — no scattered os.Getenv calls.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

type Config struct {
	App          AppConfig
	DB           DBConfig
	CORS         CORSConfig
	RateLimit    RateLimitConfig
	JWT          JWTConfig
	GoogleOAuth  GoogleOAuthConfig
	Notification NotificationConfig
	Storage      StorageConfig
	Payment      PaymentConfig
	Design       DesignConfig
	Invoice      InvoiceConfig
	CMS          CMSConfig
	Retention    RetentionConfig
	OTP          OTPConfig
	// TrustedProxies — IP/CIDR entries gin.Engine.SetTrustedProxies() should
	// trust when reading X-Forwarded-For to compute Context.ClientIP() (used
	// by rate limiting). nil/empty means "trust nothing" — ClientIP() falls
	// back to the raw TCP RemoteAddr, which is the safe default when there's
	// no reverse proxy in front (mis. local dev). See router.go.
	TrustedProxies []string
}

// RetentionConfig — parameter job pembersihan file desain (§19).
//
// Catatan penting: masa retensi RUNTIME dibaca dari tabel app_settings
// (`design_retention_days`, editable super admin dari admin panel). DefaultDays
// di sini cuma fallback kalau row setting-nya hilang/rusak — jangan dianggap
// sumber kebenaran.
type RetentionConfig struct {
	// Enabled — matikan hanya untuk lingkungan yang tidak boleh menghapus apa
	// pun (mis. mesin dev yang di-restore dari dump produksi).
	Enabled bool
	// DefaultDays — fallback masa retensi (hari).
	DefaultDays int
	// ReminderDays — H-berapa reminder internal dikirim sebelum file dihapus.
	ReminderDays int
	// SweepCron / ReminderCron — jadwal cron 5 field, waktu server.
	SweepCron    string
	ReminderCron string
	// BatchLimit — maksimum file yang diproses per run.
	BatchLimit int
	// JobTimeout — batas durasi satu run job.
	JobTimeout time.Duration
}

type StorageConfig struct {
	// LocalRoot is the absolute or working-dir-relative path where uploaded
	// files (payment proofs, design files) are persisted. Must exist and be
	// writable at startup — validated fail-fast.
	LocalRoot string
}

type PaymentConfig struct {
	// MaxUploadMB caps individual proof file size (defense-in-depth on top of
	// the reverse-proxy body limit).
	MaxUploadMB int
}

type DesignConfig struct {
	// MaxUploadMB — batas file desain (CDR/AI bisa besar). Reverse proxy
	// juga sebaiknya cap ini di sisi ingress.
	MaxUploadMB int
}

// CMSConfig — batasi ukuran upload gambar artikel; service akan re-encode
// ke WebP (nativewebp, pure Go).
type CMSConfig struct {
	MaxImageUploadMB int
}

// InvoiceConfig — data toko yg tampil di header PDF invoice (§12).
// Sengaja tidak divalidasi ketat (semua boleh kosong = pakai fallback nama saja).
type InvoiceConfig struct {
	CompanyName    string
	CompanyAddress string
	CompanyPhone   string
	CompanyEmail   string
}

type AppConfig struct {
	Env  Environment
	Port int
	// BaseURL — API/backend URL. Dipakai untuk build download link resource
	// yang ditangani backend (mis. invoice PDF). Di produksi biasanya sama
	// dgn FrontendURL karena satu domain (§2).
	BaseURL string
	// FrontendURL — URL publik yang customer/staff akses via browser (halaman
	// Nuxt). Dipakai untuk build invite URL, public tracking URL, WA link
	// (§2 "SATU sumber"). Default = BaseURL kalau env var kosong (prod
	// behavior: single-domain reverse-proxy setup).
	FrontendURL string
	LogLevel    string
}

type DBConfig struct {
	Host         string
	Port         int
	User         string
	Password     string
	Name         string
	SSLMode      string
	MaxOpenConns int
	MaxIdleConns int
}

func (d DBConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

type CORSConfig struct {
	AllowedOrigins []string
}

type RateLimitConfig struct {
	PublicRPS   float64
	PublicBurst int
	// GuestVerifyRPS/GuestVerifyBurst — dedicated (stricter) limiter for
	// POST /lacak/:resi/verify. This endpoint lets a caller guess a WhatsApp
	// number against a known resi, so it MUST be throttled harder than the
	// regular public group (spec: brute-force vector).
	GuestVerifyRPS   float64
	GuestVerifyBurst int
	// OTPRPS/OTPBurst — dedicated (stricter) limiter for
	// POST /auth/google/request-otp and POST /auth/google/complete. Same
	// brute-force shape as guest-verify above: both endpoints accept an
	// arbitrary WhatsApp number from an unauthenticated POST body (request-otp
	// can be used to spam a number with codes; complete can be used to brute-
	// force a short numeric OTP), so they get their own tight bucket instead
	// of sharing the general public group.
	OTPRPS   float64
	OTPBurst int
}

// OTPConfig — parameter kode verifikasi WhatsApp untuk pendaftaran Google
// OAuth (nomor WA wajib dibuktikan kepemilikannya sebelum akun dibuat/
// di-upgrade dari guest — lihat internal/auth/service/google_oauth_service.go).
type OTPConfig struct {
	// TTL — masa berlaku satu kode OTP sejak dibuat.
	TTL time.Duration
	// MaxAttempts — batas percobaan salah sebelum kode ditolak permanen
	// (OTP_TOO_MANY_ATTEMPTS), harus minta kode baru.
	MaxAttempts int
	// ResendCooldown — jeda minimum sebelum boleh minta kode baru untuk
	// (handoff, phone) yang sama.
	ResendCooldown time.Duration
	// CodeLength — jumlah digit kode OTP (4-8).
	CodeLength int
}

type JWTConfig struct {
	Secret    string
	AccessTTL time.Duration
	Issuer    string
	// GuestOrderTTL — lifetime of the scope-limited token issued by
	// POST /lacak/:resi/verify. Deliberately short (default 30m, NOT
	// AccessTTL's 24h) — this token only needs to live long enough for a
	// guest to upload/approve a design file in one sitting.
	GuestOrderTTL time.Duration
}

type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// Enabled reports whether Google OAuth is fully configured. Load() rejects
// any PARTIAL configuration at startup (fail-fast, §22) — by the time a
// caller holds a *Config, GoogleOAuth is guaranteed to be either fully off
// (all three empty) or fully on (all three set, RedirectURL a valid absolute
// URL), so this is a simple non-empty check, not a validation.
func (g GoogleOAuthConfig) Enabled() bool {
	return g.ClientID != "" && g.ClientSecret != "" && g.RedirectURL != ""
}

type NotificationConfig struct {
	WorkerURL string
	Timeout   time.Duration
	// InternalSecret — shared secret yang dipakai worker (Node.js/Baileys)
	// untuk authenticate ke endpoint /internal/notifications/*. Wajib
	// >= 32 chars (§22 config fail-fast).
	InternalSecret string
	// MaxAttempts — batas retry per job sebelum status jadi `dead`.
	// Rate-limit-aware (Baileys mudah banned kalau spam).
	MaxAttempts int
	// InitialBackoff — retry pertama setelah gagal; berikutnya exponential.
	InitialBackoff time.Duration
	// InternalPhone — nomor WA ops/staff (62xxx) untuk alert internal, mis.
	// reminder retensi file desain (§19). Opsional: kalau kosong, alert
	// internal tidak dikirim (fitur off), bukan error.
	InternalPhone string
}

// Load reads env vars (optionally seeded from backend/.env) and returns a
// validated *Config. Any missing/invalid required field causes an error —
// caller MUST fail-fast (log.Fatal) rather than continue with defaults.
func Load() (*Config, error) {
	// .env is optional (may not exist in production containers); ignore error.
	_ = godotenv.Load()

	var errs []string

	env := Environment(getenvDefault("APP_ENV", "development"))
	if env != EnvDevelopment && env != EnvStaging && env != EnvProduction {
		errs = append(errs, fmt.Sprintf("APP_ENV invalid: %q (want development|staging|production)", env))
	}

	port, err := getenvInt("APP_PORT", 8080)
	if err != nil {
		errs = append(errs, err.Error())
	}

	baseURL := requireEnv("APP_BASE_URL", &errs)
	if baseURL != "" && !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		errs = append(errs, "APP_BASE_URL must start with http:// or https://")
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// Optional: FrontendURL. Kalau kosong, fallback ke BaseURL (§2 SATU sumber
	// tetap valid di produksi single-domain). Di dev / staging split-domain,
	// operator wajib set ini eksplisit.
	frontendURL := strings.TrimRight(getenvDefault("APP_FRONTEND_URL", ""), "/")
	if frontendURL == "" {
		frontendURL = baseURL
	} else if !strings.HasPrefix(frontendURL, "http://") && !strings.HasPrefix(frontendURL, "https://") {
		errs = append(errs, "APP_FRONTEND_URL must start with http:// or https://")
	}

	dbHost := requireEnv("DB_HOST", &errs)
	dbPort, err := getenvInt("DB_PORT", 5432)
	if err != nil {
		errs = append(errs, err.Error())
	}
	dbUser := requireEnv("DB_USER", &errs)
	dbPass := requireEnv("DB_PASSWORD", &errs)
	dbName := requireEnv("DB_NAME", &errs)
	dbSSL := getenvDefault("DB_SSLMODE", "disable")
	dbMaxOpen, err := getenvInt("DB_MAX_OPEN_CONNS", 25)
	if err != nil {
		errs = append(errs, err.Error())
	}
	dbMaxIdle, err := getenvInt("DB_MAX_IDLE_CONNS", 5)
	if err != nil {
		errs = append(errs, err.Error())
	}

	origins := splitCSV(getenvDefault("CORS_ALLOWED_ORIGINS", "http://localhost:3000"))

	// TRUSTED_PROXIES (security review finding) — comma-separated IP/CIDR
	// list. Default EMPTY → gin.Engine.SetTrustedProxies(nil) → ClientIP()
	// ignores X-Forwarded-For entirely and uses the raw TCP RemoteAddr. This
	// is the correct default: gin's OWN default is to trust every proxy,
	// which lets any client set X-Forwarded-For to whatever it wants and
	// rotate it to get a fresh rate-limit bucket per request (mis. against
	// the guest-checkout verify endpoint above). Only set this when the API
	// genuinely sits behind a known reverse proxy (mis. "127.0.0.1" for a
	// single-VPS nginx setup) — setting it to something internet-reachable
	// defeats the whole point.
	//
	// Di PRODUCTION nilai ini WAJIB diisi eksplisit — kosong tidak boleh
	// diterima diam-diam. Kalau API berjalan di belakang nginx satu VPS
	// (topologi §19/§25) dan TRUSTED_PROXIES kosong, ClientIP() mengembalikan
	// 127.0.0.1 untuk SETIAP request, sehingga seluruh pengunjung berbagi satu
	// token bucket: endpoint verify yang 0.2 rps jadi 0.2 rps GLOBAL, dan satu
	// penyerang cukup untuk mematikan semua endpoint publik. Gagalnya senyap —
	// persis yang §22 "config fail-fast" larang. Yang benar-benar tidak punya
	// proxy di depan menyatakannya eksplisit dengan TRUSTED_PROXIES=none.
	rawTrustedProxies := strings.TrimSpace(getenvDefault("TRUSTED_PROXIES", ""))
	var trustedProxies []string
	switch {
	case strings.EqualFold(rawTrustedProxies, "none"):
		// Pernyataan eksplisit "tidak ada proxy di depan" → abaikan header proxy.
		trustedProxies = nil
	case rawTrustedProxies == "":
		if env == EnvProduction {
			errs = append(errs, "TRUSTED_PROXIES wajib diisi di production: "+
				"daftar IP/CIDR reverse proxy (mis. \"127.0.0.1\" untuk nginx satu VPS), "+
				"atau \"none\" kalau API benar-benar terekspos langsung tanpa proxy. "+
				"Dibiarkan kosong, rate limit runtuh jadi satu bucket global")
		}
		trustedProxies = nil
	default:
		trustedProxies = splitCSV(rawTrustedProxies)
		for _, tp := range trustedProxies {
			if !isValidIPOrCIDR(tp) {
				errs = append(errs, fmt.Sprintf("TRUSTED_PROXIES entry %q is not a valid IP or CIDR", tp))
			}
		}
	}

	rlRPS, err := getenvFloat("RATE_LIMIT_PUBLIC_RPS", 5)
	if err != nil {
		errs = append(errs, err.Error())
	}
	rlBurst, err := getenvInt("RATE_LIMIT_PUBLIC_BURST", 10)
	if err != nil {
		errs = append(errs, err.Error())
	}

	// Dedicated (stricter) limiter for POST /lacak/:resi/verify — brute-force
	// vector on WhatsApp number, must be tighter than the general public group.
	guestVerifyRPS, err := getenvFloat("RATE_LIMIT_GUEST_VERIFY_RPS", 0.2)
	if err != nil {
		errs = append(errs, err.Error())
	}
	if guestVerifyRPS <= 0 {
		errs = append(errs, fmt.Sprintf("RATE_LIMIT_GUEST_VERIFY_RPS must be > 0, got %v", guestVerifyRPS))
	}
	guestVerifyBurst, err := getenvInt("RATE_LIMIT_GUEST_VERIFY_BURST", 3)
	if err != nil {
		errs = append(errs, err.Error())
	}
	if guestVerifyBurst <= 0 {
		errs = append(errs, fmt.Sprintf("RATE_LIMIT_GUEST_VERIFY_BURST must be > 0, got %d", guestVerifyBurst))
	}

	// Dedicated (stricter) limiter for POST /auth/google/request-otp and
	// POST /auth/google/complete — same brute-force shape as guest-verify
	// above (arbitrary WhatsApp number in an unauthenticated POST body).
	otpRateRPS, err := getenvFloat("RATE_LIMIT_OTP_RPS", 0.2)
	if err != nil {
		errs = append(errs, err.Error())
	}
	if otpRateRPS <= 0 {
		errs = append(errs, fmt.Sprintf("RATE_LIMIT_OTP_RPS must be > 0, got %v", otpRateRPS))
	}
	otpRateBurst, err := getenvInt("RATE_LIMIT_OTP_BURST", 3)
	if err != nil {
		errs = append(errs, err.Error())
	}
	if otpRateBurst <= 0 {
		errs = append(errs, fmt.Sprintf("RATE_LIMIT_OTP_BURST must be > 0, got %d", otpRateBurst))
	}

	notifURL := getenvDefault("NOTIFICATION_WORKER_URL", "http://localhost:9090")
	notifSecret := requireEnv("NOTIFICATION_WORKER_SECRET", &errs)
	if notifSecret != "" && len(notifSecret) < 32 {
		errs = append(errs, fmt.Sprintf("NOTIFICATION_WORKER_SECRET must be >= 32 chars (got %d)", len(notifSecret)))
	}
	notifMaxAttempts, err := getenvInt("NOTIFICATION_MAX_ATTEMPTS", 5)
	if err != nil {
		errs = append(errs, err.Error())
	}
	if notifMaxAttempts <= 0 || notifMaxAttempts > 20 {
		errs = append(errs, fmt.Sprintf("NOTIFICATION_MAX_ATTEMPTS out of range [1,20], got %d", notifMaxAttempts))
	}
	notifBackoffRaw := getenvDefault("NOTIFICATION_INITIAL_BACKOFF", "30s")
	notifBackoff, err := time.ParseDuration(notifBackoffRaw)
	if err != nil {
		errs = append(errs, fmt.Sprintf("NOTIFICATION_INITIAL_BACKOFF invalid duration %q", notifBackoffRaw))
	} else if notifBackoff <= 0 {
		errs = append(errs, "NOTIFICATION_INITIAL_BACKOFF must be > 0")
	}

	storageRoot := getenvDefault("STORAGE_LOCAL_ROOT", "./storage")
	// Ensure the storage root exists / is writable — fail fast if not (spec §22).
	if err := ensureWritableDir(storageRoot); err != nil {
		errs = append(errs, fmt.Sprintf("STORAGE_LOCAL_ROOT %q: %v", storageRoot, err))
	}

	paymentMaxMB, err := getenvInt("PAYMENT_MAX_UPLOAD_MB", 5)
	if err != nil {
		errs = append(errs, err.Error())
	}
	if paymentMaxMB <= 0 || paymentMaxMB > 50 {
		errs = append(errs, fmt.Sprintf("PAYMENT_MAX_UPLOAD_MB out of range [1,50], got %d", paymentMaxMB))
	}

	designMaxMB, err := getenvInt("DESIGN_MAX_UPLOAD_MB", 25)
	if err != nil {
		errs = append(errs, err.Error())
	}
	if designMaxMB <= 0 || designMaxMB > 200 {
		errs = append(errs, fmt.Sprintf("DESIGN_MAX_UPLOAD_MB out of range [1,200], got %d", designMaxMB))
	}

	cmsImgMaxMB, err := getenvInt("CMS_IMAGE_MAX_UPLOAD_MB", 8)
	if err != nil {
		errs = append(errs, err.Error())
	}
	if cmsImgMaxMB <= 0 || cmsImgMaxMB > 25 {
		errs = append(errs, fmt.Sprintf("CMS_IMAGE_MAX_UPLOAD_MB out of range [1,25], got %d", cmsImgMaxMB))
	}

	// Nomor WA ops untuk alert internal (§19 reminder retensi). Opsional —
	// tapi kalau diisi wajib format valid, biar salah ketik ketahuan saat
	// startup, bukan saat job tengah malam gagal kirim.
	notifInternalPhone := strings.TrimSpace(os.Getenv("NOTIFICATION_INTERNAL_PHONE"))
	if notifInternalPhone != "" {
		normalized, err := phone.Normalize(notifInternalPhone)
		if err != nil {
			errs = append(errs, fmt.Sprintf("NOTIFICATION_INTERNAL_PHONE invalid: %v", err))
		} else {
			notifInternalPhone = normalized
		}
	}

	// --- Retention job (§19) ---
	retentionEnabled, err := getenvBool("RETENTION_JOB_ENABLED", true)
	if err != nil {
		errs = append(errs, err.Error())
	}
	retentionDays, err := getenvInt("RETENTION_DEFAULT_DAYS", 30)
	if err != nil {
		errs = append(errs, err.Error())
	}
	if retentionDays <= 0 || retentionDays > 365 {
		errs = append(errs, fmt.Sprintf("RETENTION_DEFAULT_DAYS out of range [1,365], got %d", retentionDays))
	}
	retentionReminderDays, err := getenvInt("RETENTION_REMINDER_DAYS", 3)
	if err != nil {
		errs = append(errs, err.Error())
	}
	if retentionReminderDays <= 0 || retentionReminderDays > 30 {
		errs = append(errs, fmt.Sprintf("RETENTION_REMINDER_DAYS out of range [1,30], got %d", retentionReminderDays))
	}
	retentionBatch, err := getenvInt("RETENTION_BATCH_LIMIT", 200)
	if err != nil {
		errs = append(errs, err.Error())
	}
	if retentionBatch <= 0 || retentionBatch > 5000 {
		errs = append(errs, fmt.Sprintf("RETENTION_BATCH_LIMIT out of range [1,5000], got %d", retentionBatch))
	}
	// Sweep dini hari (trafik minimum), reminder jam kerja supaya staff benar-
	// benar baca WA-nya dan sempat download file sebelum kehapus.
	retentionSweepCron := getenvDefault("RETENTION_SWEEP_CRON", "0 3 * * *")
	retentionReminderCron := getenvDefault("RETENTION_REMINDER_CRON", "0 9 * * *")
	retentionTimeoutRaw := getenvDefault("RETENTION_JOB_TIMEOUT", "15m")
	retentionTimeout, err := time.ParseDuration(retentionTimeoutRaw)
	if err != nil {
		errs = append(errs, fmt.Sprintf("RETENTION_JOB_TIMEOUT invalid duration %q", retentionTimeoutRaw))
	} else if retentionTimeout <= 0 {
		errs = append(errs, "RETENTION_JOB_TIMEOUT must be > 0")
	}

	jwtSecret := requireEnv("JWT_SECRET", &errs)
	if jwtSecret != "" && len(jwtSecret) < 32 {
		errs = append(errs, fmt.Sprintf("JWT_SECRET must be >= 32 chars (got %d)", len(jwtSecret)))
	}
	jwtTTLRaw := getenvDefault("JWT_ACCESS_TTL", "24h")
	jwtTTL, err := time.ParseDuration(jwtTTLRaw)
	if err != nil {
		errs = append(errs, fmt.Sprintf("JWT_ACCESS_TTL invalid duration %q", jwtTTLRaw))
	} else if jwtTTL <= 0 {
		errs = append(errs, "JWT_ACCESS_TTL must be > 0")
	}
	jwtIssuer := getenvDefault("JWT_ISSUER", "rajaku-printing")

	jwtGuestTTLRaw := getenvDefault("JWT_GUEST_ORDER_TTL", "30m")
	jwtGuestTTL, err := time.ParseDuration(jwtGuestTTLRaw)
	if err != nil {
		errs = append(errs, fmt.Sprintf("JWT_GUEST_ORDER_TTL invalid duration %q", jwtGuestTTLRaw))
	} else if jwtGuestTTL <= 0 {
		errs = append(errs, "JWT_GUEST_ORDER_TTL must be > 0")
	}

	// --- Google OAuth (§3, §10) — OPTIONAL as a whole (dev can run without
	// it), but MUST be all-or-nothing: a partially-filled config is a config
	// bug (typo'd env var name, forgot one of the three) that must fail fast
	// rather than silently disable OAuth or crash later at request time.
	googleClientID := strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_CLIENT_ID"))
	googleClientSecret := strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"))
	googleRedirectURL := strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_REDIRECT_URL"))
	errs = append(errs, validateGoogleOAuth(googleClientID, googleClientSecret, googleRedirectURL)...)

	// --- OTP WhatsApp verification (Google OAuth registration) ---
	otpTTLRaw := getenvDefault("OTP_TTL", "5m")
	otpResendCooldownRaw := getenvDefault("OTP_RESEND_COOLDOWN", "60s")
	otpMaxAttempts, err := getenvInt("OTP_MAX_ATTEMPTS", 5)
	if err != nil {
		errs = append(errs, err.Error())
	}
	otpCodeLength, err := getenvInt("OTP_CODE_LENGTH", 6)
	if err != nil {
		errs = append(errs, err.Error())
	}
	otpCfg, otpErrs := parseOTPConfig(otpTTLRaw, otpResendCooldownRaw, otpMaxAttempts, otpCodeLength)
	errs = append(errs, otpErrs...)

	if len(errs) > 0 {
		return nil, errors.New("config invalid:\n  - " + strings.Join(errs, "\n  - "))
	}

	return &Config{
		App: AppConfig{
			Env:         env,
			Port:        port,
			BaseURL:     baseURL,
			FrontendURL: frontendURL,
			LogLevel:    getenvDefault("LOG_LEVEL", "info"),
		},
		DB: DBConfig{
			Host:         dbHost,
			Port:         dbPort,
			User:         dbUser,
			Password:     dbPass,
			Name:         dbName,
			SSLMode:      dbSSL,
			MaxOpenConns: dbMaxOpen,
			MaxIdleConns: dbMaxIdle,
		},
		CORS: CORSConfig{AllowedOrigins: origins},
		RateLimit: RateLimitConfig{
			PublicRPS:        rlRPS,
			PublicBurst:      rlBurst,
			GuestVerifyRPS:   guestVerifyRPS,
			GuestVerifyBurst: guestVerifyBurst,
			OTPRPS:           otpRateRPS,
			OTPBurst:         otpRateBurst,
		},
		OTP: otpCfg,
		JWT: JWTConfig{
			Secret:        jwtSecret,
			AccessTTL:     jwtTTL,
			Issuer:        jwtIssuer,
			GuestOrderTTL: jwtGuestTTL,
		},
		GoogleOAuth: GoogleOAuthConfig{
			ClientID:     googleClientID,
			ClientSecret: googleClientSecret,
			RedirectURL:  googleRedirectURL,
		},
		Notification: NotificationConfig{
			WorkerURL:      strings.TrimRight(notifURL, "/"),
			Timeout:        10 * time.Second,
			InternalSecret: notifSecret,
			MaxAttempts:    notifMaxAttempts,
			InitialBackoff: notifBackoff,
			InternalPhone:  notifInternalPhone,
		},
		Retention: RetentionConfig{
			Enabled:      retentionEnabled,
			DefaultDays:  retentionDays,
			ReminderDays: retentionReminderDays,
			SweepCron:    retentionSweepCron,
			ReminderCron: retentionReminderCron,
			BatchLimit:   retentionBatch,
			JobTimeout:   retentionTimeout,
		},
		TrustedProxies: trustedProxies,
		Storage:        StorageConfig{LocalRoot: storageRoot},
		Payment:        PaymentConfig{MaxUploadMB: paymentMaxMB},
		Design:         DesignConfig{MaxUploadMB: designMaxMB},
		CMS:            CMSConfig{MaxImageUploadMB: cmsImgMaxMB},
		Invoice: InvoiceConfig{
			CompanyName:    getenvDefault("INVOICE_COMPANY_NAME", "Rajaku Printing"),
			CompanyAddress: os.Getenv("INVOICE_COMPANY_ADDRESS"),
			CompanyPhone:   os.Getenv("INVOICE_COMPANY_PHONE"),
			CompanyEmail:   os.Getenv("INVOICE_COMPANY_EMAIL"),
		},
	}, nil
}

// ensureWritableDir creates the directory if missing and verifies we can write
// to it (temp file probe). Returns nil if the dir is usable.
func ensureWritableDir(p string) error {
	if p == "" {
		return errors.New("empty path")
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	probe, err := os.CreateTemp(p, ".rajaku-write-probe-*")
	if err != nil {
		return fmt.Errorf("write probe: %w", err)
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return nil
}

func requireEnv(key string, errs *[]string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		*errs = append(*errs, key+" is required")
	}
	return v
}

func getenvDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be integer, got %q", key, raw)
	}
	return n, nil
}

// getenvBool menerima 1/t/true/y/yes dan 0/f/false/n/no (case-insensitive).
// Nilai lain = error eksplisit, bukan diam-diam jadi false — typo seperti
// "yess" tidak boleh menonaktifkan job pembersihan disk tanpa disadari.
func getenvBool(key string, def bool) (bool, error) {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if raw == "" {
		return def, nil
	}
	switch raw {
	case "1", "t", "true", "y", "yes":
		return true, nil
	case "0", "f", "false", "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be boolean (true/false), got %q", key, raw)
	}
}

func getenvFloat(key string, def float64) (float64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be number, got %q", key, raw)
	}
	return f, nil
}

// isValidIPOrCIDR reports whether s parses as a bare IP address or a CIDR
// block — mirrors the shape gin.Engine.prepareTrustedCIDRs() itself accepts
// (bare IP gets an implicit /32 or /128), so a value that fails here would
// also fail at SetTrustedProxies() time — we just want that failure at
// startup (fail-fast §22), not silently swallowed inside gin.
func isValidIPOrCIDR(s string) bool {
	if strings.Contains(s, "/") {
		_, _, err := net.ParseCIDR(s)
		return err == nil
	}
	return net.ParseIP(s) != nil
}

// validateGoogleOAuth enforces the fail-fast rule for the three
// GOOGLE_OAUTH_* env vars: either all three are empty (OAuth disabled — a
// valid, supported state for dev) or all three are set, in which case
// RedirectURL must parse as an absolute http(s) URL. Any other combination
// (partial fill, or a non-absolute RedirectURL) returns error message(s) to
// merge into Load()'s errs slice.
//
// Kept as a pure function (mirrors isValidIPOrCIDR below) so it's unit-
// testable without needing the rest of Load()'s required env vars.
func validateGoogleOAuth(clientID, clientSecret, redirectURL string) []string {
	filled := 0
	for _, v := range []string{clientID, clientSecret, redirectURL} {
		if v != "" {
			filled++
		}
	}
	switch filled {
	case 0:
		return nil
	case 3:
		u, err := url.Parse(redirectURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return []string{fmt.Sprintf("GOOGLE_OAUTH_REDIRECT_URL must be an absolute http(s) URL, got %q", redirectURL)}
		}
		return nil
	default:
		return []string{"GOOGLE_OAUTH_CLIENT_ID, GOOGLE_OAUTH_CLIENT_SECRET, and GOOGLE_OAUTH_REDIRECT_URL must all be set, or all left empty (partial config is not allowed)"}
	}
}

// parseOTPConfig validates & builds an OTPConfig from raw env inputs. Kept
// as a pure function (mirrors validateGoogleOAuth above) so it's unit-
// testable without needing the rest of Load()'s required env vars.
func parseOTPConfig(ttlRaw, resendCooldownRaw string, maxAttempts, codeLength int) (OTPConfig, []string) {
	var errs []string

	ttl, err := time.ParseDuration(ttlRaw)
	if err != nil {
		errs = append(errs, fmt.Sprintf("OTP_TTL invalid duration %q", ttlRaw))
	} else if ttl <= 0 {
		errs = append(errs, "OTP_TTL must be > 0")
	}

	resend, err := time.ParseDuration(resendCooldownRaw)
	if err != nil {
		errs = append(errs, fmt.Sprintf("OTP_RESEND_COOLDOWN invalid duration %q", resendCooldownRaw))
	} else if resend <= 0 {
		errs = append(errs, "OTP_RESEND_COOLDOWN must be > 0")
	}

	if maxAttempts < 1 || maxAttempts > 10 {
		errs = append(errs, fmt.Sprintf("OTP_MAX_ATTEMPTS out of range [1,10], got %d", maxAttempts))
	}

	if codeLength < 4 || codeLength > 8 {
		errs = append(errs, fmt.Sprintf("OTP_CODE_LENGTH out of range [4,8], got %d", codeLength))
	}

	if len(errs) > 0 {
		return OTPConfig{}, errs
	}
	return OTPConfig{TTL: ttl, MaxAttempts: maxAttempts, ResendCooldown: resend, CodeLength: codeLength}, nil
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
