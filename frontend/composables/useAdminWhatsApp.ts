/**
 * useAdminWhatsApp — API wrapper modul `notification` (pairing Baileys) untuk
 * admin panel. Endpoint di-gate permission `notification.manage` di backend
 * (GET maupun POST — lihat docblock endpoint di bawah).
 *
 *   GET  /admin/whatsapp/pairing   status pairing (worker reachable? tertaut? QR?)
 *   POST /admin/whatsapp/unlink    putus pairing supaya QR baru terbit
 *
 * Kontrak (final, backend dikerjakan paralel — JANGAN diubah dari sisi FE):
 * - `worker_reachable:false` → layanan notification-worker mati/tidak terjangkau.
 *   Endpoint tetap balas HTTP 200 untuk kasus ini (bukan error) — beda makna
 *   dari request yang benar-benar gagal (ApiError).
 * - `connected:true` → nomor sudah tertaut, `qr_data_url` biasanya null.
 * - `connected:false` + `qr_data_url` terisi → menunggu di-scan.
 * - `quota`/`circuit`/`pacing` objek bebas dari worker — tampilkan seperlunya,
 *   jangan dipaksa jadi tabel kaku (bentuknya bisa berubah tanpa breaking FE).
 */

export interface WhatsAppPairingStatus {
  worker_reachable: boolean
  connected: boolean
  self_number: string | null
  qr_data_url: string | null
  quota?: Record<string, unknown> | null
  circuit?: Record<string, unknown> | null
  pacing?: Record<string, unknown> | null
}

export function useAdminWhatsApp() {
  const api = useApi()

  function getPairing(): Promise<WhatsAppPairingStatus> {
    return api.get<WhatsAppPairingStatus>('/admin/whatsapp/pairing')
  }

  function unlink(): Promise<{ ok: boolean }> {
    return api.post<{ ok: boolean }>('/admin/whatsapp/unlink')
  }

  return { getPairing, unlink }
}
