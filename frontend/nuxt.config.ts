// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: '2025-01-01',
  // Tombol melayang DevTools mengganggu saat menata tampilan. Ini hanya
  // pernah muncul di mode dev — build produksi tidak pernah memuatnya, jadi
  // mematikannya murni soal kenyamanan kerja, bukan keamanan.
  devtools: { enabled: false },

  modules: [
    '@nuxtjs/tailwindcss',
    '@pinia/nuxt',
    '@vueuse/nuxt',
    '@nuxtjs/google-fonts',
    '@tresjs/nuxt',
    // Menyediakan `eslint.config.mjs` yang sadar auto-import Nuxt (useHead,
    // ref, computed, dst tidak dilaporkan sebagai undefined) — lihat
    // eslint.config.mjs di root frontend.
    '@nuxt/eslint',
  ],

  // TresJS devtools tab hanya berguna waktu dev; matikan di build produksi
  // untuk bundle sedikit lebih ramping. Modul TresJS sendiri hanya dipakai di
  // /showcase (§16 poin 2) — tidak pernah di-import di homepage.
  tres: {
    devtools: false,
  },

  // Font pairing CLAUDE.md §26.3 — Fraunces (serif display, editorial), Inter
  // (sans body & UI), JetBrains Mono (data teknis / ID / slug).
  // Module ini otomatis self-host (download saat build, no runtime hit ke Google).
  googleFonts: {
    display: 'swap',
    preconnect: true,
    families: {
      Fraunces: {
        wght: [500, 600, 700],
        // Optical size axis — Fraunces variable, atur untuk display size.
        opsz: '9..144',
      },
      Inter: [400, 500, 600, 700],
      'JetBrains Mono': [400, 500],
    },
  },

  // Runtime config — SATU SUMBER untuk semua base URL (spec section 2).
  // Public keys diekspos ke client; non-public keys hanya di server.
  runtimeConfig: {
    // server-only
    appEnv: process.env.NUXT_APP_ENV || 'development',

    public: {
      apiBase: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080/api/v1',
      appBaseUrl: process.env.NUXT_PUBLIC_APP_BASE_URL || 'http://localhost:3000',
    },
  },

  // SSR/SSG untuk SEO (spec section 15).
  ssr: true,

  app: {
    // Transisi antar halaman — fade singkat (§26.6: 150-250ms ease-out).
    // Definisi kelas `.page-*` ada di assets/css/tailwind.css, termasuk override
    // `prefers-reduced-motion` (transisi dimatikan total, bukan cuma dipercepat).
    // Nuxt hanya memakai ini untuk navigasi client-side setelah hydration —
    // first paint SSR tidak pernah terbungkus transisi, jadi tidak ada risiko
    // konten "mulai dari opacity 0" di initial load (§15 SEO).
    pageTransition: { name: 'page', mode: 'out-in' },
    head: {
      htmlAttrs: { lang: 'id' },
      // Dijalankan SEBELUM body dicat: menandai bahwa JS hidup, sehingga
      // jaring pengaman `html:not(.motion-ready)` di tailwind.css berhenti
      // berlaku dan animasi reveal berjalan normal tanpa kedipan.
      // Watchdog melepas kelasnya lagi kalau hydration tidak pernah selesai
      // dalam 6 detik — menutup kasus bundle gagal termuat, bukan cuma
      // kasus JS dimatikan.
      script: [
        {
          innerHTML:
            "(function(){var d=document.documentElement;d.classList.add('motion-ready');" +
            "setTimeout(function(){if(!window.__rjkHydrated){d.classList.remove('motion-ready')}},6000)})()",
        },
      ],
      meta: [
        { charset: 'utf-8' },
        { name: 'viewport', content: 'width=device-width, initial-scale=1' },
        { name: 'theme-color', content: '#8B1A1A' },
      ],
      // Favicon dari maskot logo (kepala raja bermahkota). Sengaja dipotong ke
      // kepala saja: logo penuh beserta wordmark tidak terbaca di 16-32px.
      // PNG didahulukan daripada favicon.svg (monogram R) supaya brand asli
      // yang tampil di tab. favicon.svg tetap ada, dipakai sebagai aksen di UI.
      link: [
        { rel: 'icon', type: 'image/png', sizes: '32x32', href: '/favicon-32.png' },
        { rel: 'icon', type: 'image/png', sizes: '192x192', href: '/favicon-192.png' },
        { rel: 'apple-touch-icon', sizes: '180x180', href: '/apple-touch-icon.png' },
      ],
    },
  },

  typescript: {
    strict: true,
    typeCheck: false, // aktifkan di CI (npm run typecheck), bukan tiap dev restart
  },
})
