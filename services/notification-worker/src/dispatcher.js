// Dispatcher loop — poll backend, kirim via WA, lapor status.
//
// Semua perlindungan anti-ban (§13 — Baileys tidak resmi) berlapis di sini:
//   1. pacing.js  — jeda acak + istirahat berkala (bukan interval konstan).
//   2. quota.js   — warmup nomor baru, cap harian, cap per penerima.
//   3. wa.js      — cek nomor terdaftar (onWhatsApp) + simulasi mengetik.
//   4. circuit.js — berhenti sementara saat WA mulai menahan.
//
// Kontrak dengan backend tidak berubah: worker lapor `sent` / `failed` sekali
// per attempt, backend yang memutuskan `failed` (retry) vs `dead` (habis).
// Untuk kegagalan yang PASTI tidak akan sembuh dengan retry (nomor tidak
// terdaftar, message kosong, format nomor salah) alasan diberi prefix
// `PERMANENT:` supaya gampang difilter operator/backend saat triase.
import pino from 'pino';
import { claimJobs, markSent, markFailed } from './backendClient.js';
import { sendText, isReady, isRegisteredOnWhatsApp } from './wa.js';
import { config } from './config.js';
import * as pacing from './pacing.js';
import * as quota from './quota.js';
import * as circuit from './circuit.js';

const log = pino({ level: config.logLevel, base: undefined });

const PERMANENT = 'PERMANENT:';

let running = false;
let stopped = false;
let lastCapLogAt = 0;

async function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms));
}

// Tidur yang tetap responsif terhadap SIGTERM — dipakai untuk jeda panjang
// (cap harian habis / circuit open) supaya shutdown tidak nunggu berjam-jam.
async function sleepInterruptible(ms) {
  const until = Date.now() + ms;
  while (!stopped && Date.now() < until) {
    await sleep(Math.min(1000, until - Date.now()));
  }
}

async function reportFailure(job, reason) {
  try {
    await markFailed(job.id, reason);
  } catch (reportErr) {
    // Kalau lapor gagal juga, log — backend akan re-claim setelah
    // next_attempt_at expire (job masih di status 'sending', tapi
    // ClaimBatch guarded by SKIP LOCKED; kalau worker crash, row
    // stuck 'sending' — mitigation: tambah janitor untuk unstick,
    // TODO(notif): sweeper untuk 'sending' > N menit).
    log.error({ jobId: job.id, err: reportErr?.message }, 'failed reporting failure — akan retry saat next claim');
  }
}

// Job yang sudah ter-claim tapi tidak jadi dikirim WAJIB tetap dilaporkan,
// kalau tidak row-nya nyangkut di status 'sending'.
async function deferJobs(jobs, reason) {
  for (const job of jobs) {
    log.warn({ jobId: job.id, kind: job.kind, reason }, 'job ditunda — dilaporkan gagal supaya backend reschedule');
    await reportFailure(job, reason);
  }
}

async function processJob(job) {
  const phone = (job.recipient_phone || '').trim();

  // Sabuk pengaman: message kosong dari backend tidak boleh dikirim (pesan
  // kosong ke pelanggan = noise + sinyal spam, dan retry tidak akan menolong).
  if (!job.message || !job.message.trim()) {
    const reason = `${PERMANENT} message kosong dari backend — tidak dikirim`;
    log.error({ jobId: job.id, kind: job.kind }, reason);
    await reportFailure(job, reason);
    return;
  }

  if (!/^62\d{8,13}$/.test(phone)) {
    const reason = `${PERMANENT} format nomor tidak valid: "${phone}" (harus 62xxx)`;
    log.error({ jobId: job.id, kind: job.kind }, reason);
    await reportFailure(job, reason);
    return;
  }

  // Cap per penerima — mencegah satu pelanggan dibanjiri sampai menekan
  // Blokir/Laporkan (sinyal ban terkuat).
  const rec = quota.checkRecipient(phone);
  if (!rec.allowed) {
    if (rec.waitMs > 0 && rec.waitMs <= config.recipientWaitMaxMs) {
      log.info({ jobId: job.id, phone, waitMs: rec.waitMs }, 'menunggu jeda minimum ke nomor yang sama');
      await sleepInterruptible(rec.waitMs);
      if (stopped) {
        await reportFailure(job, 'worker shutdown sebelum kirim — silakan retry');
        return;
      }
    } else {
      const reason = `ditunda: ${rec.reason}`;
      log.warn({ jobId: job.id, phone, sentToRecipient: rec.sentToRecipient }, reason);
      await reportFailure(job, reason);
      return;
    }
  }

  // Nomor tidak punya WhatsApp → jangan kirim sama sekali.
  const registered = await isRegisteredOnWhatsApp(phone);
  if (registered === false) {
    const reason = `${PERMANENT} nomor tidak terdaftar di WhatsApp (onWhatsApp: exists=false)`;
    log.error({ jobId: job.id, kind: job.kind, phone }, reason);
    await reportFailure(job, reason);
    return;
  }

  await pacing.waitBeforeSend();
  if (stopped) {
    await reportFailure(job, 'worker shutdown sebelum kirim — silakan retry');
    return;
  }

  try {
    try {
      await sendText(phone, job.message);
    } finally {
      // Dihitung per PERCOBAAN: gagal pun sudah menyentuh server WhatsApp,
      // jadi jeda & kuota tetap dibebani (arah aman untuk risiko ban).
      pacing.noteSent();
      quota.noteSent(phone);
    }
    circuit.recordSuccess();
    await markSent(job.id);
    const d = quota.checkDaily();
    log.info({
      jobId: job.id, kind: job.kind, phone,
      sentToday: d.sent, limitToday: d.limit, sessionDay: d.sessionDay,
    }, 'sent');
  } catch (err) {
    const msg = err?.message || String(err);
    circuit.recordFailure(err);
    log.error({ jobId: job.id, kind: job.kind, err: msg }, 'send failed — reporting to backend');
    await reportFailure(job, msg);
  }
}

