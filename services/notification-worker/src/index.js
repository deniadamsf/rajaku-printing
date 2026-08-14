// Entrypoint: bootstrap WA, start dispatcher loop, jalankan HTTP kecil
// untuk /healthz + /qr (operator scan QR dari browser kalau tidak lihat
// terminal).
import http from 'node:http';
import pino from 'pino';
import QRCode from 'qrcode-terminal';
import { config } from './config.js';
import { startWA, isReady, getLastQR } from './wa.js';
import { startDispatcher, stopDispatcher } from './dispatcher.js';

const log = pino({ level: config.logLevel, base: undefined });

async function main() {
  log.info({
    backendURL: config.backendURL,
    pollIntervalMs: config.pollIntervalMs,
    batchSize: config.batchSize,
    minSendIntervalMs: config.minSendIntervalMs,
    port: config.workerPort,
  }, 'starting notification-worker');

  await startWA();
  // Jalankan dispatcher loop tanpa await — biar HTTP server ikut naik.
  startDispatcher().catch((e) => log.error({ err: e.message }, 'dispatcher crashed'));

  const server = http.createServer((req, res) => {
    if (req.method === 'GET' && req.url === '/healthz') {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ ok: true, wa_ready: isReady() }));
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
  server.listen(config.workerPort, () => log.info(`HTTP siap di :${config.workerPort} (GET /healthz, GET /qr)`));

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
