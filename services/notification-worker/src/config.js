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

const backendURL = required('BACKEND_API_URL').replace(/\/+$/, '');
const internalSecret = required('INTERNAL_SECRET');
if (internalSecret.length < 32) {
  throw new Error('INTERNAL_SECRET must be >= 32 chars (must match backend NOTIFICATION_WORKER_SECRET)');
}

export const config = {
  backendURL,
  internalSecret,
  pollIntervalMs: intWithDefault('POLL_INTERVAL_MS', 5000),
  batchSize: intWithDefault('BATCH_SIZE', 5),
  minSendIntervalMs: intWithDefault('MIN_SEND_INTERVAL_MS', 3000),
  sessionDir: (process.env.SESSION_DIR || './session').trim(),
  workerPort: intWithDefault('WORKER_PORT', 9090),
  logLevel: (process.env.LOG_LEVEL || 'info').trim(),
};
