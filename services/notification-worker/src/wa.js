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
          log.warn('session dihapus — restart worker & scan QR baru');
        } catch (e) {
          log.error({ err: e.message }, 'gagal hapus session dir');
        }
      } else {
        // Reconnect otomatis setelah delay singkat.
        setTimeout(() => { startWA().catch((e) => log.error({ err: e.message }, 'reconnect failed')); }, 5_000);
      }
    }
  });

  return sock;
}

// sendText — kirim WA plain text ke nomor 62xxx (tanpa `+` / `0`).
// Baileys wajib format JID: <number>@s.whatsapp.net.
export async function sendText(phone62, text) {
  if (!ready || !sock) {
    throw new Error('WA belum siap (belum scan QR atau koneksi putus)');
  }
  if (!/^62\d{8,13}$/.test(phone62)) {
    throw new Error(`format nomor tidak valid: ${phone62} (harus 62xxx)`);
  }
  const jid = `${phone62}@s.whatsapp.net`;
  await sock.sendMessage(jid, { text });
}
