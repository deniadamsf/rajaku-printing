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
