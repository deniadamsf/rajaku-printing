// Konfigurasi ESLint frontend (flat config, ESLint 9).
//
// Basisnya `.nuxt/eslint.config.mjs` yang digenerate modul @nuxt/eslint saat
// `nuxt prepare` — itu yang membuat auto-import Nuxt (ref, computed, useHead,
// useRuntimeConfig, definePageMeta, dst) dikenali, sehingga tidak dilaporkan
// sebagai `no-undef`. Jangan tulis daftar global manual di sini; kalau ada
// auto-import yang tidak dikenali, jalankan `npm run postinstall` dulu.
import withNuxt from './.nuxt/eslint.config.mjs'

export default withNuxt({
  rules: {
    // Nuxt 3 memakai konvensi nama file/route sebagai identitas komponen, dan
    // banyak halaman memang bernama satu kata (index, order, katalog,
    // login). Aturan bawaan vue ini menuntut nama multi-kata dan akan
    // menandai hampir semua halaman — tidak berguna di sini.
    'vue/multi-word-component-names': 'off',

    // Variabel/argumen yang sengaja tidak dipakai boleh, ASAL diawali "_" —
    // penanda eksplisit "ini memang dibuang", bukan sisa kode yang terlupa.
    '@typescript-eslint/no-unused-vars': ['error', {
      argsIgnorePattern: '^_',
      varsIgnorePattern: '^_',
      caughtErrorsIgnorePattern: '^_',
    }],
  },
})
