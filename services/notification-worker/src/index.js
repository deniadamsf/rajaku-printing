// Entrypoint: bootstrap WA, start dispatcher loop, jalankan HTTP kecil
// untuk /healthz + /qr (operator scan QR dari browser kalau tidak lihat
// terminal).
import http from 'node:http';
import pino from 'pino';
import QRCode from 'qrcode-terminal';
import QRImage from 'qrcode';
import crypto from 'node:crypto';
import { config } from './config.js';
import { startWA, isReady, getLastQR, getSelfNumber, unlinkSession } from './wa.js';
import { startDispatcher, stopDispatcher } from './dispatcher.js';
import { quotaSnapshot } from './quota.js';
import { circuitSnapshot } from './circuit.js';
import { pacingSnapshot } from './pacing.js';

const log = pino({ level: config.logLevel, base: undefined });

async function main() {
  log.info({
    backendURL: config.backendURL,
    pollIntervalMs: config.pollIntervalMs,
    batchSize: config.batchSize,
    sendIntervalMs: [config.minSendIntervalMs, config.maxSendIntervalMs],
    warmupRamp: config.warmupRamp,
    dailyCap: config.dailyCap,
    timezone: config.timezone,
    stateFile: config.stateFile,
    port: config.workerPort,
  }, 'starting notification-worker');

  const server = http.createServer((req, res) => {
    if (req.method === 'GET' && req.url === '/healthz') {
      // Sertakan status kuota/circuit — operator perlu tahu kalau worker
      // sedang diam karena cap harian atau circuit terbuka, bukan karena mati.
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        ok: true,
        wa_ready: isReady(),
        quota: quotaSnapshot(),
        circuit: circuitSnapshot(),
        pacing: pacingSnapshot(),
      }));
      return;
    }
    // ---- Gerbang auth untuk endpoint yang membocorkan kredensial ----
    // QR pairing SETARA kredensial: siapa pun yang memindainya menautkan
    // WhatsApp-nya sendiri ke nomor toko dan bisa membaca/mengirim pesan
    // atas nama toko. Sebelumnya endpoint ini terbuka tanpa auth sama
    // sekali. /healthz sengaja tetap terbuka — cuma sinyal hidup/mati.
    if (req.url === '/pairing' || req.url === '/pairing/logout' || req.url === '/qr') {
      if (!hasValidSecret(req)) {
        res.writeHead(401, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ error: 'unauthorized' }));
        return;
      }
    }

    // GET /pairing — dipakai backend Go untuk halaman pairing di admin panel.
    if (req.method === 'GET' && req.url === '/pairing') {
      const qr = getLastQR();
      const done = (dataUrl) => {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
          wa_ready: isReady(),
          self_number: getSelfNumber(),
          qr_data_url: dataUrl,
          quota: quotaSnapshot(),
          circuit: circuitSnapshot(),
          pacing: pacingSnapshot(),
        }));
      };
      if (!qr) { done(null); return; }
      QRImage.toDataURL(qr, { errorCorrectionLevel: 'M', margin: 1, width: 320 })
        .then(done)
        .catch((e) => {
          log.error({ err: e.message }, 'gagal render QR jadi gambar');
          done(null);
        });
      return;
    }

    // POST /pairing/logout — putuskan pairing supaya QR baru terbit.
    if (req.method === 'POST' && req.url === '/pairing/logout') {
      unlinkSession()
        .then(() => {
          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({ ok: true }));
        })
        .catch((e) => {
          log.error({ err: e.message }, 'unlink session gagal');
          res.writeHead(500, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({ error: e.message }));
        });
      return;
    }

    if (req.method === 'GET' && req.url === '/qr') {
      const qr = getLastQR();
      if (!qr) {
        res.writeHead(200, { 'Content-Type': 'text/plain; charset=utf-8' });
        res.end(isReady() ? 'WA sudah terhubung. Tidak ada QR aktif.' : 'Menunggu QR dari Baileys…');
        return;
      }
      // ASCII QR — cukup untuk operator; tidak butuh render library extra.
      let ascii = '';
      QRCode.generate(qr, { small: true }, (out) => { ascii = out; });
      res.writeHead(200, { 'Content-Type': 'text/plain; charset=utf-8' });
      res.end(`Scan QR di WhatsApp → Perangkat Tertaut:\n\n${ascii}\n\n(refresh kalau tidak muncul — QR di-refresh otomatis oleh Baileys)`);
      return;
    }
    res.writeHead(404); res.end();
  });
  // HTTP DULUAN, baru Baileys. Urutannya sengaja begini: bootstrap Baileys
  // (fetchLatestBaileysVersion + handshake) bisa lambat atau gagal, dan
  // selama itu dulu server HTTP belum menyala sama sekali — halaman pairing
  // di admin panel jadi melaporkan "worker tidak bisa dihubungi" padahal
  // worker hidup dan sedang menyiapkan diri. Dengan urutan ini /healthz &
  // /pairing selalu menjawab; wa_ready:false yang menceritakan kondisi WA.
  server.listen(config.workerPort, () => log.info(`HTTP siap di :${config.workerPort} (GET /healthz, GET /qr)`));

  // Tanpa await — kegagalan bootstrap WA dicatat, TIDAK mematikan proses.
  // Worker yang mati total menghapus satu-satunya cara operator melihat apa
  // yang salah dari browser (§13: pairing tanpa SSH).
  startWA().catch((e) => log.error({ err: e.message, stack: e.stack }, 'bootstrap Baileys gagal — worker tetap hidup, WA belum siap'));

  // Dispatcher aman jalan sebelum WA siap: loop-nya menunggu isReady().
  startDispatcher().catch((e) => log.error({ err: e.message }, 'dispatcher crashed'));

  for (const sig of ['SIGINT', 'SIGTERM']) {
    process.on(sig, () => {
      log.info({ sig }, 'shutdown signal — stopping dispatcher');
      stopDispatcher();
      server.close(() => process.exit(0));
      // Kalau graceful hang, force-exit setelah 10s.
      setTimeout(() => process.exit(1), 10_000).unref();
    });
  }
}

main().catch((err) => {
  log.fatal({ err: err.message, stack: err.stack }, 'worker failed to start');
  process.exit(1);
});

// hasValidSecret membandingkan header X-Internal-Secret dengan secret bersama
// secara timing-safe. Header yang sama dipakai worker saat memanggil backend
// (backendClient.js) — arah sebaliknya kini ikut terlindungi.
function hasValidSecret(req) {
  const got = req.headers['x-internal-secret'];
  if (typeof got !== 'string') return false;
  const a = Buffer.from(got);
  const b = Buffer.from(config.internalSecret);
  if (a.length !== b.length) return false;
  return crypto.timingSafeEqual(a, b);
}
