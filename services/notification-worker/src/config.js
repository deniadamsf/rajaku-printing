// Config loader — validasi fail-fast, mirror pola backend Go (§22).
// Ambil dari process.env; kalau ada file .env, load manual (tanpa dep tambahan).
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const envFile = path.resolve(__dirname, '..', '.env');
if (fs.existsSync(envFile)) {
  const raw = fs.readFileSync(envFile, 'utf8');
  for (const line of raw.split(/\r?\n/)) {
    const m = line.match(/^\s*([A-Z0-9_]+)\s*=\s*(.*)\s*$/);
    if (!m) continue;
    if (line.trim().startsWith('#')) continue;
    // Jangan override kalau sudah ada di process.env (mis. docker-compose).
    if (process.env[m[1]] === undefined) {
      let val = m[2];
      if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
        val = val.slice(1, -1);
      }
      process.env[m[1]] = val;
    }
  }
}

function required(name) {
  const v = (process.env[name] || '').trim();
  if (!v) throw new Error(`missing required env: ${name}`);
  return v;
}

function intWithDefault(name, def) {
  const raw = (process.env[name] || '').trim();
  if (!raw) return def;
  const n = Number.parseInt(raw, 10);
  if (Number.isNaN(n) || n <= 0) throw new Error(`${name} must be positive integer, got "${raw}"`);
  return n;
}

// Varian yang mengizinkan 0 — dipakai untuk knob yang "0 = matikan fitur"
// (mis. BURST_PAUSE_EVERY=0 → tidak pernah istirahat).
function uintWithDefault(name, def) {
  const raw = (process.env[name] || '').trim();
  if (!raw) return def;
  const n = Number.parseInt(raw, 10);
  if (Number.isNaN(n) || n < 0) throw new Error(`${name} must be non-negative integer, got "${raw}"`);
  return n;
}

function boolWithDefault(name, def) {
  const raw = (process.env[name] || '').trim().toLowerCase();
  if (!raw) return def;
  if (['1', 'true', 'yes', 'y', 'on'].includes(raw)) return true;
  if (['0', 'false', 'no', 'n', 'off'].includes(raw)) return false;
  throw new Error(`${name} must be boolean (true/false), got "${raw}"`);
}

// CSV angka positif, mis. WA_WARMUP_RAMP=20,40,80,150,250. String kosong =
// ramp dimatikan (langsung pakai WA_DAILY_CAP).
function intListWithDefault(name, def) {
  const raw = process.env[name];
  if (raw === undefined) return def;
  const trimmed = raw.trim();
  if (!trimmed) return [];
  return trimmed.split(',').map((part) => {
    const n = Number.parseInt(part.trim(), 10);
    if (Number.isNaN(n) || n <= 0) {
      throw new Error(`${name} must be comma-separated positive integers, got "${raw}"`);
    }
    return n;
  });
}

function timezoneWithDefault(name, def) {
  const tz = (process.env[name] || '').trim() || def;
  try {
    new Intl.DateTimeFormat('en-CA', { timeZone: tz });
  } catch {
    throw new Error(`${name} is not a valid IANA timezone: "${tz}"`);
  }
  return tz;
}

// Pasangan min/max wajib konsisten — fail fast daripada diam-diam terbalik.
function assertRange(minName, minVal, maxName, maxVal) {
  if (maxVal < minVal) {
    throw new Error(`${maxName} (${maxVal}) must be >= ${minName} (${minVal})`);
  }
}

const backendURL = required('BACKEND_API_URL').replace(/\/+$/, '');
const internalSecret = required('INTERNAL_SECRET');
if (internalSecret.length < 32) {
  throw new Error('INTERNAL_SECRET must be >= 32 chars (must match backend NOTIFICATION_WORKER_SECRET)');
}

const sessionDir = (process.env.SESSION_DIR || './session').trim();

// Pacing — jeda antar pesan diacak dalam rentang [min, max] plus "istirahat"
// berkala. Jeda konstan = tanda tangan bot (§13, risiko banned).
const minSendIntervalMs = intWithDefault('MIN_SEND_INTERVAL_MS', 3000);
const maxSendIntervalMs = intWithDefault('MAX_SEND_INTERVAL_MS', 9000);
assertRange('MIN_SEND_INTERVAL_MS', minSendIntervalMs, 'MAX_SEND_INTERVAL_MS', maxSendIntervalMs);

