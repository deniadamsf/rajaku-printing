/**
 * useApi — wrapper $fetch dengan base URL dari runtimeConfig (spec section 2).
 * SEMUA panggilan API backend WAJIB via composable ini — jangan hardcode
 * `http://localhost:8080` di komponen.
 *
 * Auto-attach Bearer token dari cookie `rajaku_token` (SSR-safe) kalau ada.
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

export function useApi() {
  const config = useRuntimeConfig()
  const baseURL = config.public.apiBase
  const tokenCookie = useAuthTokenCookie()

  async function request<T>(path: string, opts: Parameters<typeof $fetch>[1] = {}): Promise<T> {
    const headers: Record<string, string> = {
      ...(opts.headers as Record<string, string> | undefined),
    }
    if (tokenCookie.value) {
      headers['Authorization'] = `Bearer ${tokenCookie.value}`
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
      throw new ApiError(err?.message ?? 'Network error', 'NETWORK_ERROR', err?.status ?? err?.statusCode)
    }
  }

  return {
    get: <T>(path: string, opts?: Parameters<typeof $fetch>[1]) =>
      request<T>(path, { ...opts, method: 'GET' }),
    post: <T>(path: string, body?: unknown, opts?: Parameters<typeof $fetch>[1]) =>
      request<T>(path, { ...opts, method: 'POST', body }),
    put: <T>(path: string, body?: unknown, opts?: Parameters<typeof $fetch>[1]) =>
      request<T>(path, { ...opts, method: 'PUT', body }),
    patch: <T>(path: string, body?: unknown, opts?: Parameters<typeof $fetch>[1]) =>
      request<T>(path, { ...opts, method: 'PATCH', body }),
    delete: <T>(path: string, opts?: Parameters<typeof $fetch>[1]) =>
      request<T>(path, { ...opts, method: 'DELETE' }),
  }
}
