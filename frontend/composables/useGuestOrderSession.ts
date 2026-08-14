/**
 * useGuestOrderSession — sesi terbatas untuk pembeli guest (order tanpa akun)
 * membuktikan kepemilikan order di halaman lacak resi publik, lewat nomor
 * resi + nomor WA yang dipakai saat order (bukan password — guest memang
 * tidak punya).
 *
 * Endpoint: POST /lacak/:resi/verify  body { phone } → { token, expires_at }
 *
 * Token DISIMPAN DI sessionStorage (hilang saat tab ditutup), per-resi, dan
 * TIDAK PERNAH ditulis ke cookie `rajaku_token` — supaya tidak pernah
 * tertukar dengan sesi customer login asli (lihat useApi/useAuthTokenCookie).
 * Token berlaku 30 menit sesuai `expires_at` dari backend; sesudah lewat,
 * `isVerified` otomatis balik ke false (di-cek ulang tiap beberapa detik lewat
 * ticker internal, supaya tab yang dibiarkan terbuka tetap ter-update tanpa
 * perlu interaksi user).
 */
import { ApiError } from '~/composables/useApi'

interface GuestSessionPayload {
  token: string
  expires_at: string
}

function storageKey(resi: string): string {
  return `rajaku_guest_session_${resi}`
}

export function useGuestOrderSession(resi: () => string) {
  const api = useApi()

  const token = ref<string | null>(null)
  const expiresAt = ref<string | null>(null)
  const verifying = ref(false)
  const errorMsg = ref<string | null>(null)

  // Ticker ringan supaya computed expiry re-evaluate walau tidak ada
  // interaksi user (tab dibiarkan terbuka melewati batas 30 menit).
  const nowTick = ref(Date.now())
  let timer: ReturnType<typeof setInterval> | undefined

  function load() {
    if (!import.meta.client) return
    try {
      const raw = sessionStorage.getItem(storageKey(resi()))
      if (!raw) return
      const parsed = JSON.parse(raw) as GuestSessionPayload
      token.value = parsed.token
      expiresAt.value = parsed.expires_at
    } catch {
      token.value = null
      expiresAt.value = null
    }
  }

  function persist(payload: GuestSessionPayload) {
    token.value = payload.token
    expiresAt.value = payload.expires_at
    if (import.meta.client) {
      sessionStorage.setItem(storageKey(resi()), JSON.stringify(payload))
    }
  }

  function clear() {
    token.value = null
    expiresAt.value = null
    if (import.meta.client) {
      sessionStorage.removeItem(storageKey(resi()))
    }
  }

  const isVerified = computed(() => {
    void nowTick.value // dependency supaya re-evaluate tiap tick
    if (!token.value || !expiresAt.value) return false
    return new Date(expiresAt.value).getTime() > Date.now()
  })

  const expiresInMs = computed(() => {
    void nowTick.value
    if (!expiresAt.value) return 0
    return Math.max(0, new Date(expiresAt.value).getTime() - Date.now())
  })

  async function verify(phone: string): Promise<boolean> {
    verifying.value = true
    errorMsg.value = null
    try {
      const res = await api.post<GuestSessionPayload>(`/lacak/${resi()}/verify`, { phone })
      persist(res)
      return true
    } catch (e) {
      if (e instanceof ApiError) {
        errorMsg.value =
          e.status === 429
            ? 'Terlalu banyak percobaan verifikasi. Silakan tunggu beberapa saat sebelum mencoba lagi.'
            : e.message || 'Nomor WhatsApp tidak cocok dengan pesanan ini.'
      } else {
        errorMsg.value = 'Gagal memverifikasi. Coba lagi.'
      }
      return false
    } finally {
      verifying.value = false
    }
  }

  function logout() {
    clear()
  }

  onMounted(() => {
    load()
    timer = setInterval(() => {
      nowTick.value = Date.now()
    }, 15_000)
  })
  onBeforeUnmount(() => {
    if (timer) clearInterval(timer)
  })

  return {
    // Hanya expose token kalau memang masih valid — pemakai (mis. useDesign
    // token override) tidak perlu cek isVerified berulang.
    token: computed(() => (isVerified.value ? token.value : null)),
    isVerified,
    verifying,
    errorMsg,
    expiresAt: computed(() => expiresAt.value),
    expiresInMs,
    verify,
    logout,
  }
}
