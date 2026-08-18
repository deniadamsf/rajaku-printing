/**
 * useSiteMedia — jembatan landing page ↔ modul `sitemedia` (admin bisa ganti
 * gambar landing tanpa deploy ulang).
 *
 * Sumber tunggal pemetaan slot → path file statis bawaan (SITE_MEDIA_FALLBACKS).
 * Tambah slot baru cukup satu baris di sini — samakan `slot` dengan
 * `backend/internal/sitemedia/model/slot_registry.go`.
 *
 * Kontrak: `GET /site-media` (publik) balas peta `{slot: url}` HANYA untuk
 * slot yang sudah diisi admin. Wajib dipakai lewat `useAsyncData` (bukan
 * `onMounted`) supaya gambar ikut ter-render di HTML SSR (§15 SEO).
 *
 * Cadangan statis WAJIB tetap jalan kalau slot kosong, request gagal, atau
 * backend mati — kegagalan fetch di sini SELALU ditangani sebagai "pakai
 * bawaan", tidak pernah dilempar sebagai error yang menggagalkan render
 * halaman. Pola: `resolve(slot) → url dari API ?? path statis`.
 */
export type SiteMediaMap = Record<string, string>

/** slot → path file statis di /public. Satu-satunya tempat pemetaan ini hidup. */
export const SITE_MEDIA_FALLBACKS: Readonly<Record<string, string>> = {
  hero_poster_desktop: '/hero/poster.webp',
  hero_poster_mobile: '/hero/poster-mobile.webp',

  brand_logo_full: '/brand/logo-full.webp',
  brand_logo_full_sm: '/brand/logo-full-sm.webp',
  brand_logo_mark: '/brand/logo-mark.webp',

  proses_1: '/proses/proses-01.webp',
  proses_2: '/proses/proses-02.webp',
  proses_3: '/proses/proses-03.webp',
  proses_4: '/proses/proses-04.webp',
  proses_5: '/proses/proses-05.webp',
  proses_6: '/proses/proses-06.webp',

  og_image: '/og-image.jpg',
}

/**
 * useSiteMedia — fetch peta site-media saat SSR (key sama di semua pemanggil
 * supaya Nuxt dedupe jadi satu request per navigasi, bukan berkali-kali per
 * komponen yang butuh gambar berbeda — lihat docs Nuxt `useAsyncData` dedupe
 * by key).
 *
 * Selalu aman dipanggil sinkron (tidak melempar, tidak butuh `<Suspense>`).
 * Untuk komponen yang WAJIB SSR-blocking (dirender langsung di HTML awal,
 * bukan di dalam `<ClientOnly>`) — supaya crawler & first paint dapat gambar
 * final, bukan fallback statis yang lalu "berkedip" ganti — `await` properti
 * `ready` di top-level `<script setup>` (pola sama dengan
 * `await useAsyncData(...)` yang sudah dipakai di
 * `components/landing/ServicesSection.vue`/`ProductSlider.vue`):
 *
 *   const { resolve, ready } = useSiteMedia()
 *   await ready
 *
 * `useAsyncData` di Nuxt mengembalikan objek yang juga "thenable" (resolve ke
 * dirinya sendiri setelah fetch awal selesai) — `ready` di sini membungkusnya
 * jadi Promise biasa, caller tidak perlu tahu detail itu.
 */
export function useSiteMedia() {
  const api = useApi()

  const asyncData = useAsyncData<SiteMediaMap>(
    'site-media-map',
    async () => {
      try {
        return await api.get<SiteMediaMap>('/site-media')
      } catch {
        // Backend mati/gagal → peta kosong, semua slot jatuh ke fallback statis.
        // Sengaja tidak melempar error: kegagalan endpoint ini tidak boleh
        // menggagalkan render landing page.
        return {}
      }
    },
    { default: () => ({} as SiteMediaMap) },
  )

  /** url dari API kalau slot sudah diisi admin, else path statis bawaan. */
  function resolve(slot: string): string {
    return asyncData.data.value?.[slot] || SITE_MEDIA_FALLBACKS[slot] || ''
  }

  return {
    media: asyncData.data,
    resolve,
    ready: Promise.resolve(asyncData).then(() => undefined),
  }
}
