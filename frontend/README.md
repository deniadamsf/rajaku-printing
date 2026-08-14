# Frontend — Rajaku Printing

Nuxt 3 (SSR/SSG) — spec: [`../CLAUDE.md`](../CLAUDE.md).

## Dev

```bash
cp .env.example .env
npm install
npm run dev   # http://localhost:3000
```

## Aturan singkat

- Base URL API **hanya** dari `useRuntimeConfig().public.apiBase` — jangan hardcode.
- API call via `useApi()` composable (lihat `composables/useApi.ts`), bukan `$fetch` langsung.
- Untuk komponen yang mahal (TresJS, GSAP scroll-scrub), gunakan `useDevice()` untuk conditional render di level pohon komponen (bukan `display:none` di CSS) — spec section 18.
- Animasi microinteraction: **motion-v** (import `motion-v`). GSAP ScrollTrigger hanya untuk hero cinematic scroll-scrub desktop — spec section 16.
