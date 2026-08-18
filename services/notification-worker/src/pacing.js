// Pacing pengiriman — jeda ACAK antar pesan + "istirahat" berkala.
//
// Kenapa: jeda tetap persis N ms adalah tanda tangan bot yang gampang
// di-fingerprint WhatsApp (§13 — Baileys tidak resmi, nomor bisa banned).
// Manusia tidak mengirim tiap 3000 ms presisi, dan sesekali berhenti lama.
//
// Modul ini murni soal WAKTU. Kuota harian/per-penerima ada di quota.js,
// perlindungan saat error beruntun ada di circuit.js.
import pino from 'pino';
import { config } from './config.js';

const log = pino({ level: config.logLevel, base: undefined });

let lastSendAt = 0;
let sentSinceRest = 0;
let totalRests = 0;

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms));
}

// Acak dalam [min, max] dengan distribusi segitiga (rata-rata dua uniform)
// sehingga nilai menumpuk di tengah seperti perilaku manusia — bukan uniform
// kaku yang tetap terlihat sintetis kalau di-plot. ~15% kemungkinan "tail"
// panjang biar polanya tidak pernah stabil di satu rentang sempit.
export function jitterMs(min, max) {
  if (max <= min) return min;
  const span = max - min;
  let r = (Math.random() + Math.random()) / 2;
  if (Math.random() < 0.15) r = 0.6 + Math.random() * 0.4;
  return Math.round(min + span * r);
}

// Lama "mengetik" proporsional panjang pesan, di-jitter per karakter lalu
// di-clamp supaya pesan panjang tidak bikin worker diam kelamaan.
export function typingDelayMs(text) {
  const len = (text || '').length;
  const perChar = jitterMs(config.typingMsPerCharMin, config.typingMsPerCharMax);
  const raw = len * perChar;
  if (raw < config.typingMinMs) return config.typingMinMs;
  if (raw > config.typingMaxMs) return config.typingMaxMs;
  return Math.round(raw);
}

// Tunggu sampai boleh kirim pesan berikutnya. Dipanggil tepat sebelum
// sendText(); pemanggil wajib memanggil noteSent() setelah percobaan kirim.
export async function waitBeforeSend() {
  let gap;
  let resting = false;
  if (config.burstPauseEvery > 0 && sentSinceRest >= config.burstPauseEvery) {
    gap = jitterMs(config.burstPauseMs, config.burstPauseMaxMs);
    resting = true;
    sentSinceRest = 0;
    totalRests += 1;
  } else {
    gap = jitterMs(config.minSendIntervalMs, config.maxSendIntervalMs);
  }

  // lastSendAt === 0 (belum pernah kirim sejak start) → tidak perlu menunggu.
  const wait = lastSendAt === 0 ? 0 : gap - (Date.now() - lastSendAt);
  if (resting) {
    log.info({ pauseMs: Math.max(wait, 0), afterMessages: config.burstPauseEvery, totalRests },
      'burst pause — istirahat sejenak biar pola kirim tidak seperti bot');
  }
  if (wait > 0) await sleep(wait);
}

// Catat bahwa satu percobaan kirim baru saja terjadi. Dipanggil BAIK saat
// sukses MAUPUN gagal — yang penting kita sudah menyentuh server WhatsApp,
// jadi jeda berikutnya tetap harus dihormati.
export function noteSent() {
  lastSendAt = Date.now();
  sentSinceRest += 1;
}

export function pacingSnapshot() {
  return {
    last_send_at: lastSendAt ? new Date(lastSendAt).toISOString() : null,
    sent_since_rest: sentSinceRest,
    burst_pause_every: config.burstPauseEvery,
    total_rests: totalRests,
  };
}
