/**
 * useDesign — API wrapper untuk modul design (§6).
 *
 * Endpoint yg dipakai admin panel:
 *   GET  /orders/:resi/design-files             — list file (auth cukup, ownership di service)
 *   POST /admin/orders/:resi/design-drafts      — staff upload draft (multipart) — perm design.work
 *   POST /admin/orders/:resi/design-verify      — verifikasi customer_upload — perm design.approve
 *   POST /admin/orders/:resi/design-walkin-approve — POS instant approve (§11) — perm design.approve
 *   GET  /design-files/:id/file                 — stream (auth req, Bearer header) — dipakai fetchFileBlob
 */
import type { DesignFile, DesignFileList } from '~/types/design'

export function useDesign() {
  const api = useApi()
  const config = useRuntimeConfig()
  const tokenCookie = useAuthTokenCookie()

  function listByResi(resi: string): Promise<DesignFileList> {
    return api.get<DesignFileList>(`/orders/${resi}/design-files`)
  }

  function verifyUpload(resi: string, note?: string): Promise<{ ok: boolean }> {
    return api.post<{ ok: boolean }>(`/admin/orders/${resi}/design-verify`, note ? { note } : undefined)
  }

  function walkinApprove(resi: string, note?: string): Promise<{ ok: boolean }> {
    return api.post<{ ok: boolean }>(`/admin/orders/${resi}/design-walkin-approve`, note ? { note } : undefined)
  }

  // Customer-side actions on a staff draft.
  function approveDraft(draftId: string): Promise<{ ok: boolean }> {
    return api.post<{ ok: boolean }>(`/design-drafts/${draftId}/approve`)
  }

  function requestRevision(draftId: string, notes: string): Promise<{ ok: boolean }> {
    return api.post<{ ok: boolean }>(`/design-drafts/${draftId}/revision`, { notes })
  }

  async function uploadDraft(resi: string, file: File, notes?: string): Promise<DesignFile> {
    const form = new FormData()
    form.append('file', file)
    if (notes) form.append('notes', notes)
    return api.post<DesignFile>(`/admin/orders/${resi}/design-drafts`, form)
  }

  /**
   * fetchFileBlob — GET file via Bearer header, return object URL untuk render
   * di <img>/<object> tanpa expose token di URL. Caller wajib URL.revokeObjectURL
   * saat unmount / ganti file supaya tidak leak memory.
   */
  async function fetchFileBlob(id: string): Promise<{ url: string; mime: string; name: string }> {
    const res = await fetch(`${config.public.apiBase}/design-files/${id}/file`, {
      headers: tokenCookie.value ? { Authorization: `Bearer ${tokenCookie.value}` } : {},
    })
    if (!res.ok) throw new Error(`Fetch file gagal: ${res.status}`)
    const blob = await res.blob()
    return {
      url: URL.createObjectURL(blob),
      mime: res.headers.get('content-type') || blob.type || 'application/octet-stream',
      // Filename dari Content-Disposition header kalau ada; fallback id.
      name: parseContentDispositionFilename(res.headers.get('content-disposition')) || id,
    }
  }

  return {
    listByResi,
    verifyUpload,
    walkinApprove,
    approveDraft,
    requestRevision,
    uploadDraft,
    fetchFileBlob,
  }
}

function parseContentDispositionFilename(header: string | null): string {
  if (!header) return ''
  // "inline; filename=\"foo.png\"" → foo.png
  const m = /filename\*?=(?:UTF-8'')?"?([^";]+)"?/i.exec(header)
  return m?.[1] ? decodeURIComponent(m[1]) : ''
}
