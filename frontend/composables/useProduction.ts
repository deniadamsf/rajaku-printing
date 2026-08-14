/**
 * useProduction — API wrapper untuk modul production (§4 state machine
 * production-side). Semua endpoint POST tanpa payload wajib kecuali
 * markShipped yang butuh courier + tracking.
 */

interface MarkShippedBody {
  courier: string
  tracking_number: string
  note?: string
}

export function useProduction() {
  const api = useApi()

  // Helper: kirim {note} kalau ada supaya backend log audit.
  function noteBody(note?: string): Record<string, string> | undefined {
    return note && note.trim() ? { note: note.trim() } : undefined
  }

  function startCetak(resi: string, note?: string): Promise<{ ok: boolean }> {
    return api.post<{ ok: boolean }>(`/admin/orders/${resi}/production/start-cetak`, noteBody(note))
  }

  function startQC(resi: string, note?: string): Promise<{ ok: boolean }> {
    return api.post<{ ok: boolean }>(`/admin/orders/${resi}/production/start-qc`, noteBody(note))
  }

  function markSiap(resi: string, note?: string): Promise<{ ok: boolean }> {
    return api.post<{ ok: boolean }>(`/admin/orders/${resi}/production/mark-siap`, noteBody(note))
  }

  function markShipped(resi: string, body: MarkShippedBody): Promise<{ ok: boolean }> {
    return api.post<{ ok: boolean }>(`/admin/orders/${resi}/production/mark-shipped`, body)
  }

  function markSelesai(resi: string, note?: string): Promise<{ ok: boolean }> {
    return api.post<{ ok: boolean }>(`/admin/orders/${resi}/production/mark-selesai`, noteBody(note))
  }

  return { startCetak, startQC, markSiap, markShipped, markSelesai }
}
