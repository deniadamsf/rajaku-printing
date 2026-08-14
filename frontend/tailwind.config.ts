import type { Config } from 'tailwindcss'

/**
 * Tailwind config — brand tokens diserap dari LOGO.png (crimson + emas + hitam
 * ribbon di atas off-white). Ini sumber tunggal warna untuk semua UI baru.
 *
 * Lihat CLAUDE.md §26 untuk panduan pakai. Ringkasnya:
 *   - `bg-brand-500`  crimson deep, CTA primary + brand accent
 *   - `bg-gold-500`   emas antique, aksen premium / divider ornamental
 *   - `text-ink-*`    foreground & warm-neutral gray scale
 *   - `bg-canvas`     off-white background hangat (bukan #FFF klinis)
 *   - `border-hairline`  alias untuk border halus (ink.200)
 *
 * Jangan pakai `rose-*`, `slate-*`, `red-*` (bawaan Tailwind) di file baru —
 * palet itu bikin drift dari tone premium minimalist yang ditetapkan §26.
 */
export default <Partial<Config>>{
  content: [
    './components/**/*.{vue,js,ts}',
    './layouts/**/*.vue',
    './pages/**/*.vue',
    './app.vue',
    './composables/**/*.{js,ts}',
  ],
  theme: {
    extend: {
      colors: {
        // Merah crimson deep — dari tulisan RAJAKU + jubah raja di logo,
        // di-desaturasi sedikit supaya tetap premium di flat UI besar.
        brand: {
          DEFAULT: '#8B1A1A',
          50:  '#FAF3F3',
          100: '#F5E5E5',
          200: '#E8B8B8',
          300: '#D98A8A',
          400: '#B84343',
          500: '#8B1A1A', // primary CTA
          600: '#6E1414', // hover / pressed
          700: '#5A0F0F',
          800: '#420909',
          900: '#2B0505',
          950: '#1A0303',
        },
        // Emas antique — dari mahkota logo, muted (bukan #FFD700 shiny yg cheap).
        // Pakai untuk aksen premium / badge VIP / divider ornamental. Jangan overuse.
        gold: {
          DEFAULT: '#B08D57',
          50:  '#FBF6EC',
          100: '#F5E9CD',
          200: '#E8D4A0',
          300: '#D6BC77',
          400: '#C9A44A',
          500: '#B08D57',
          600: '#8F7042',
          700: '#6E552F',
          800: '#4B3A20',
          900: '#2E2313',
          950: '#1A1409',
        },
        // Warm neutral gray — pengganti slate (yg biru-tint). Base dari ribbon
        // hitam logo untuk foreground, di-blend ke stone-like warm neutrals.
        ink: {
          DEFAULT: '#0A0A0A',
          50:  '#FAFAF9',
          100: '#F5F5F4',
          200: '#E7E5E4', // hairline
          300: '#D6D3D1',
          400: '#A8A29E',
          500: '#78716C', // muted text
          600: '#57534E',
          700: '#44403C',
          800: '#292524',
          900: '#171717', // body
          950: '#0A0A0A', // strong/heading
        },
        // Off-white background hangat, bukan #FFF klinis.
        canvas: {
          DEFAULT: '#FAFAF9',
          alt: '#F5F5F4',
        },
        // Alias — supaya `border-hairline` bisa dipakai lgs.
        hairline: '#E7E5E4',
      },
      fontFamily: {
        // Serif display untuk headline hero / section besar — regal, sesuai
        // tema raja di logo. WAJIB load via <link> Google Fonts di app head
        // sebelum bisa dipakai (belum di-setup — task terpisah kalau butuh).
        serif: ['"Fraunces"', '"Playfair Display"', 'ui-serif', 'Georgia', 'serif'],
        // Sans body — Inter default (belum di-link ke Google Fonts, tapi
        // fallback ui-sans-serif tampil rapi).
        sans: ['"Inter"', 'ui-sans-serif', 'system-ui', '-apple-system', 'BlinkMacSystemFont', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace'],
      },
      boxShadow: {
        // Elevated tapi tetap subtle — kombinasi dgn ring-1 ring-black/5.
        // Hindari shadow-2xl (gaudy) & colored shadow (kesan konsumer).
        card: '0 1px 2px 0 rgb(0 0 0 / 0.04), 0 0 0 1px rgb(0 0 0 / 0.04)',
      },
      screens: {
        // Konsisten dengan useDevice(): mobile < 768, tablet 768-1023, desktop >= 1024
        // (default Tailwind sudah cocok, ini eksplisit dokumentasi.)
      },
    },
  },
  plugins: [],
}
