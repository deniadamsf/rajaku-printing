/**
 * useThermalPrint — jembatan ke Print Agent lokal (`services/print-agent`)
 * untuk printer thermal EPPOS EP8081/RPP02.
 *
 * KENAPA INI ADA (jangan hapus tanpa baca): `window.print()` TIDAK BISA
 * dipakai untuk printer ini. Driver Windows-nya mengirim blok raster
 * `GS v 0` setinggi 24 baris tapi hanya memajukan kertas 21 titik — cacat di
 * jalur driver, bukan soal CSS (dibuktikan 22 Agustus 2026, lihat
 * `services/print-agent/README.md`). Agen lokal ini merender HTML struk jadi
 * gambar lalu mengirimkannya sebagai `ESC *`, yang terbukti jalan.
 *
 * `window.print()` TETAP dipakai sebagai jalur cetak untuk printer lain (mis.
 * cetak ke PDF/printer laser) dan sebagai jaring pengaman saat agen ini mati
 * — lihat pemanggil di pages/admin/pos & pages/admin/order/[resi].
 */

interface AgentHealthResponse {
  success: boolean
  data?: { status: string; com_port: string }
  error?: { code: string; message: string }
}

interface AgentPrintResponse {
  success: boolean
  data?: { width_mm: number; height_dots: number; bytes: number; ms: number }
  error?: { code: string; message: string }
}

// Cache di level modul (bukan di dalam ref komponen) — bertahan selama sesi
// halaman ini hidup di browser (sampai reload/navigasi penuh), supaya kasir
// yang cetak struk berkali-kali tidak menembak GET /health tiap kali. Nilai
// `null` berarti belum pernah dicek pada sesi ini.
let cachedAgentAvailable: boolean | null = null

export function useThermalPrint() {
  const config = useRuntimeConfig()
  const agentUrl = config.public.printAgentUrl.replace(/\/$/, '')

  /**
   * Cek apakah Print Agent lokal sedang hidup. Timeout PENDEK (1.5 detik) —
   * ini cuma probe untuk memutuskan jalur cetak, kasir tidak boleh menunggu
   * lama hanya untuk tahu agennya mati. Gagal APA PUN (timeout, agen belum
   * dijalankan, CORS, dll) dikembalikan sebagai `false` diam-diam — pemanggil
   * cukup jatuh balik ke `window.print()`, tidak perlu try/catch sendiri.
   */
  async function isAgentAvailable(): Promise<boolean> {
    if (cachedAgentAvailable !== null) return cachedAgentAvailable

    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), 1500)
    try {
      const res = await $fetch<AgentHealthResponse>(`${agentUrl}/health`, {
        method: 'GET',
        signal: controller.signal,
      })
      cachedAgentAvailable = res?.success === true
    } catch {
      cachedAgentAvailable = false
    } finally {
      clearTimeout(timer)
    }
    return cachedAgentAvailable
  }

  /**
   * Merakit HTML MANDIRI (self-contained) dari elemen `#struk` yang sedang
   * hidup di DOM. Print Agent merender ini sebagai berkas terpisah lewat
   * Chrome headless (bukan dalam konteks tab yang sedang terbuka), jadi
   * berkasnya tidak boleh bergantung pada apa pun yang ada di luar dirinya
   * sendiri.
   */
  function buildStandaloneHtml(): string {
    const struk = document.getElementById('struk')
    if (!struk) {
      throw new Error(
        'Elemen struk (#struk) tidak ditemukan di halaman. Pastikan komponen ReceiptStruk sudah ter-mount sebelum mencetak.',
      )
    }

    // Salin SEMUA <style> dan <link rel="stylesheet"> dari <head> dokumen ini
    // apa adanya. Tailwind (utility classes) dan CSS print milik ReceiptStruk
    // hidup di sana — tanpa ini struk akan terkirim tanpa styling sama sekali.
    const styleTags = Array.from(
      document.head.querySelectorAll('style, link[rel="stylesheet"]'),
    )
      .map((el) => el.outerHTML)
      .join('\n')

    // `<base href>` WAJIB jadi elemen PERTAMA di <head>. Chrome headless milik
    // agen merender berkas HTML ini dari disk (bukan dari URL asli halaman),
    // jadi path relatif di CSS/font/`<img>` (mis. `/fonts/…`, `/logo.png`)
    // tidak akan ketemu tanpa base href eksplisit ke origin aplikasi ini.
    const baseHref = `<base href="${window.location.origin}/">`

    // Di layar, #struk sengaja disembunyikan (dipakai print-only via CSS
    // media `print`) — lihat ReceiptStruk.vue. Agen merender pada media
    // `screen` (Chrome headless default), BUKAN `print`, jadi tanpa override
    // ini hasil tangkapannya adalah halaman kosong. Diletakkan PALING AKHIR
    // di <head> supaya urutan cascade CSS menang atas aturan sembunyi di atas.
    const forceVisible = '<style>#struk{display:block!important}</style>'

    return `<!doctype html>
<html lang="id">
<head>
<meta charset="utf-8">
${baseHref}
${styleTags}
${forceVisible}
</head>
<body>
${struk.outerHTML}
</body>
</html>`
  }

  /**
   * Kirim struk yang sedang ter-mount ke Print Agent. Timeout 60 detik —
   * jauh lebih longgar dari `/health` karena agen benar-benar merender
   * gambar & mengirim byte ke port COM (§ README "Alur di dalam", proses
   * nyata bisa beberapa detik).
   *
   * Melempar `Error` (bukan mengembalikan boolean) kalau gagal — pemanggil
   * WAJIB menangkapnya untuk jatuh balik ke `window.print()` dan memberi
   * tahu kasir bahwa hasil cetak agen tidak terjamin.
   */
  async function printViaAgent(widthMm: 58 | 80): Promise<void> {
    const html = buildStandaloneHtml()

    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), 60000)
    try {
      const res = await $fetch<AgentPrintResponse>(`${agentUrl}/print`, {
        method: 'POST',
        body: { html, width_mm: widthMm },
        signal: controller.signal,
      }).catch((e: unknown) => {
        // $fetch melempar untuk status non-2xx — body error envelope masih
        // ada di `e.data` (pola sama dengan useApi.ts), gali dari sana dulu
        // sebelum menyerah ke pesan generik.
        const err = e as { data?: AgentPrintResponse; message?: string }
        if (err?.data) return err.data
        throw e
      })

      if (!res?.success) {
        throw new Error(res?.error?.message || 'Print Agent menolak mencetak struk.')
      }
    } finally {
      clearTimeout(timer)
    }
  }

  return {
    agentUrl,
    isAgentAvailable,
    buildStandaloneHtml,
    printViaAgent,
  }
}
