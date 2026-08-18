/**
 * useAdminSiteMedia — API wrapper modul `sitemedia` untuk admin panel.
 * Endpoint di-gate permission `sitemedia.manage` di backend.
 *
 *   GET    /admin/site-media         daftar SEMUA slot registry + nilai terisi
 *   POST   /admin/site-media/:slot   ganti gambar slot (multipart, field "file")
 *   DELETE /admin/site-media/:slot   kosongkan slot (kembali ke aset statis)
 *
 * Catatan: response POST cuma berisi baris `site_media` mentah (tanpa field
 * `url` siap pakai — backend membangunnya on-the-fly dari BaseURL saat list).
 * Supaya frontend tidak menduplikasi logika pembentukan URL, caller WAJIB
 * refetch `list()` setelah upload/delete berhasil, bukan menambal item lokal
 * dari response upload.
 */

export interface AdminSiteMediaItem {
  slot: string
  label: string
  description: string
  suggested_width_px: number
  suggested_height_px: number

  url: string | null
  original_name: string | null
  mime_type: string | null
  size_bytes: number | null
  width_px: number | null
  height_px: number | null
  uploaded_by: string | null
  uploaded_at: string | null
  updated_at: string | null
}

export function useAdminSiteMedia() {
  const api = useApi()

  function list(): Promise<{ items: AdminSiteMediaItem[] }> {
    return api.get<{ items: AdminSiteMediaItem[] }>('/admin/site-media')
  }

  function upload(slot: string, file: File): Promise<unknown> {
    const form = new FormData()
    form.append('file', file)
    return api.post(`/admin/site-media/${slot}`, form)
  }

  function remove(slot: string): Promise<{ ok: boolean }> {
    return api.delete<{ ok: boolean }>(`/admin/site-media/${slot}`)
  }

  return { list, upload, remove }
}
