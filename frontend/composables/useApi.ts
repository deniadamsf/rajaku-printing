/**
 * useApi — wrapper $fetch dengan base URL dari runtimeConfig (spec section 2).
 * SEMUA panggilan API backend WAJIB via composable ini — jangan hardcode
 * `http://localhost:8080` di komponen.
 *
 * Auto-attach Bearer token dari cookie `rajaku_token` (SSR-safe) kalau ada.
 * Untuk kasus token bukan dari cookie sesi customer/staff biasa — mis. sesi
 * guest terbatas di halaman lacak resi publik (§ guest design upload) — panggil
 * `useApi({ token: () => guestToken.value })`. Override ini TIDAK menyentuh
 * cookie `rajaku_token`, jadi sesi customer login asli tidak pernah tertukar.
 * Semua call site lama `useApi()` tanpa argumen tetap pakai cookie seperti biasa.
 *
 * Response envelope backend: `{ success, data, error }`. Wrapper ini mengembalikan
 * `data` langsung untuk happy path & melempar ApiError dengan code/message dari
 * envelope saat gagal.
 */

export interface ApiEnvelope<T> {
  success: boolean
  data?: T
  error?: {
    code: string
    message: string
    details?: unknown
  }
}

export class ApiError extends Error {
  code: string
  status?: number
  details?: unknown

  constructor(message: string, code: string, status?: number, details?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
    this.details = details
  }
}

const TOKEN_COOKIE = 'rajaku_token'

export function useAuthTokenCookie() {
  // Cookie SSR-safe. Umur = 24h (match backend JWT_ACCESS_TTL default).
  return useCookie<string | null>(TOKEN_COOKIE, {
    maxAge: 60 * 60 * 24,
    sameSite: 'lax',
    secure: process.env.NODE_ENV === 'production',
    default: () => null,
  })
}

export interface UseApiOptions {
  /** Sumber token override (mis. sesi guest). Default: cookie `rajaku_token`. */
  token?: () => string | null | undefined
}

/**
 * humanNetworkMessage — pesan pengganti saat respons backend tidak membawa
 * envelope error. Pesan asli $fetch ("[POST] \"http://localhost:8080/…\":
 * 404 Not Found") membocorkan URL internal dan tidak terbaca pengguna.
 */
function humanNetworkMessage(status?: number): string {
  if (status === undefined) return 'Tidak bisa terhubung ke server. Cek koneksi Anda lalu coba lagi.'
  if (status === 404) return 'Layanan yang diminta tidak tersedia. Coba lagi nanti.'
  if (status === 429) return 'Terlalu banyak permintaan. Tunggu sebentar lalu coba lagi.'
  if (status >= 500) return 'Server sedang bermasalah. Coba lagi beberapa saat lagi.'
  return 'Permintaan gagal diproses. Coba lagi.'
}

export function useApi(options: UseApiOptions = {}) {
  const config = useRuntimeConfig()
  const baseURL = config.public.apiBase
  const tokenCookie = useAuthTokenCookie()
  const getToken = options.token ?? (() => tokenCookie.value)

  async function request<T>(path: string, opts: Parameters<typeof $fetch>[1] = {}): Promise<T> {
    const headers: Record<string, string> = {
      ...(opts.headers as Record<string, string> | undefined),
    }
    const token = getToken()
    if (token) {
      headers['Authorization'] = `Bearer ${token}`
    }

    try {
      const res = await $fetch<ApiEnvelope<T>>(path, {
        baseURL,
        ...opts,
        headers,
      })
      if (!res.success || res.data === undefined) {
        throw new ApiError(
          res.error?.message ?? 'API request failed',
          res.error?.code ?? 'UNKNOWN',
          undefined,
          res.error?.details,
        )
      }
      return res.data
    } catch (e: unknown) {
      // Kalau backend return non-2xx, $fetch throw dengan `data` = body envelope.
      const err = e as { data?: ApiEnvelope<unknown>; status?: number; statusCode?: number; message?: string }
      const env = err?.data
      if (env && env.error) {
        throw new ApiError(
          env.error.message,
          env.error.code,
          err.status ?? err.statusCode,
          env.error.details,
        )
      }
      if (e instanceof ApiError) throw e
      // Tidak ada envelope: 404 polos dari router, kegagalan jaringan, atau
      // error proxy. Pesan mentah $fetch memuat URL backend lengkap — jangan
      // pernah ditampilkan ke pengguna. Simpan di `details` untuk debugging,
      // tampilkan pesan yang bisa dibaca manusia.
      const status = err?.status ?? err?.statusCode
      throw new ApiError(humanNetworkMessage(status), 'NETWORK_ERROR', status, err?.message)
    }
  }

  return {
    get: <T>(path: string, opts?: Parameters<typeof $fetch>[1]) =>
      request<T>(path, { ...opts, method: 'GET' }),
    post: <T>(path: string, body?: unknown, opts?: Parameters<typeof $fetch>[1]) =>
      request<T>(path, { ...opts, method: 'POST', body: body as Record<string, unknown> | BodyInit | null | undefined }),
    put: <T>(path: string, body?: unknown, opts?: Parameters<typeof $fetch>[1]) =>
      request<T>(path, { ...opts, method: 'PUT', body: body as Record<string, unknown> | BodyInit | null | undefined }),
    patch: <T>(path: string, body?: unknown, opts?: Parameters<typeof $fetch>[1]) =>
      request<T>(path, { ...opts, method: 'PATCH', body: body as Record<string, unknown> | BodyInit | null | undefined }),
    delete: <T>(path: string, opts?: Parameters<typeof $fetch>[1]) =>
      request<T>(path, { ...opts, method: 'DELETE' }),
  }
}
