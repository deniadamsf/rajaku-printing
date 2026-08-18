// Kuota kirim — warmup nomor baru, cap harian, dan cap per penerima.
//
// Kenapa (§13, Baileys tidak resmi):
//  1. Nomor yang baru dipasang lalu langsung menyemprot ratusan pesan adalah
//     penyebab ban paling umum → cap harian menanjak (warmup ramp) mengikuti
//     umur sesi.
//  2. Satu pelanggan yang dibanjiri akan menekan Blokir/Laporkan — laporan
//     penerima adalah sinyal ban terkuat → cap per penerima + jeda minimum.
//
// State di-PERSIST ke disk (config.stateFile) karena worker bisa restart
// kapan saja (pm2/systemd) dan hitungan harian tidak boleh ikut ter-reset.
// File sengaja ditaruh di SEBELAH session dir, bukan di dalamnya: isi session
// dir dihapus saat logout, sedangkan riwayat kuota masih berguna. Umur sesi
// di-anchor ke JID yang ter-pairing — kalau nomor WA-nya berganti, warmup
// otomatis mulai lagi dari hari ke-1 (persis kasus "nomor baru dipasang").
import fs from 'node:fs';
import path from 'node:path';
import pino from 'pino';
import { config } from './config.js';

const log = pino({ level: config.logLevel, base: undefined });

const STATE_VERSION = 1;

let state = null;

function emptyState() {
  return {
    version: STATE_VERSION,
    waJid: null,
    sessionStartedDay: null, // day key lokal saat sesi/nomor ini pertama dipakai
    day: null,               // day key lokal untuk hitungan di bawah
    sentToday: 0,
    recipients: {},          // { "62812...": { n: <count>, t: <lastSentAt ms> } }
  };
}

