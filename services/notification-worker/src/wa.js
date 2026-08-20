// Baileys wrapper — start socket, persist session, expose sendText().
// Kalau QR belum di-scan, worker LOG QR di terminal (qrcode-terminal) +
// simpan dataURL terakhir untuk GET /qr (kalau operator lebih suka scan
// dari browser).
import { makeWASocket, useMultiFileAuthState, DisconnectReason, fetchLatestBaileysVersion } from '@whiskeysockets/baileys';
import { Boom } from '@hapi/boom';
import qrcodeTerminal from 'qrcode-terminal';
import fs from 'node:fs';
import path from 'node:path';
import pino from 'pino';
import { config } from './config.js';
import { typingDelayMs, jitterMs } from './pacing.js';
import { bindSession } from './quota.js';

// Baileys butuh logger dengan child(). pino cocok — kita silent-kan level
// bawaan Baileys sendiri (verbose banget) tapi pertahankan level worker.
const baileysLogger = pino({ level: 'warn' });

const log = pino({ level: config.logLevel, base: undefined });

let sock = null;
let ready = false;
let lastQR = null;
let readyPromiseResolvers = [];

function markReady(isReady) {
  ready = isReady;
  if (isReady) {
    const rs = readyPromiseResolvers;
    readyPromiseResolvers = [];
    for (const r of rs) r();
  }
}

export function isReady() { return ready; }
export function getLastQR() { return lastQR; }

// unlinkSession memutus pairing WhatsApp saat ini atas permintaan admin
// (halaman pairing di admin panel). Baileys akan memancarkan connection
// 'close' dengan alasan loggedOut, yang menghapus folder session lalu
// menyalakan ulang socket — jadi QR baru langsung terbit tanpa perlu SSH
// ke server.
export async function unlinkSession() {
  if (!sock) throw new Error('socket belum aktif');
  await sock.logout();
  markReady(false);
  return true;
}

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms));
}

// Nomor yang sedang ter-pairing, format 62xxx (dari "62xxx:12@s.whatsapp.net").
export function getSelfNumber() {
  const id = sock?.user?.id;
  if (!id) return null;
  return id.split('@')[0].split(':')[0];
}

// Await first successful connection. Dipakai kalau caller mau tunggu
// worker beneran online sebelum start polling loop.
export function waitUntilReady(timeoutMs = 0) {
  if (ready) return Promise.resolve();
  return new Promise((resolve, reject) => {
    readyPromiseResolvers.push(resolve);
    if (timeoutMs > 0) {
      setTimeout(() => reject(new Error('waitUntilReady: timeout')), timeoutMs);
    }
  });
}

export async function startWA() {
  // Pastikan session dir ada — Baileys tulis credential JSON di sini.
  if (!fs.existsSync(config.sessionDir)) {
    fs.mkdirSync(config.sessionDir, { recursive: true });
  }
  const { state, saveCreds } = await useMultiFileAuthState(config.sessionDir);
  const { version } = await fetchLatestBaileysVersion();
  log.info({ waVersion: version }, 'starting Baileys socket');

  sock = makeWASocket({
    version,
    auth: state,
    logger: baileysLogger,
    // Perilaku aman: browser identity fixed biar server WA tidak nge-flag
    // sebagai perangkat baru tiap restart.
    browser: ['Rajaku Printing', 'Chrome', '1.0.0'],
    // printQRInTerminal deprecated di Baileys 6.7+; kita handle sendiri
    // via connection.update event.
  });

  sock.ev.on('creds.update', saveCreds);

  sock.ev.on('connection.update', (u) => {
    const { connection, lastDisconnect, qr } = u;
    if (qr) {
      lastQR = qr;
      log.warn('scan QR di WhatsApp → Perangkat Tertaut untuk pairing:');
      qrcodeTerminal.generate(qr, { small: true });
    }
    if (connection === 'open') {
      log.info('WA connected — worker siap kirim');
      lastQR = null;
      // Anchor warmup ke nomor yang ter-pairing: kalau nomornya berganti,
      // quota.js otomatis mulai ramp dari hari ke-1 lagi.
      bindSession(getSelfNumber());
      markReady(true);
    } else if (connection === 'close') {
      markReady(false);
      const reason = new Boom(lastDisconnect?.error)?.output?.statusCode;
      const loggedOut = reason === DisconnectReason.loggedOut;
      log.warn({ reason, loggedOut }, 'WA disconnected');
      if (loggedOut) {
        // Session invalid — hapus supaya next start memicu pairing ulang.
        try {
          for (const f of fs.readdirSync(config.sessionDir)) {
            fs.rmSync(path.join(config.sessionDir, f), { recursive: true, force: true });
          }
          log.warn('session dihapus — menyalakan ulang socket untuk QR baru');
        } catch (e) {
          log.error({ err: e.message }, 'gagal hapus session dir');
        }
        // Nyalakan ulang supaya QR baru terbit sendiri. Tanpa ini, operator
        // wajib restart worker manual — yang membuat halaman pairing di
        // admin panel tidak ada gunanya untuk ganti nomor.
        setTimeout(() => { startWA().catch((e) => log.error({ err: e.message }, 'restart setelah logout gagal')); }, 2_000);
      } else {
        // Reconnect otomatis setelah delay singkat.
        setTimeout(() => { startWA().catch((e) => log.error({ err: e.message }, 'reconnect failed')); }, 5_000);
      }
    }
  });

  return sock;
}

