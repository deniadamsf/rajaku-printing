// Dispatcher loop — poll backend, kirim via WA, lapor status. Rate-limit
// aware: setiap pesan minimal jeda MIN_SEND_INTERVAL_MS (§13 — Baileys
// tidak resmi, gampang banned kalau spam).
import pino from 'pino';
import { claimJobs, markSent, markFailed } from './backendClient.js';
import { sendText, isReady } from './wa.js';
import { config } from './config.js';

const log = pino({ level: config.logLevel, base: undefined });

let running = false;
let stopped = false;
let lastSentAt = 0;

async function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms));
}

async function throttle() {
  const now = Date.now();
  const wait = config.minSendIntervalMs - (now - lastSentAt);
  if (wait > 0) await sleep(wait);
  lastSentAt = Date.now();
}

async function processJob(job) {
  try {
    await throttle();
    await sendText(job.recipient_phone, job.message);
    await markSent(job.id);
    log.info({ jobId: job.id, kind: job.kind, phone: job.recipient_phone }, 'sent');
  } catch (err) {
    const msg = err?.message || String(err);
    log.error({ jobId: job.id, kind: job.kind, err: msg }, 'send failed — reporting to backend');
    try {
      await markFailed(job.id, msg);
    } catch (reportErr) {
      // Kalau lapor gagal juga, log — backend akan re-claim setelah
      // next_attempt_at expire (job masih di status 'sending', tapi
      // ClaimBatch guarded by SKIP LOCKED; kalau worker crash, row
      // stuck 'sending' — mitigation: tambah janitor untuk unstick,
      // TODO(notif): sweeper untuk 'sending' > N menit).
      log.error({ jobId: job.id, err: reportErr?.message }, 'failed reporting failure — akan retry saat next claim');
    }
  }
}

export async function startDispatcher() {
  if (running) return;
  running = true;
  log.info({ pollIntervalMs: config.pollIntervalMs, batchSize: config.batchSize }, 'dispatcher loop started');
  while (!stopped) {
    try {
      if (!isReady()) {
        // WA belum konek — jangan drain queue supaya job tidak ke-attempt & langsung failed.
        await sleep(config.pollIntervalMs);
        continue;
      }
      const jobs = await claimJobs(config.batchSize);
      if (jobs.length === 0) {
        await sleep(config.pollIntervalMs);
        continue;
      }
      log.debug({ n: jobs.length }, 'claimed batch');
      // Sequential — throttle antar-pesan sudah handled di processJob.
      for (const job of jobs) {
        if (stopped) break;
        await processJob(job);
      }
    } catch (err) {
      // Umum: backend down / secret salah. Log + backoff.
      log.error({ err: err?.message, status: err?.status }, 'poll cycle failed');
      await sleep(Math.max(config.pollIntervalMs, 10_000));
    }
  }
  running = false;
  log.info('dispatcher stopped');
}

export function stopDispatcher() { stopped = true; }
