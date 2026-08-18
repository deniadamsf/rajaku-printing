/**
 * phone.ts — validasi ringan nomor WhatsApp Indonesia di sisi client.
 * Backend tetap sumber kebenaran untuk normalisasi ke format `62xxx` (§13) —
 * util ini cuma cegah submit nomor yang jelas-jelas bukan nomor HP sebelum
 * request dikirim ke server.
 *
 * Terima: `08xxxxxxxxx`, `62xxxxxxxxx`, `+62xxxxxxxxx` (spasi/strip diabaikan).
 */
const PHONE_PATTERN = /^(\+62|62|0)8\d{8,11}$/

export function isValidIndonesianPhone(raw: string): boolean {
  const cleaned = raw.trim().replace(/[\s-]/g, '')
  return PHONE_PATTERN.test(cleaned)
}

/**
 * Sensor tampilan nomor buat UI (mis. "Kode terkirim ke 0812****678") —
 * murni kosmetik client-side (user memang baru saja mengetik nomor ini
 * sendiri), beda dari sensor §5 di backend untuk data punya orang lain.
 * Nomor pendek/tidak valid dikembalikan apa adanya (tidak ada yang perlu
 * disembunyikan kalau formatnya sudah aneh).
 */
export function maskPhoneDisplay(raw: string): string {
  const cleaned = raw.trim().replace(/[\s-]/g, '')
  if (cleaned.length < 8) return cleaned
  const head = cleaned.slice(0, 4)
  const tail = cleaned.slice(-3)
  return `${head}${'*'.repeat(Math.max(4, cleaned.length - head.length - tail.length))}${tail}`
}