function logDailyCapReached(daily) {
  const now = Date.now();
  if (now - lastCapLogAt < 300_000) return;
  lastCapLogAt = now;
  const resetAt = new Date(now + daily.resetInMs).toISOString();
  log.warn({
    sentToday: daily.sent,
    limitToday: daily.limit,
    sessionDay: daily.sessionDay,
    warmup: daily.warmup,
    resetInMs: daily.resetInMs,
    resetAtUTC: resetAt,
    timezone: config.timezone,
  }, daily.warmup
    ? 'CAP WARMUP HARIAN TERCAPAI — berhenti klaim job baru sampai tengah malam waktu lokal (nomor masih masa warmup)'
    : 'CAP HARIAN TERCAPAI — berhenti klaim job baru sampai tengah malam waktu lokal');
}

export async function startDispatcher() {
  if (running) return;
  running = true;
  quota.initQuota();
  log.info({
    pollIntervalMs: config.pollIntervalMs,
    batchSize: config.batchSize,
    sendIntervalMs: [config.minSendIntervalMs, config.maxSendIntervalMs],
    burstPauseEvery: config.burstPauseEvery,
    burstPauseMs: [config.burstPauseMs, config.burstPauseMaxMs],
    warmupRamp: config.warmupRamp,
    dailyCap: config.dailyCap,
    maxPerRecipientDay: config.maxPerRecipientDay,
    checkRegistered: config.checkRegistered,
    simulateTyping: config.simulateTyping,
    timezone: config.timezone,
  }, 'dispatcher loop started');

  while (!stopped) {
    try {
      if (!isReady()) {
        // WA belum konek — jangan drain queue supaya job tidak ke-attempt & langsung failed.
        await sleep(config.pollIntervalMs);
        continue;
      }

      // Circuit terbuka → WA sedang menahan. Jangan klaim apa pun.
      const openMs = circuit.remainingOpenMs();
      if (openMs > 0) {
        await sleepInterruptible(Math.min(openMs, 30_000));
        continue;
      }

      const daily = quota.checkDaily();
      if (!daily.allowed) {
        logDailyCapReached(daily);
        await sleepInterruptible(Math.min(daily.resetInMs, 60_000));
        continue;
      }

      const limit = Math.max(1, Math.min(config.batchSize, daily.remaining));
      const jobs = await claimJobs(limit);
      if (jobs.length === 0) {
        await sleep(config.pollIntervalMs);
        continue;
      }
      log.debug({ n: jobs.length, remainingToday: daily.remaining }, 'claimed batch');

      // Sequential — pacing antar-pesan sudah handled di processJob.
      for (let i = 0; i < jobs.length; i += 1) {
        if (stopped) {
          await deferJobs(jobs.slice(i), 'worker shutdown — job dikembalikan untuk retry');
          break;
        }
        if (circuit.remainingOpenMs() > 0) {
          await deferJobs(jobs.slice(i), 'circuit breaker terbuka (WA menahan) — ditunda');
          break;
        }
        if (!quota.checkDaily().allowed) {
          await deferJobs(jobs.slice(i), 'cap harian WA tercapai — ditunda sampai besok');
          break;
        }
        await processJob(jobs[i]);
      }
    } catch (err) {
      // Umum: backend down / secret salah. Log + backoff.
      log.error({ err: err?.message, status: err?.status }, 'poll cycle failed');
      await sleepInterruptible(Math.max(config.pollIntervalMs, 10_000));
    }
  }
  running = false;
  log.info('dispatcher stopped');
}

export function stopDispatcher() { stopped = true; }