// Day key lokal (YYYY-MM-DD) menurut config.timezone. Pakai Intl supaya tidak
// bergantung pada TZ proses Node.
export function localDayKey(date = new Date()) {
  return new Intl.DateTimeFormat('en-CA', {
    timeZone: config.timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(date);
}

// Sisa ms sampai tengah malam waktu lokal — dipakai untuk log "cap reset jam
// sekian" dan untuk tidur saat cap harian habis.
export function msUntilLocalMidnight(date = new Date()) {
  const parts = new Intl.DateTimeFormat('en-GB', {
    timeZone: config.timezone,
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).formatToParts(date);
  const get = (t) => Number.parseInt(parts.find((p) => p.type === t)?.value || '0', 10);
  const h = get('hour') % 24;
  const m = get('minute');
  const s = get('second');
  const elapsed = ((h * 60 + m) * 60 + s) * 1000 + date.getMilliseconds();
  return Math.max(86_400_000 - elapsed, 1000);
}

function dayKeyToUTC(key) {
  return Date.parse(`${key}T00:00:00Z`);
}

function persist() {
  try {
    const dir = path.dirname(config.stateFile);
    if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
    const tmp = `${config.stateFile}.tmp`;
    // Tulis ke tmp lalu rename — hindari file setengah tertulis kalau worker
    // dimatikan tepat saat persist.
    fs.writeFileSync(tmp, JSON.stringify(state), 'utf8');
    fs.renameSync(tmp, config.stateFile);
  } catch (err) {
    // Bukan alasan untuk berhenti kirim; tapi harus kelihatan di log karena
    // artinya hitungan bisa ter-reset saat restart.
    log.error({ err: err.message, file: config.stateFile }, 'gagal menulis state kuota WA');
  }
}

function rollover() {
  const today = localDayKey();
  if (state.day === today) return;
  if (state.day) {
    log.info({ prevDay: state.day, prevSent: state.sentToday, newDay: today },
      'hari baru (waktu lokal) — hitungan cap harian di-reset');
  }
  state.day = today;
  state.sentToday = 0;
  state.recipients = {};
  persist();
}

export function initQuota() {
  // Idempotent — bindSession() bisa saja sudah memuat state duluan saat event
  // 'connection open' Baileys datang sebelum dispatcher start.
  if (state) {
    rollover();
    return;
  }
  state = emptyState();
  try {
    if (fs.existsSync(config.stateFile)) {
      const parsed = JSON.parse(fs.readFileSync(config.stateFile, 'utf8'));
      if (parsed && typeof parsed === 'object') {
        state = {
          ...emptyState(),
          ...parsed,
          recipients: (parsed.recipients && typeof parsed.recipients === 'object') ? parsed.recipients : {},
          sentToday: Number.isInteger(parsed.sentToday) ? parsed.sentToday : 0,
        };
      }
    }
  } catch (err) {
    log.warn({ err: err.message, file: config.stateFile },
      'state kuota WA tidak terbaca — mulai dari nol (konservatif: warmup dihitung ulang)');
    state = emptyState();
  }
  rollover();
  log.info({
    stateFile: config.stateFile,
    timezone: config.timezone,
    warmupRamp: config.warmupRamp,
    dailyCap: config.dailyCap,
    sessionDay: sessionDayIndex(),
    limitToday: dailyLimit(),
    sentToday: state.sentToday,
  }, 'kuota WA siap');
}

// Kaitkan state ke nomor yang sedang ter-pairing. Kalau nomornya berbeda dari
// yang tercatat (nomor baru dipasang / re-pairing dengan nomor lain), umur
// sesi di-reset supaya warmup mulai lagi dari hari ke-1.
export function bindSession(waJid) {
  if (!state) initQuota();
  if (!waJid) return;
  if (state.waJid === waJid && state.sessionStartedDay) return;
  const isSwitch = Boolean(state.waJid) && state.waJid !== waJid;
  state.waJid = waJid;
  state.sessionStartedDay = localDayKey();
  if (isSwitch) {
    // Nomor ganti → hitungan harian nomor lama tidak relevan.
    state.sentToday = 0;
    state.recipients = {};
  }
  persist();
  log.warn({ waJid, sessionStartedDay: state.sessionStartedDay, isSwitch, limitToday: dailyLimit() },
    isSwitch
      ? 'nomor WA berganti — warmup dimulai ulang dari hari ke-1'
      : 'sesi WA baru tercatat — warmup dimulai dari hari ke-1');
}

// Umur sesi dalam hari, 1-based (hari pertama = 1).
export function sessionDayIndex() {
  if (!state?.sessionStartedDay) return 1;
  const start = dayKeyToUTC(state.sessionStartedDay);
  const now = dayKeyToUTC(localDayKey());
  if (Number.isNaN(start) || Number.isNaN(now)) return 1;
  const diff = Math.floor((now - start) / 86_400_000);
  return diff < 0 ? 1 : diff + 1;
}

// Cap hari ini: ikut ramp selama masih dalam masa warmup, lalu cap normal.
export function dailyLimit() {
  const ramp = config.warmupRamp;
  const idx = sessionDayIndex();
  if (ramp.length > 0 && idx <= ramp.length) {
    return Math.min(ramp[idx - 1], config.dailyCap);
  }
  return config.dailyCap;
}

// Apakah masih boleh kirim hari ini (global).
export function checkDaily() {
  if (!state) initQuota();
  rollover();
  const limit = dailyLimit();
  const allowed = state.sentToday < limit;
  return {
    allowed,
    limit,
    sent: state.sentToday,
    remaining: Math.max(limit - state.sentToday, 0),
    warmup: config.warmupRamp.length > 0 && sessionDayIndex() <= config.warmupRamp.length,
    sessionDay: sessionDayIndex(),
    resetInMs: msUntilLocalMidnight(),
  };
}

// Apakah boleh kirim ke nomor ini sekarang.
// - allowed:true            → kirim.
// - allowed:false, waitMs>0 → boleh dikirim setelah menunggu segitu.
// - allowed:false, waitMs=0 → cap harian per penerima habis, jangan tunggu.
export function checkRecipient(phone62) {
  if (!state) initQuota();
  rollover();
  const rec = state.recipients[phone62];
  if (!rec) return { allowed: true, waitMs: 0, sentToRecipient: 0 };

  if (rec.n >= config.maxPerRecipientDay) {
    return {
      allowed: false,
      waitMs: 0,
      sentToRecipient: rec.n,
      reason: `cap per penerima tercapai (${rec.n}/${config.maxPerRecipientDay} hari ini)`,
    };
  }
  const since = Date.now() - (rec.t || 0);
  const wait = config.minRecipientIntervalMs - since;
  if (wait > 0) {
    return {
      allowed: false,
      waitMs: wait,
      sentToRecipient: rec.n,
      reason: `jeda minimum ke nomor yang sama belum lewat (${Math.round(wait / 1000)}s lagi)`,
    };
  }
  return { allowed: true, waitMs: 0, sentToRecipient: rec.n };
}

// Catat satu percobaan kirim. Sengaja dihitung per PERCOBAAN (bukan hanya yang
// sukses) — percobaan yang gagal pun sudah menyentuh server WhatsApp, jadi
// tetap harus membebani kuota. Ini arah yang aman untuk risiko ban.
export function noteSent(phone62) {
  if (!state) initQuota();
  rollover();
  state.sentToday += 1;
  const rec = state.recipients[phone62] || { n: 0, t: 0 };
  rec.n += 1;
  rec.t = Date.now();
  state.recipients[phone62] = rec;
  persist();
}

export function quotaSnapshot() {
  if (!state) initQuota();
  const d = checkDaily();
  return {
    timezone: config.timezone,
    day: state.day,
    session_day: d.sessionDay,
    warmup_active: d.warmup,
    limit_today: d.limit,
    sent_today: d.sent,
    remaining_today: d.remaining,
    recipients_today: Object.keys(state.recipients).length,
    reset_in_ms: d.resetInMs,
  };
}
