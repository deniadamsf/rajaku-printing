// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: '2025-01-01',
  devtools: { enabled: true },

  modules: [
    '@nuxtjs/tailwindcss',
    '@pinia/nuxt',
    '@vueuse/nuxt',
    '@nuxtjs/google-fonts',
    '@tresjs/nuxt',
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
    head: {
      htmlAttrs: { lang: 'id' },
      meta: [
        { charset: 'utf-8' },
        { name: 'viewport', content: 'width=device-width, initial-scale=1' },
        { name: 'theme-color', content: '#8B1A1A' },
      ],
      link: [{ rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' }],
    },
  },

  typescript: {
    strict: true,
    typeCheck: false, // aktifkan di CI (npm run typecheck), bukan tiap dev restart
  },
})
