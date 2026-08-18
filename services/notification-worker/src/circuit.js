// Circuit breaker — berhenti sementara saat WhatsApp mulai menahan.
//
// Kalau Baileys mulai balas "connection closed" / rate-limit / 429-like
// berturut-turut, itu sinyal bahaya: server WA sudah menahan nomor kita.
// Terus menghantam justru mempercepat ban (§13). Jadi: buka circuit, tidur
// dengan backoff eksponensial, baru coba lagi.
import pino from 'pino';
import { config } from './config.js';
import { jitterMs } from './pacing.js';

const log = pino({ level: config.logLevel, base: undefined });

// Pola error yang dianggap "sinyal bahaya" dari sisi WhatsApp/Baileys.
// Sengaja longgar — false positive cuma bikin worker jeda, false negative
// bikin nomor kena.
const DANGER_PATTERNS = [
  /rate.?limit/i,
  /too many/i,
  /429/,
  /overlimit/i,
  /connection closed/i,
  /connection lost/i,
  /connection terminated/i,
  /stream error/i,
  /restart required/i,
  /timed? out/i,
  /timeout/i,
  /service unavailable/i,
  /503/,
  /precondition required/i,
  /not-?authorized/i,
  /forbidden/i,
  /blocked/i,
  /wa belum siap/i,
];

const DANGER_STATUS = new Set([408, 428, 429, 440, 500, 503]);

let consecutiveDanger = 0;
let openUntil = 0;
let openings = 0; // berapa kali circuit terbuka sejak sukses terakhir
let totalOpenings = 0;

export function isDangerError(err) {
  if (!err) return false;
  const status = err.output?.statusCode ?? err.status ?? err.statusCode;
  if (typeof status === 'number' && DANGER_STATUS.has(status)) return true;
  const msg = err.message || String(err);
  return DANGER_PATTERNS.some((re) => re.test(msg));
}

export function recordSuccess() {
  if (consecutiveDanger > 0 || openings > 0) {
    log.info({ prevConsecutive: consecutiveDanger }, 'kirim sukses lagi — circuit di-reset');
  }
  consecutiveDanger = 0;
  openings = 0;
  openUntil = 0;
}

// Catat kegagalan. Return true kalau circuit baru saja dibuka.
export function recordFailure(err) {
  if (!isDangerError(err)) return false;
  consecutiveDanger += 1;
  if (consecutiveDanger < config.circuitFailureThreshold) {
    log.warn({ consecutiveDanger, threshold: config.circuitFailureThreshold, err: err?.message },
      'sinyal bahaya dari WhatsApp — belum sampai ambang circuit');
    return false;
  }

  openings += 1;
  totalOpenings += 1;
  const shift = Math.min(openings - 1, 20);
  const base = Math.min(config.circuitBaseBackoffMs * 2 ** shift, config.circuitMaxBackoffMs);
  // Jitter ke atas (0-25%) supaya beberapa restart worker tidak serempak
  // menghantam di detik yang sama.
  const backoff = Math.min(jitterMs(base, Math.round(base * 1.25)), config.circuitMaxBackoffMs);
  openUntil = Date.now() + backoff;
  consecutiveDanger = 0;
  log.error({
    backoffMs: backoff,
    openings,
    totalOpenings,
    resumeAt: new Date(openUntil).toISOString(),
    err: err?.message,
  }, 'CIRCUIT OPEN — pengiriman WA dihentikan sementara untuk melindungi nomor');
  return true;
}

export function remainingOpenMs() {
  const left = openUntil - Date.now();
  return left > 0 ? left : 0;
}

export function isOpen() {
  return remainingOpenMs() > 0;
}

export function circuitSnapshot() {
  return {
    open: isOpen(),
    remaining_open_ms: remainingOpenMs(),
    consecutive_danger: consecutiveDanger,
    openings_since_success: openings,
    total_openings: totalOpenings,
  };
}