const burstPauseMs = intWithDefault('BURST_PAUSE_MS', 45_000);
const burstPauseMaxMs = intWithDefault('BURST_PAUSE_MAX_MS', 120_000);
assertRange('BURST_PAUSE_MS', burstPauseMs, 'BURST_PAUSE_MAX_MS', burstPauseMaxMs);

const typingMsPerCharMin = intWithDefault('WA_TYPING_MS_PER_CHAR_MIN', 40);
const typingMsPerCharMax = intWithDefault('WA_TYPING_MS_PER_CHAR_MAX', 80);
assertRange('WA_TYPING_MS_PER_CHAR_MIN', typingMsPerCharMin, 'WA_TYPING_MS_PER_CHAR_MAX', typingMsPerCharMax);

const typingMinMs = intWithDefault('WA_TYPING_MIN_MS', 1000);
const typingMaxMs = intWithDefault('WA_TYPING_MAX_MS', 4000);
assertRange('WA_TYPING_MIN_MS', typingMinMs, 'WA_TYPING_MAX_MS', typingMaxMs);

const circuitBaseBackoffMs = intWithDefault('WA_CIRCUIT_BASE_BACKOFF_MS', 60_000);
const circuitMaxBackoffMs = intWithDefault('WA_CIRCUIT_MAX_BACKOFF_MS', 1_800_000);
assertRange('WA_CIRCUIT_BASE_BACKOFF_MS', circuitBaseBackoffMs, 'WA_CIRCUIT_MAX_BACKOFF_MS', circuitMaxBackoffMs);

export const config = {
  backendURL,
  internalSecret,
  pollIntervalMs: intWithDefault('POLL_INTERVAL_MS', 5000),
  batchSize: intWithDefault('BATCH_SIZE', 5),
  sessionDir,
  workerPort: intWithDefault('WORKER_PORT', 9090),
  logLevel: (process.env.LOG_LEVEL || 'info').trim(),

  // Timezone acuan untuk batas harian (cap & warmup reset lewat tengah malam).
  timezone: timezoneWithDefault('TZ', 'Asia/Jakarta'),

  // --- Pacing (anti-pola-bot) ---
  minSendIntervalMs,
  maxSendIntervalMs,
  burstPauseEvery: uintWithDefault('BURST_PAUSE_EVERY', 8),
  burstPauseMs,
  burstPauseMaxMs,

  // --- Warmup & cap harian ---
  // Ramp per hari umur sesi (hari ke-1..ke-N); habis ramp → warmupDailyCap.
  warmupRamp: intListWithDefault('WA_WARMUP_RAMP', [20, 40, 80, 150, 250]),
  dailyCap: intWithDefault('WA_DAILY_CAP', 500),
  // State cap/warmup ditulis di sini. Default: sebelah SESSION_DIR (bukan di
  // dalamnya) — isi session dir dihapus saat logout, state warmup jangan ikut
  // hilang gara-gara reconnect biasa.
  stateFile: (process.env.WA_STATE_FILE || '').trim()
    || path.resolve(process.cwd(), sessionDir, '..', 'wa-state.json'),

  // --- Cap per penerima ---
  maxPerRecipientDay: intWithDefault('WA_MAX_PER_RECIPIENT_DAY', 5),
  minRecipientIntervalMs: uintWithDefault('WA_MIN_RECIPIENT_INTERVAL_MS', 60_000),
  // Kalau jeda ke nomor yang sama belum lewat, worker menunggu selama <= nilai
  // ini; lebih dari itu job dilaporkan gagal (backend yang reschedule) supaya
  // antrian tidak macet lama.
  recipientWaitMaxMs: uintWithDefault('WA_RECIPIENT_WAIT_MAX_MS', 120_000),

  // --- Cek nomor terdaftar ---
  checkRegistered: boolWithDefault('WA_CHECK_REGISTERED', true),
  numberCacheTtlMs: intWithDefault('WA_NUMBER_CACHE_TTL_MS', 86_400_000),

  // --- Simulasi mengetik ---
  simulateTyping: boolWithDefault('WA_SIMULATE_TYPING', true),
  typingMsPerCharMin,
  typingMsPerCharMax,
  typingMinMs,
  typingMaxMs,

  // --- Circuit breaker ---
  circuitFailureThreshold: intWithDefault('WA_CIRCUIT_FAILURE_THRESHOLD', 3),
  circuitBaseBackoffMs,
  circuitMaxBackoffMs,
};