// --- Cek nomor terdaftar di WhatsApp -----------------------------------
// Mengirim ke nomor yang tidak punya WhatsApp adalah sinyal spam yang kuat
// (§13). Hasil di-cache in-memory dengan TTL supaya nomor yang sama tidak
// di-query berulang — query onWhatsApp sendiri juga termasuk traffic.
const numberCache = new Map(); // phone62 -> { registered: boolean, ts: number }

function cacheGet(phone62) {
  const hit = numberCache.get(phone62);
  if (!hit) return undefined;
  if (Date.now() - hit.ts > config.numberCacheTtlMs) {
    numberCache.delete(phone62);
    return undefined;
  }
  return hit.registered;
}

function cacheSet(phone62, registered) {
  // Prune kasar supaya map tidak tumbuh selamanya di proses long-running.
  if (numberCache.size > 5000) {
    for (const [k, v] of numberCache) {
      if (Date.now() - v.ts > config.numberCacheTtlMs) numberCache.delete(k);
    }
    if (numberCache.size > 5000) numberCache.clear();
  }
  numberCache.set(phone62, { registered, ts: Date.now() });
}

// Return true = terdaftar, false = TIDAK terdaftar (jangan kirim),
// null = tidak diketahui (fitur dimatikan / query gagal) → caller boleh kirim,
// karena error infrastruktur tidak boleh jadi kegagalan permanen.
export async function isRegisteredOnWhatsApp(phone62) {
  if (!config.checkRegistered) return null;
  const cached = cacheGet(phone62);
  if (cached !== undefined) return cached;
  if (!ready || !sock) return null;

  const jid = `${phone62}@s.whatsapp.net`;
  try {
    const res = await sock.onWhatsApp(jid);
    const entry = Array.isArray(res) ? res[0] : undefined;
    const registered = Boolean(entry?.exists);
    cacheSet(phone62, registered);
    return registered;
  } catch (err) {
    log.warn({ phone: phone62, err: err?.message },
      'cek onWhatsApp gagal — lanjut kirim (tidak dianggap nomor mati)');
    return null;
  }
}

// sendText — kirim WA plain text ke nomor 62xxx (tanpa `+` / `0`).
// Baileys wajib format JID: <number>@s.whatsapp.net.
//
// `text` dikirim APA ADANYA — penyusunan/variasi teks ada di backend Go
// (kolom notification_jobs.message); worker tidak boleh menormalkan isinya.
export async function sendText(phone62, text) {
  if (!ready || !sock) {
    throw new Error('WA belum siap (belum scan QR atau koneksi putus)');
  }
  if (!/^62\d{8,13}$/.test(phone62)) {
    throw new Error(`format nomor tidak valid: ${phone62} (harus 62xxx)`);
  }
  const jid = `${phone62}@s.whatsapp.net`;

  // Pola manusia: subscribe presence → "sedang mengetik" selama proporsional
  // panjang pesan → "paused" → baru kirim. Kegagalan presence TIDAK boleh
  // membatalkan pengiriman (cuma kosmetik perilaku).
  if (config.simulateTyping) {
    try {
      await sock.presenceSubscribe(jid);
      await sleep(jitterMs(200, 600));
      await sock.sendPresenceUpdate('composing', jid);
      await sleep(typingDelayMs(text));
      await sock.sendPresenceUpdate('paused', jid);
    } catch (err) {
      log.warn({ phone: phone62, err: err?.message }, 'presence/typing gagal — lanjut kirim');
    }
  }

  await sock.sendMessage(jid, { text });
}
