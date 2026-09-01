<script setup lang="ts">
/**
 * /admin/design-system — Living reference untuk CLAUDE.md §26.
 * Semua token diambil dari tailwind.config.ts (di-hardcode di sini karena
 * Tailwind config tidak runtime-introspectable; kalau config berubah,
 * update halaman ini juga — flow §26.11).
 */
import {
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CreditCard,
  Crown,
  ImageOff as ImageOffIcon,
  Package,
  Palette as PaletteIcon,
  Plus,
  RotateCcw,
  Sparkles,
  Trash2,
  Upload,
} from '@lucide/vue'
import { motion } from 'motion-v'
import type { OrderItem } from '~/types/order'

// Demo sorotan kursor (lihat section #spotlight).
const { onPointerMove } = useSpotlight()

// Demo baris keranjang dapat dilipat (§32, /admin/pos) — lihat section #collapsible-cart-row.
const demoCartRowCollapsed = ref(false)

// Demo tabel item pesanan (§32 Order Multi-Item) — lihat section #order-items-table.
const demoOrderItems: OrderItem[] = [
  {
    id: 'demo-item-1',
    line_no: 1,
    product_name: 'Banner Flexi 280gsm',
    material_name: 'Flexi China',
    pricing_type: 'per_m2',
    width_cm: 300,
    height_cm: 100,
    quantity: 1,
    unit_price: 165000,
    subtotal: 165000,
    discount_amount: 0,
    design_source: 'upload',
  },
  {
    id: 'demo-item-2',
    line_no: 2,
    product_name: 'X-Banner 60x160',
    material_name: 'Albatros',
    pricing_type: 'paket',
    width_cm: 60,
    height_cm: 160,
    quantity: 2,
    unit_price: 85000,
    subtotal: 170000,
    discount_amount: 0,
    design_source: 'request',
    design_brief: 'Warna dominan emas, logo di kiri atas.',
  },
]


definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Design System — Rajaku Admin' })

interface Swatch {
  name: string
  hex: string
  className: string
  /** true kalau text di atasnya harus canvas (bg gelap). */
  darkFg?: boolean
  /** deskripsi peran singkat. */
  note?: string
}

const brand: Swatch[] = [
  { name: 'brand.50',  hex: '#FAF3F3', className: 'bg-brand-50' },
  { name: 'brand.100', hex: '#F5E5E5', className: 'bg-brand-100' },
  { name: 'brand.200', hex: '#E8B8B8', className: 'bg-brand-200' },
  { name: 'brand.300', hex: '#D98A8A', className: 'bg-brand-300' },
  { name: 'brand.400', hex: '#B84343', className: 'bg-brand-400', darkFg: true },
  { name: 'brand.500', hex: '#8B1A1A', className: 'bg-brand-500', darkFg: true, note: 'Primary CTA' },
  { name: 'brand.600', hex: '#6E1414', className: 'bg-brand-600', darkFg: true, note: 'Hover / pressed' },
  { name: 'brand.700', hex: '#5A0F0F', className: 'bg-brand-700', darkFg: true },
  { name: 'brand.800', hex: '#420909', className: 'bg-brand-800', darkFg: true },
  { name: 'brand.900', hex: '#2B0505', className: 'bg-brand-900', darkFg: true },
  { name: 'brand.950', hex: '#1A0303', className: 'bg-brand-950', darkFg: true },
]

const gold: Swatch[] = [
  { name: 'gold.50',  hex: '#FBF6EC', className: 'bg-gold-50' },
  { name: 'gold.100', hex: '#F5E9CD', className: 'bg-gold-100' },
  { name: 'gold.200', hex: '#E8D4A0', className: 'bg-gold-200' },
  { name: 'gold.300', hex: '#D6BC77', className: 'bg-gold-300' },
  { name: 'gold.400', hex: '#C9A44A', className: 'bg-gold-400' },
  { name: 'gold.500', hex: '#B08D57', className: 'bg-gold-500', darkFg: true, note: 'Aksen premium' },
  { name: 'gold.600', hex: '#8F7042', className: 'bg-gold-600', darkFg: true },
  { name: 'gold.700', hex: '#6E552F', className: 'bg-gold-700', darkFg: true },
  { name: 'gold.800', hex: '#4B3A20', className: 'bg-gold-800', darkFg: true },
  { name: 'gold.900', hex: '#2E2313', className: 'bg-gold-900', darkFg: true },
]

const ink: Swatch[] = [
  { name: 'canvas',     hex: '#FAFAF9', className: 'bg-canvas',   note: 'Background utama' },
  { name: 'canvas.alt', hex: '#F5F5F4', className: 'bg-canvas-alt', note: 'Section alt' },
  { name: 'ink.200',    hex: '#E7E5E4', className: 'bg-ink-200',  note: 'Hairline border' },
  { name: 'ink.300',    hex: '#D6D3D1', className: 'bg-ink-300' },
  { name: 'ink.400',    hex: '#A8A29E', className: 'bg-ink-400' },
  { name: 'ink.500',    hex: '#78716C', className: 'bg-ink-500',  darkFg: true, note: 'Muted text' },
  { name: 'ink.600',    hex: '#57534E', className: 'bg-ink-600',  darkFg: true },
  { name: 'ink.700',    hex: '#44403C', className: 'bg-ink-700',  darkFg: true },
  { name: 'ink.800',    hex: '#292524', className: 'bg-ink-800',  darkFg: true },
  { name: 'ink.900',    hex: '#171717', className: 'bg-ink-900',  darkFg: true, note: 'Body text' },
  { name: 'ink.950',    hex: '#0A0A0A', className: 'bg-ink-950',  darkFg: true, note: 'Heading / sidebar' },
]

// --- Copy-to-clipboard ---
const copiedKey = ref<string | null>(null)
async function copy(value: string, key: string) {
  try {
    await navigator.clipboard.writeText(value)
    copiedKey.value = key
    setTimeout(() => {
      if (copiedKey.value === key) copiedKey.value = null
    }, 1200)
  } catch {
    // Fallback: nothing — user can select manually.
  }
}

// --- Pemilih produk (multi-select, dipakai form diskon §28.9) ---
const demoProductSelection = ref<string[]>(['prod-2'])
const demoProducts = [
  { id: 'prod-1', name: 'Banner Vinyl', category: 'Outdoor', is_active: true },
  { id: 'prod-2', name: 'X-Banner', category: 'Indoor', is_active: true },
  { id: 'prod-3', name: 'Spanduk', category: 'Outdoor', is_active: true },
  { id: 'prod-4', name: 'Roll Banner (lama)', category: 'Indoor', is_active: false },
]

// --- Pemilih member (multi-select, dipakai form diskon §30.3) ---
const demoMemberSelection = ref<string[]>(['mem-1'])
const demoMembers = [
  { id: 'mem-1', name: 'Bu Sari', phone: '628123456789' },
  { id: 'mem-2', name: 'Pak Joko', phone: '628567891234' },
  { id: 'mem-3', name: 'Nadia Putri', phone: '628998877665' },
]

// --- Editor artikel: toolbar markdown + panel SEO (dipakai /admin/artikel) ---
const dsMarkdown = ref('## Kenapa memilih banner outdoor tahan air\n\nBanner outdoor tahan air cocok untuk...')
const dsFocusKeyword = ref('banner outdoor tahan air')
const dsSecondaryKeywords = ref('spanduk outdoor, banner flexi')
const dsMetaTitle = ref('')
const dsMetaDescription = ref('')
const dsSeoScore = ref(0)

// --- Interactive badge sample ---
const badgeStates = [
  { label: 'draft', tone: 'amber' },
  { label: 'published', tone: 'green' },
  { label: 'pending', tone: 'amber' },
  { label: 'approved', tone: 'green' },
  { label: 'rejected', tone: 'rose' },
  { label: 'archived', tone: 'ink' },
  { label: 'dikirim', tone: 'sky' },
]

// --- Motion demo ---
const spinning = ref(false)
function triggerSpin() {
  spinning.value = true
  setTimeout(() => (spinning.value = false), 2000)
}

// --- Auto-slider (crossfade) demo — pola dipakai di LandingProductSlider ---
const prefersReducedMotion = usePrefersReducedMotion()
const dsSlides = ['Banner Outdoor', 'X-Banner Pameran', 'Backdrop Event']
const dsActive = ref(0)
const dsPaused = ref(false)
const DS_DWELL_MS = 4000

let dsTimer: ReturnType<typeof setTimeout> | null = null
function dsClearTimer() {
  if (dsTimer) {
    clearTimeout(dsTimer)
    dsTimer = null
  }
}
function dsScheduleNext() {
  dsClearTimer()
  if (prefersReducedMotion.value || dsPaused.value) return
  dsTimer = setTimeout(() => {
    dsActive.value = (dsActive.value + 1) % dsSlides.length
    dsScheduleNext()
  }, DS_DWELL_MS)
}
function dsGoTo(i: number) {
  dsActive.value = i
  dsScheduleNext()
}
function dsOnEnter() {
  dsPaused.value = true
  dsClearTimer()
}
function dsOnLeave() {
  dsPaused.value = false
  dsScheduleNext()
}
onMounted(dsScheduleNext)
onUnmounted(dsClearTimer)
const dsTransition = computed(() =>
  prefersReducedMotion.value ? { duration: 0 } : { duration: 0.6, ease: [0.22, 1, 0.36, 1] as const },
)

// --- Input demo state ---
const demoInput = ref('')
const demoTextarea = ref('')

// --- Select / dropdown filter demo (pola dipakai di /admin/rekap) ---
const demoSelect = ref('')
const demoSelectLoading = ref(false)

// --- Toggle switch demo state (pola dipakai di /admin/pengaturan, §30.1) ---
const dsToggleOn = ref(true)
const dsToggleDisabled = ref(false)

// --- OTP input demo state (pola dipakai di /auth/google, sub-state otp_form) ---
const demoOtp = ref('')
function onDemoOtpInput(e: Event) {
  const raw = (e.target as HTMLInputElement).value
  const digits = raw.replace(/\D/g, '').slice(0, 6)
  demoOtp.value = digits
  ;(e.target as HTMLInputElement).value = digits
}
</script>

<template>
  <section>
    <AdminPageHeader
      title="Design System"
      subtitle="Living reference untuk CLAUDE.md §26. Klik swatch/token untuk copy classname."
    />

    <!-- Table of contents -->
    <nav class="mb-10 grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs font-medium">
      <a href="#palet" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Palet warna</a>
      <a href="#typography" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Tipografi</a>
      <a href="#buttons" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Buttons</a>
      <a href="#inputs" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Inputs</a>
      <a href="#toggle-switch" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Toggle switch</a>
      <a href="#product-multiselect" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Pemilih produk</a>
      <a href="#member-multiselect" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Pemilih member</a>
      <a href="#badges" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Badges</a>
      <a href="#cards" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Cards</a>
      <a href="#order-items-table" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Tabel item pesanan</a>
      <a href="#item-editor" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Editor baris item</a>
      <a href="#collapsible-cart-row" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Baris keranjang</a>
      <a href="#media-upload" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Media upload</a>
      <a href="#icons" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Icons</a>
      <a href="#motion" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Motion</a>
      <a href="#slider" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Auto-slider</a>
      <a href="#dark-section" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Section gelap</a>
      <a href="#spotlight" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Sorotan kursor</a>
      <a href="#scroll-motion" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">Gerak scroll</a>
    </nav>

    <!-- ================================= Palet ================================= -->
    <section id="palet" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Palet warna</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Tiga rumpun: <strong class="text-ink-900">brand</strong> (crimson deep, primary CTA + brand accent),
        <strong class="text-ink-900">gold</strong> (emas antique, aksen premium),
        <strong class="text-ink-900">ink</strong> (warm neutral gray — foreground, border, background).
        Klik swatch untuk copy classname Tailwind.
      </p>

      <h3 class="mt-8 mb-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Brand (crimson)</h3>
      <div class="grid grid-cols-4 sm:grid-cols-6 lg:grid-cols-11 gap-2">
        <button
          v-for="s in brand"
          :key="s.name"
          type="button"
          :class="[s.className, 'group aspect-square rounded-md p-2 text-left text-[10px] font-medium ring-1 ring-black/5 hover:ring-black/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-all flex flex-col justify-end']"
          @click="copy(s.className, s.name)"
        >
          <span :class="[s.darkFg ? 'text-canvas' : 'text-ink-800']">
            {{ s.hex }}
          </span>
          <span :class="[s.darkFg ? 'text-canvas/70' : 'text-ink-700', 'font-mono text-[9px] mt-0.5']">
            {{ s.name }}
          </span>
          <span
            v-if="copiedKey === s.name"
            class="mt-1 inline-flex items-center gap-1 text-canvas text-[9px]"
          >
            <Check class="h-3 w-3" :stroke-width="2" /> copied
          </span>
        </button>
      </div>

      <h3 class="mt-8 mb-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Gold (aksen)</h3>
      <div class="grid grid-cols-4 sm:grid-cols-6 lg:grid-cols-11 gap-2">
        <button
          v-for="s in gold"
          :key="s.name"
          type="button"
          :class="[s.className, 'group aspect-square rounded-md p-2 text-left text-[10px] font-medium ring-1 ring-black/5 hover:ring-black/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-all flex flex-col justify-end']"
          @click="copy(s.className, s.name)"
        >
          <span :class="[s.darkFg ? 'text-canvas' : 'text-ink-800']">{{ s.hex }}</span>
          <span :class="[s.darkFg ? 'text-canvas/70' : 'text-ink-700', 'font-mono text-[9px] mt-0.5']">{{ s.name }}</span>
          <span v-if="copiedKey === s.name" class="mt-1 inline-flex items-center gap-1 text-canvas text-[9px]">
            <Check class="h-3 w-3" :stroke-width="2" /> copied
          </span>
        </button>
      </div>

      <h3 class="mt-8 mb-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Ink & Canvas (warm neutral)</h3>
      <div class="grid grid-cols-4 sm:grid-cols-6 lg:grid-cols-11 gap-2">
        <button
          v-for="s in ink"
          :key="s.name"
          type="button"
          :class="[s.className, 'group aspect-square rounded-md p-2 text-left text-[10px] font-medium ring-1 ring-black/5 hover:ring-black/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-all flex flex-col justify-end']"
          @click="copy(s.className, s.name)"
        >
          <span :class="[s.darkFg ? 'text-canvas' : 'text-ink-800']">{{ s.hex }}</span>
          <span :class="[s.darkFg ? 'text-canvas/70' : 'text-ink-700', 'font-mono text-[9px] mt-0.5']">{{ s.name }}</span>
          <span v-if="copiedKey === s.name" class="mt-1 inline-flex items-center gap-1 text-canvas text-[9px]">
            <Check class="h-3 w-3" :stroke-width="2" /> copied
          </span>
        </button>
      </div>

      <div class="mt-6 rounded-lg border border-hairline bg-canvas p-4 text-xs text-ink-500 leading-relaxed">
        <p><strong class="text-ink-900">Semantic states</strong> (untuk badge/alert) pakai emerald/amber/rose/sky Tailwind default — dianggap semantic, bukan brand. Hindari untuk decorative.</p>
      </div>
    </section>

    <!-- ================================= Typography ================================= -->
    <section id="typography" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Tipografi</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Trio wajib: <strong class="text-ink-900">Fraunces</strong> (serif display, editorial),
        <strong class="text-ink-900">Inter</strong> (sans UI/body), <strong class="text-ink-900">JetBrains Mono</strong> (data teknis).
      </p>

      <div class="mt-8 space-y-6">
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">Display XL · Fraunces 600 · text-5xl md:text-7xl</p>
          <p class="font-serif text-5xl md:text-7xl font-semibold tracking-tight text-ink-950 leading-[1.05]">Cetak yang Regal.</p>
        </div>

        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">Display · Fraunces 600 · text-4xl md:text-5xl</p>
          <p class="font-serif text-4xl md:text-5xl font-semibold tracking-tight text-ink-950">Hero admin section</p>
        </div>

        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">H1 page · Fraunces 600 · text-2xl md:text-3xl</p>
          <p class="font-serif text-2xl md:text-3xl font-semibold tracking-tight text-ink-950">Verifikasi Pembayaran</p>
        </div>

        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">H2 section · Inter 600 · text-lg md:text-xl</p>
          <p class="text-lg md:text-xl font-semibold text-ink-900">Ringkasan minggu ini</p>
        </div>

        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">Body · Inter 400 · text-sm md:text-base · leading-relaxed</p>
          <p class="text-sm md:text-base text-ink-900 leading-relaxed max-w-2xl">
            Rajaku Printing membantu Anda mencetak banner berkualitas dengan proses yang cepat dan
            transparan. Upload desain sendiri atau minta bantuan tim desainer kami — hasil dijamin
            tahan cuaca outdoor.
          </p>
        </div>

        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">Small / meta · Inter 400 · text-xs · text-ink-500</p>
          <p class="text-xs text-ink-500">Diupdate 12 Agu 2026, 13.36 · oleh Owner Rajaku</p>
        </div>

        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">Eyebrow · Inter 500 · tracking-[0.14em] uppercase</p>
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Section header</p>
        </div>

        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">Mono · JetBrains Mono 400 · text-xs</p>
          <p class="font-mono text-xs text-ink-500">RJK-8F3K2A9X · deb76a6e-0be9-4a2f-99ea-adcfc27a6e0c</p>
        </div>
      </div>
    </section>

    <!-- ================================= Buttons ================================= -->
    <section id="buttons" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Buttons</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Hover, focus (Tab-in), disabled — semua state ada. Primary = brand crimson, secondary = outline hairline, destructive tetap brand (bukan rose).
      </p>

      <div class="mt-6 rounded-lg border border-hairline bg-canvas p-6 space-y-5">
        <div>
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">Primary</p>
          <div class="flex flex-wrap gap-3 items-center">
            <button class="inline-flex items-center rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">
              Simpan perubahan
            </button>
            <button disabled class="inline-flex items-center rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas opacity-50 cursor-not-allowed">
              Disabled
            </button>
          </div>
        </div>

        <div>
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">Secondary</p>
          <div class="flex flex-wrap gap-3 items-center">
            <button class="inline-flex items-center rounded-md border border-hairline bg-canvas px-4 py-2 text-sm font-semibold text-ink-900 hover:bg-canvas-alt hover:border-ink-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">
              Batal
            </button>
            <button class="inline-flex items-center rounded-md border border-hairline bg-canvas px-4 py-2 text-sm font-semibold text-ink-500 hover:text-ink-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">
              Ghost link
            </button>
          </div>
        </div>

        <div>
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">Destructive (pakai brand-500, bukan rose)</p>
          <div class="flex flex-wrap gap-3 items-center">
            <button class="inline-flex items-center rounded-md border border-brand-200 bg-canvas px-4 py-2 text-sm font-semibold text-brand-700 hover:bg-brand-50 hover:border-brand-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">
              Hapus
            </button>
            <button class="inline-flex items-center rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">
              Konfirmasi hapus
            </button>
          </div>
        </div>

        <div>
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">With icon (Lucide monoline)</p>
          <div class="flex flex-wrap gap-3 items-center">
            <button class="inline-flex items-center gap-2 rounded-md bg-ink-950 px-4 py-2 text-sm font-semibold text-canvas hover:bg-ink-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors">
              <CreditCard class="h-4 w-4" :stroke-width="1.75" />
              Verifikasi bukti
            </button>
          </div>
        </div>

        <div>
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">
            Third-party mark (satu-satunya pengecualian hex hardcode §26 — brand guideline Google)
          </p>
          <div class="max-w-xs">
            <AuthGoogleLoginButton redirect="/akun" />
          </div>
          <p class="mt-2 text-xs text-ink-500 leading-relaxed">
            Dipakai di <span class="font-mono">/login</span> & <span class="font-mono">/register</span> — komponen
            <span class="font-mono">components/auth/GoogleLoginButton.vue</span>. Style tetap secondary button
            (border-hairline), warna 4-tone SVG "G" resmi Google adalah satu-satunya pengecualian hex hardcode.
          </p>
        </div>
      </div>
    </section>

    <!-- ================================= Inputs ================================= -->
    <section id="inputs" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Inputs</h2>
      <div class="mt-6 grid gap-4 md:grid-cols-2">
        <div class="rounded-lg border border-hairline bg-canvas p-5 space-y-4">
          <div>
            <label class="block text-sm font-medium text-ink-900">Text input</label>
            <input
              v-model="demoInput"
              type="text"
              placeholder="Judul artikel…"
              class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
            >
          </div>

          <div>
            <label class="block text-sm font-medium text-ink-900">Textarea</label>
            <textarea
              v-model="demoTextarea"
              rows="3"
              placeholder="Body markdown…"
              class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
            />
          </div>
        </div>

        <div class="rounded-lg border border-hairline bg-canvas p-5 space-y-4">
          <div>
            <label class="block text-sm font-medium text-ink-900">Error state</label>
            <input
              type="text"
              value="invalid slug!"
              class="mt-1 block w-full rounded-md border border-brand-500 bg-canvas px-3 py-2 text-sm text-ink-900 focus:ring-brand-500/30 focus:ring-2 focus:outline-none"
            >
            <p class="mt-1 text-xs text-brand-700">Slug hanya boleh a-z, 0-9, dan dash.</p>
          </div>

          <div>
            <label class="block text-sm font-medium text-ink-500">Disabled</label>
            <input
              type="text"
              value="Read-only value"
              disabled
              class="mt-1 block w-full rounded-md border border-hairline bg-canvas-alt px-3 py-2 text-sm text-ink-500 cursor-not-allowed"
            >
          </div>
        </div>

        <div class="rounded-lg border border-hairline bg-canvas p-5 space-y-2 md:col-span-2">
          <label class="block text-sm font-medium text-ink-900">Kode OTP (verifikasi WhatsApp)</label>
          <p class="text-xs text-ink-500 leading-relaxed">
            Dipakai di <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/auth/google</code>
            sub-state <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">otp_form</code> —
            input tunggal 6 digit (bukan 6 kotak terpisah), <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">inputmode="numeric"</code>,
            strip karakter non-digit saat input, <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">autocomplete="one-time-code"</code> untuk auto-fill SMS/WA di mobile.
          </p>
          <input
            :value="demoOtp"
            type="text"
            inputmode="numeric"
            autocomplete="one-time-code"
            maxlength="6"
            placeholder="000000"
            class="mt-1 block w-full max-w-xs rounded-md border border-hairline bg-canvas px-3 py-2.5 text-center font-mono text-lg tracking-[0.5em] text-ink-900 placeholder-ink-300 transition-colors focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20"
            @input="onDemoOtpInput"
          >
          <p class="text-xs text-ink-500">Countdown kedaluwarsa & tombol kirim ulang pakai style link/tombol standar (lihat halaman aslinya) — bukan komponen terpisah di sini.</p>
        </div>

        <div class="rounded-lg border border-hairline bg-canvas p-5 space-y-2 md:col-span-2">
          <label class="block text-sm font-medium text-ink-900">Rentang tanggal (date range)</label>
          <p class="text-xs text-ink-500 leading-relaxed">
            Dipakai di <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/admin/rekap</code> — dua
            <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">&lt;input type="date"&gt;</code> berdampingan,
            saling membatasi lewat <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">:max</code>/<code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">:min</code>
            satu sama lain supaya "Dari" tidak bisa lebih besar dari "Sampai" — bukan komponen date-range picker
            kustom, cukup native input yang sudah didukung semua browser modern.
          </p>
          <div class="flex flex-wrap items-end gap-3">
            <div>
              <label class="block text-xs font-medium text-ink-700">Dari tanggal</label>
              <input
                type="date"
                value="2026-07-25"
                class="mt-1 block rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              >
            </div>
            <div>
              <label class="block text-xs font-medium text-ink-700">Sampai tanggal</label>
              <input
                type="date"
                value="2026-08-24"
                class="mt-1 block rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              >
            </div>
          </div>
        </div>

        <div class="rounded-lg border border-hairline bg-canvas p-5 space-y-2 md:col-span-2">
          <label class="block text-sm font-medium text-ink-900">Select / dropdown filter berbasis data server</label>
          <p class="text-xs text-ink-500 leading-relaxed">
            Dipakai di <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/admin/rekap</code> untuk
            filter kasir & diskon — <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">&lt;select&gt;</code>
            native (bukan combobox/searchable kustom), opsinya dimuat dari endpoint (bukan hardcode) dan
            dimuat ulang tiap dependensinya berubah (di sini: rentang tanggal). Kalau pilihan yang sedang aktif
            hilang dari daftar baru, kosongkan otomatis — jangan biarkan filter menyaring dengan nilai yang
            sudah tak terlihat di dropdown.
          </p>
          <div class="flex flex-wrap items-end gap-3">
            <div>
              <label class="block text-xs font-medium text-ink-700">Diskon</label>
              <select
                v-model="demoSelect"
                :disabled="demoSelectLoading"
                class="mt-1 block w-56 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:opacity-60"
              >
                <option value="">Semua diskon</option>
                <option value="__manual__">Diskon manual</option>
                <option value="a1b2c3d4-0000-0000-0000-000000000001">Promo Lebaran</option>
              </select>
              <p class="mt-1 text-xs text-ink-500">{{ demoSelectLoading ? 'Memuat pilihan…' : 'Opsi "Diskon manual" dikirim lewat parameter query terpisah, bukan sebagai ID.' }}</p>
            </div>
            <button
              type="button"
              class="rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
              @click="demoSelectLoading = !demoSelectLoading"
            >
              Toggle loading
            </button>
          </div>
        </div>
      </div>
    </section>

    <!-- ================================= Toggle switch ================================= -->
    <section id="toggle-switch" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Toggle switch</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">&lt;AdminToggleSwitch&gt;</code> —
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">v-model</code> boolean, dipakai untuk
        setting on/off satu keping (mis. <strong class="text-ink-900">Membership</strong> di
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/admin/pengaturan</code>, §30.1). Native
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">&lt;button role="switch"&gt;</code> — bukan
        checkbox — karena konvensinya membaca "saklar yang langsung berefek", bukan "pilih dari daftar".
      </p>
      <div class="mt-6 rounded-lg border border-hairline bg-canvas p-5 space-y-4">
        <div class="flex items-center gap-3">
          <AdminToggleSwitch v-model="dsToggleOn" aria-label="Contoh toggle aktif" />
          <span class="text-sm font-medium text-ink-900">{{ dsToggleOn ? 'Aktif' : 'Nonaktif' }}</span>
        </div>
        <div class="flex items-center gap-3">
          <AdminToggleSwitch v-model="dsToggleDisabled" disabled aria-label="Contoh toggle disabled" />
          <span class="text-sm font-medium text-ink-500">Disabled (mis. tanpa permission settings.manage)</span>
        </div>
      </div>
    </section>

    <!-- ================================= Pemilih produk (multi-select) ================================= -->
    <section id="product-multiselect" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Pemilih produk (multi-select)</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">&lt;AdminProductMultiSelect&gt;</code> —
        checklist produk + pencarian, dipakai form <strong class="text-ink-900">Diskon</strong>
        (<code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/admin/diskon</code>) untuk cakupan
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">applies_to: 'selected'</code> (§28.9). Produk
        nonaktif tetap tampil (ditandai "Nonaktif"), tidak disembunyikan — supaya diskon lama yang sudah menunjuk
        produk tak-aktif tetap terlihat apa adanya saat form dibuka ulang.
      </p>
      <div class="mt-6 max-w-md rounded-lg border border-hairline bg-canvas p-5">
        <AdminProductMultiSelect v-model="demoProductSelection" :products="demoProducts" />
        <p class="mt-3 text-xs text-ink-500">
          Terpilih: <span class="font-mono text-ink-700">{{ demoProductSelection.join(', ') || '(kosong)' }}</span>
        </p>
      </div>
    </section>

    <!-- ================================= Pemilih member ================================= -->
    <section id="member-multiselect" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Pemilih member (multi-select)</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">&lt;AdminMemberMultiSelect&gt;</code> —
        checklist member aktif + pencarian, dipakai form <strong class="text-ink-900">Diskon</strong>
        (<code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/admin/diskon</code>) untuk cakupan
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">member_scope: 'selected_members'</code> (§30.3). Pola
        sama dengan pemilih produk di atas, tapi sumber datanya daftar member berstatus <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">active</code> —
        diskon khusus member cuma pernah berlaku untuk mereka.
      </p>
      <div class="mt-6 max-w-md rounded-lg border border-hairline bg-canvas p-5">
        <AdminMemberMultiSelect v-model="demoMemberSelection" :members="demoMembers" />
        <p class="mt-3 text-xs text-ink-500">
          Terpilih: <span class="font-mono text-ink-700">{{ demoMemberSelection.join(', ') || '(kosong)' }}</span>
        </p>
      </div>
    </section>

    <!-- ================================= Badges ================================= -->
    <section id="badges" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Badges</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Status pill semantic — auto-tone via <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">&lt;AdminStatusBadge&gt;</code>.
      </p>
      <div class="mt-6 rounded-lg border border-hairline bg-canvas p-5 flex flex-wrap gap-2">
        <AdminStatusBadge v-for="b in badgeStates" :key="b.label" :status="b.label" />
      </div>

      <p class="mt-4 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Kosakata di luar keyword bawaan (mis. status modul <strong class="text-ink-900">Diskon</strong> — 'aktif',
        'terjadwal', 'kadaluarsa', 'nonaktif', 'kuota_habis' — tidak cocok dengan keyword order/artikel manapun)
        WAJIB memberi prop <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">tone</code> eksplisit,
        bukan mengandalkan deteksi kata kunci otomatis:
      </p>
      <div class="mt-3 rounded-lg border border-hairline bg-canvas p-5 flex flex-wrap gap-2">
        <AdminStatusBadge status="Aktif" tone="green" />
        <AdminStatusBadge status="Terjadwal" tone="amber" />
        <AdminStatusBadge status="Kuota habis" tone="rose" />
        <AdminStatusBadge status="Kadaluarsa" tone="ink" />
        <AdminStatusBadge status="Nonaktif" tone="ink" />
      </div>

      <p class="mt-4 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Badge <strong class="text-ink-900">Member</strong> (§30.4) BUKAN preset <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">&lt;AdminStatusBadge&gt;</code> —
        markup manual dengan <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">bg-gold-50 text-gold-900 ring-gold-200</code>, dipakai di
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/admin/pos</code> (hasil pencarian pelanggan),
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/admin/pelanggan</code> (daftar &amp; detail pelanggan) &amp;
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/akun/membership</code> (badge "Member Aktif") — emas dipilih karena konteksnya
        memang premium (§26.1), bukan status semantic biasa.
      </p>
      <div class="mt-3 rounded-lg border border-hairline bg-canvas p-5 flex flex-wrap gap-2">
        <span class="inline-flex items-center gap-1 rounded-full bg-gold-50 px-1.5 py-0.5 text-[10px] font-medium text-gold-900 ring-1 ring-inset ring-gold-200">
          <Crown class="h-2.5 w-2.5" :stroke-width="1.75" />
          Member
        </span>
        <span class="inline-flex items-center gap-1.5 rounded-full bg-gold-50 px-2.5 py-1 text-xs font-medium text-gold-900 ring-1 ring-inset ring-gold-200">
          <Crown class="h-3 w-3" :stroke-width="1.75" />
          Member Aktif
        </span>
      </div>
    </section>

    <!-- ================================= Cards ================================= -->
    <section id="cards" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Cards</h2>
      <div class="mt-6 grid gap-4 md:grid-cols-3">
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <h3 class="text-sm font-semibold text-ink-900">Static card</h3>
          <p class="mt-1 text-xs text-ink-500 leading-relaxed">Container info yang tidak diklik. Default state — no shadow, hairline border.</p>
        </div>

        <a href="#" class="group rounded-lg border border-hairline bg-canvas p-6 hover:border-ink-300 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas">
          <h3 class="text-sm font-semibold text-ink-900 group-hover:text-brand-500 transition-colors">Interactive link card</h3>
          <p class="mt-1 text-xs text-ink-500 leading-relaxed">Hover: border shift to ink-300, title shift to brand-500.</p>
        </a>

        <div class="rounded-lg bg-ink-950 p-6 text-canvas">
          <h3 class="font-serif text-lg font-semibold">Dark card</h3>
          <p class="mt-1 text-xs text-ink-300 leading-relaxed">Untuk highlight premium — kombinasi dengan aksen <span class="text-gold-400">gold-400</span>.</p>
        </div>
      </div>

      <p class="mt-4 max-w-2xl text-sm text-ink-500 leading-relaxed">
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">&lt;AdminDataTable&gt;</code> punya prop opsional
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">row-clickable</code> + event
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">@row-click</code> — seluruh baris jadi klikable
        (bukan cuma satu sel jadi link), dipakai <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/admin/pelanggan</code>
        untuk navigasi ke halaman detail. Sel yang punya elemen interaktifnya sendiri (tombol/link) wajib
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">@click.stop</code> supaya tidak ikut memicu navigasi baris.
      </p>
    </section>

    <!-- ================================= Tabel item pesanan ================================= -->
    <section id="order-items-table" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Tabel item pesanan</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">&lt;OrderItemsTable&gt;</code> (§32 Order
        Multi-Item) — dipakai bersama oleh <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/admin/order/[resi]</code>
        &amp; <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/akun/pesanan/[resi]</code> untuk
        menampilkan baris produk (produk, bahan, ukuran, qty, harga satuan, subtotal). Sengaja
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">overflow-x-auto</code>, bukan kartu
        bertumpuk — ini tabel BACA-SAJA di admin/akun, beda dari form order publik yang wajib kartu di HP (§18).
        Angka uang level order (Subtotal → Diskon → Ongkir → TOTAL, §28.7) TIDAK PERNAH dihitung dari tabel ini —
        selalu dari field <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">Order</code> langsung (§32.2).
      </p>
      <div class="mt-6">
        <OrderItemsTable :items="demoOrderItems" />
      </div>
    </section>

    <!-- ================================= Editor baris item (§32.9) ================================= -->
    <section id="item-editor" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Editor baris item pesanan</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Dipakai modal "Edit pesanan" di <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/admin/order/[resi]</code>
        (§32.9, super admin koreksi item). Beda dari "Tabel item pesanan" di atas — ini FORM EDITABLE, bukan
        baca-saja. Dua bentuk kartu baris: baris EXISTING (identitas produk/bahan tampil read-only + hint "hapus
        lalu tambah baru" — ganti produk sebuah baris memang tidak mungkin, itu keputusan sadar §32.9, bukan
        keterbatasan UI) dan baris BARU (pemilih produk/bahan + estimasi harga live dari <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/catalog/quote</code>,
        pola sama dengan <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/order</code>). Badge
        emerald kecil <span class="rounded-full bg-emerald-50 px-1.5 py-0.5 text-[10px] font-medium text-emerald-800 ring-1 ring-inset ring-emerald-200">Baru</span>
        menandai baris baru. Angka subtotal per baris &amp; total di footer editor SELALU berlabel "perkiraan" —
        angka final datang dari respons backend setelah simpan (§32.9), bukan dihitung ulang total dari nol lewat
        endpoint quote.
      </p>
      <div class="mt-6 max-w-lg space-y-3">
        <div class="rounded-md border border-hairline bg-canvas-alt/40 p-3">
          <div class="flex items-center justify-between gap-2">
            <p class="text-xs font-medium text-ink-900">Baris 1</p>
            <button type="button" class="rounded p-1 text-ink-500 hover:text-brand-600 transition-colors" aria-label="Hapus baris">
              <Trash2 class="h-3.5 w-3.5" :stroke-width="1.75" />
            </button>
          </div>
          <p class="mt-1.5 text-sm text-ink-900">Banner Flexi 280gsm <span class="text-ink-500">· Flexi China</span></p>
          <p class="text-[11px] text-ink-400">Ganti produk/bahan? Hapus baris ini, lalu tambah baris baru.</p>
          <div class="mt-2 flex justify-between border-t border-hairline pt-2 text-xs">
            <span class="text-ink-500">Subtotal baris</span>
            <span class="font-medium text-ink-900">Rp165.000</span>
          </div>
        </div>
        <div class="rounded-md border border-hairline bg-canvas-alt/40 p-3">
          <div class="flex items-center justify-between gap-2">
            <p class="text-xs font-medium text-ink-900">
              Baris 2
              <span class="ml-1.5 rounded-full bg-emerald-50 px-1.5 py-0.5 text-[10px] font-medium text-emerald-800 ring-1 ring-inset ring-emerald-200">Baru</span>
            </p>
            <button type="button" class="rounded p-1 text-ink-500 hover:text-brand-600 transition-colors" aria-label="Hapus baris">
              <Trash2 class="h-3.5 w-3.5" :stroke-width="1.75" />
            </button>
          </div>
          <p class="mt-1.5 text-xs text-ink-500">Estimasi Rp85.000 / pcs (dihitung ulang backend)</p>
        </div>
        <button type="button" class="inline-flex items-center gap-1.5 rounded-md border border-hairline bg-canvas px-3 py-1.5 text-xs font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors">
          <Plus class="h-3.5 w-3.5" :stroke-width="1.75" />
          Tambah baris (2/20)
        </button>
      </div>
    </section>

    <!-- ================================= Baris keranjang (dapat dilipat) ================================= -->
    <section id="collapsible-cart-row" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Baris keranjang (dapat dilipat)</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Dipakai <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/admin/pos</code> (§32 Order
        Multi-Item, ergonomi kasir) — beda tujuan dari "Tabel item pesanan" di atas: ini kartu FORM EDITABLE
        per baris keranjang (produk, bahan, ukuran, desain), bukan tabel baca-saja. Header baris (chevron +
        ringkasan + subtotal + hapus) selalu terlihat; isi form disembunyikan lewat <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">v-show</code>
        (bukan <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">v-if</code>, supaya state input
        tidak reset saat dilipat) — dipilih supaya layar tetap ringkas saat kasir melayani banyak barang
        sekaligus di depan konter, tanpa kehilangan konteks baris mana yang sedang dikerjakan. Baris pertama
        tidak pernah punya tombol hapus (order selalu butuh minimal 1 item, §32.4).
      </p>
      <div class="mt-6 max-w-xl rounded-md border border-hairline overflow-hidden">
        <div class="flex items-center gap-2 bg-canvas-alt px-3 py-2">
          <button
            type="button"
            class="flex-none rounded-md p-1 text-ink-500 hover:text-ink-900 hover:bg-canvas transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt"
            :aria-label="demoCartRowCollapsed ? 'Buka baris' : 'Lipat baris'"
            @click="demoCartRowCollapsed = !demoCartRowCollapsed"
          >
            <ChevronDown class="h-4 w-4 transition-transform" :class="demoCartRowCollapsed ? '-rotate-90' : ''" :stroke-width="1.75" />
          </button>
          <div class="min-w-0 flex-1">
            <p class="truncate text-sm font-semibold text-ink-900">1. Banner Flexi 280gsm</p>
            <p class="truncate text-xs text-ink-500"><span class="font-mono">300×100cm</span> · 1 pcs</p>
          </div>
          <span class="flex-none text-sm font-medium tabular-nums text-ink-900">Rp165.000</span>
          <button
            type="button"
            class="flex-none rounded-md p-1.5 text-ink-500 hover:text-brand-600 hover:bg-brand-50 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt"
            aria-label="Hapus baris"
          >
            <Trash2 class="h-4 w-4" :stroke-width="1.75" />
          </button>
        </div>
        <div v-show="!demoCartRowCollapsed" class="space-y-3 p-4 text-sm text-ink-500">
          <p>Isi form (produk, bahan, ukuran, sumber desain, catatan item) tampil di sini saat baris dibuka.</p>
        </div>
      </div>
    </section>

    <!-- ================================= Media upload card ================================= -->
    <section id="media-upload" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Media upload card</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Kartu ganti-gambar per slot — dipakai di
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/admin/site-media</code>. Pratinjau
        4:3 dengan placeholder monoline saat kosong, label input file (bukan tombol terpisah — klik area
        label langsung buka file picker, pola sama dengan cover upload artikel), tombol ikon "kembalikan ke
        bawaan" yang butuh <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">&lt;AdminConfirmDialog&gt;</code>
        karena destruktif. Dialog itu sendiri punya prop opsional
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">confirm-disabled</code> — dipakai
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/admin/pelanggan</code> untuk menonaktifkan
        tombol konfirmasi blokir/aktifkan sampai input alasan wajib cukup panjang (≥10 karakter), terpisah dari
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">loading</code> supaya spinner tidak ikut muncul
        saat cuma menunggu input.
      </p>
      <div class="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex items-start gap-2">
            <PaletteIcon class="mt-0.5 h-4 w-4 shrink-0 text-ink-400" :stroke-width="1.5" />
            <div>
              <h3 class="text-sm font-semibold text-ink-900">Hero Poster (Desktop)</h3>
              <p class="mt-1 text-xs leading-relaxed text-ink-500">Poster fallback hero cinematic scroll-scrub.</p>
            </div>
          </div>
          <div class="mt-3 flex items-center justify-between text-[10px] text-ink-500">
            <span class="font-mono">hero_poster_desktop</span>
            <span>Disarankan <span class="font-mono">1920×1080px</span></span>
          </div>
          <div class="mt-3 flex aspect-[4/3] items-center justify-center rounded-md border border-hairline bg-ink-950">
            <span class="text-[10px] text-canvas/60">Pratinjau terisi</span>
          </div>
          <dl class="mt-3 space-y-0.5 text-[10px] text-ink-500">
            <div class="flex justify-between gap-2"><dt>Berkas</dt><dd class="font-mono text-ink-700">poster-baru.webp</dd></div>
            <div class="flex justify-between gap-2"><dt>Ukuran</dt><dd class="font-mono text-ink-700">312 KB · 1920×1080px</dd></div>
          </dl>
          <div class="mt-4 flex items-center gap-2">
            <span class="inline-flex flex-1 items-center justify-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-xs font-semibold text-canvas">
              <Upload class="h-3.5 w-3.5" :stroke-width="1.75" /> Ganti gambar
            </span>
            <span class="inline-flex items-center justify-center rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700">
              <RotateCcw class="h-3.5 w-3.5" :stroke-width="1.75" />
            </span>
          </div>
        </div>

        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex items-start gap-2">
            <PaletteIcon class="mt-0.5 h-4 w-4 shrink-0 text-ink-400" :stroke-width="1.5" />
            <div>
              <h3 class="text-sm font-semibold text-ink-900">Langkah Proses 3</h3>
              <p class="mt-1 text-xs leading-relaxed text-ink-500">Belum pernah diunggah admin.</p>
            </div>
          </div>
          <div class="mt-3 flex items-center justify-between text-[10px] text-ink-500">
            <span class="font-mono">proses_3</span>
            <span>Disarankan <span class="font-mono">800×600px</span></span>
          </div>
          <div class="mt-3 flex aspect-[4/3] flex-col items-center justify-center gap-1.5 rounded-md border border-hairline bg-canvas-alt text-ink-400">
            <ImageOffIcon class="h-6 w-6" :stroke-width="1.5" />
            <span class="text-[10px]">Belum diatur — memakai gambar bawaan</span>
          </div>
          <div class="mt-4 flex items-center gap-2">
            <span class="inline-flex flex-1 items-center justify-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-xs font-semibold text-canvas">
              <Upload class="h-3.5 w-3.5" :stroke-width="1.75" /> Unggah gambar
            </span>
            <span class="inline-flex items-center justify-center rounded-md border border-hairline bg-canvas-alt px-3 py-2 text-ink-300">
              <RotateCcw class="h-3.5 w-3.5" :stroke-width="1.75" />
            </span>
          </div>
        </div>
      </div>
      <p class="mt-3 text-xs text-ink-500 leading-relaxed">
        Tombol reset dinonaktifkan (<code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">disabled</code>) kalau slot masih kosong — tidak ada yang bisa "dikembalikan".
      </p>
    </section>

    <!-- ================================= Icons ================================= -->
    <section id="icons" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Icons</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Wajib Lucide monoline (<code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">@lucide/vue</code>).
        Stroke 1.5-1.75, ukuran seragam per konteks (16 sidebar, 20 button, 24 card hero).
      </p>
      <div class="mt-6 rounded-lg border border-hairline bg-canvas p-6 flex flex-wrap gap-6 items-end">
        <div class="flex flex-col items-center gap-1">
          <CreditCard class="h-4 w-4 text-ink-700" :stroke-width="1.75" />
          <span class="text-[10px] text-ink-500 font-mono">h-4 sidebar</span>
        </div>
        <div class="flex flex-col items-center gap-1">
          <Package class="h-5 w-5 text-ink-700" :stroke-width="1.5" />
          <span class="text-[10px] text-ink-500 font-mono">h-5 button</span>
        </div>
        <div class="flex flex-col items-center gap-1">
          <PaletteIcon class="h-6 w-6 text-ink-700" :stroke-width="1.5" />
          <span class="text-[10px] text-ink-500 font-mono">h-6 card hero</span>
        </div>
        <div class="flex flex-col items-center gap-1">
          <PaletteIcon class="h-5 w-5 text-brand-500" :stroke-width="1.5" />
          <span class="text-[10px] text-ink-500 font-mono">brand-500 active</span>
        </div>
        <div class="flex flex-col items-center gap-1">
          <PaletteIcon class="h-5 w-5 text-gold-500" :stroke-width="1.5" />
          <span class="text-[10px] text-ink-500 font-mono">gold-500 aksen</span>
        </div>
      </div>
    </section>

    <!-- ================================= Motion ================================= -->
    <section id="motion" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Motion</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Transisi 150-250ms ease-out. Focus ring wajib. Spinner monoline halus, no rainbow.
      </p>

      <div class="mt-6 grid gap-4 md:grid-cols-3">
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-3">Focus ring (Tab-in)</p>
          <button class="rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas hover:bg-brand-600 transition-colors">
            Tab kemari
          </button>
          <p class="mt-3 text-xs text-ink-500">Tekan Tab lalu Shift+Tab untuk lihat ring.</p>
        </div>

        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-3">Hover transition</p>
          <div class="h-10 w-full rounded-md bg-ink-200 hover:bg-brand-500 transition-colors duration-200 cursor-pointer flex items-center justify-center text-sm font-medium text-ink-700 hover:text-canvas">
            Hover di sini
          </div>
        </div>

        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-3">Spinner</p>
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-md bg-ink-950 px-4 py-2 text-sm font-semibold text-canvas hover:bg-ink-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors"
            @click="triggerSpin"
          >
            <span
              v-if="spinning"
              class="inline-block h-3 w-3 rounded-full border-2 border-canvas/70 border-t-transparent animate-spin"
              aria-hidden="true"
            />
            {{ spinning ? 'Memproses…' : 'Trigger spin' }}
          </button>
        </div>
      </div>
    </section>

    <!-- ================================= Auto-slider (crossfade) ================================= -->
    <section id="slider" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Auto-slider (crossfade)</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Dipakai di <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">components/landing/ProductSlider.vue</code>.
        Crossfade via prop <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">:animate</code> reaktif motion-v
        (BUKAN <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">AnimatePresence</code> — semua slide tetap
        satu kali di-mount, SSR-friendly, hanya <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">opacity</code>
        yang berubah). Dwell ~6.5 detik, transisi 700ms ease-out.
      </p>
      <ul class="mt-3 max-w-2xl list-disc space-y-1 pl-5 text-xs text-ink-500 leading-relaxed">
        <li>Jeda otomatis saat hover ATAU fokus keyboard di dalam slider.</li>
        <li>Kontrol manual (panah + dot) selalu berfungsi, keyboard-reachable, focus ring §26.6.</li>
        <li><code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">prefers-reduced-motion</code>: auto-advance mati total, transisi jadi instan (durasi 0) — kontrol manual tetap jalan.</li>
        <li>Tanpa <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">aria-live</code> cerewet — cukup <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">aria-label</code> wajar di tiap kontrol.</li>
      </ul>

      <div class="mt-6 rounded-lg border border-hairline bg-canvas p-6">
        <div
          class="relative aspect-[21/9] w-full max-w-xl overflow-hidden rounded-lg border border-hairline bg-ink-950"
          role="group"
          aria-roledescription="carousel"
          aria-label="Contoh sorotan produk"
          @mouseenter="dsOnEnter"
          @mouseleave="dsOnLeave"
          @focusin="dsOnEnter"
          @focusout="dsOnLeave"
        >
          <motion.div
            v-for="(s, i) in dsSlides"
            :key="s"
            class="absolute inset-0 flex items-center justify-center px-6 text-center"
            :class="i === dsActive ? 'pointer-events-auto' : 'pointer-events-none'"
            :animate="{ opacity: i === dsActive ? 1 : 0 }"
            :transition="dsTransition"
            :aria-hidden="i !== dsActive"
          >
            <p class="font-serif text-lg font-semibold text-canvas">{{ s }}</p>
          </motion.div>

          <button
            type="button"
            class="absolute left-2 top-1/2 z-10 inline-flex h-8 w-8 -translate-y-1/2 items-center justify-center rounded-full bg-ink-950/50 text-canvas backdrop-blur transition-colors duration-200 ease-out hover:bg-ink-950/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold-400/60 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
            aria-label="Slide sebelumnya"
            @click="dsGoTo((dsActive - 1 + dsSlides.length) % dsSlides.length)"
          >
            <ChevronLeft class="h-4 w-4" :stroke-width="1.75" />
          </button>
          <button
            type="button"
            class="absolute right-2 top-1/2 z-10 inline-flex h-8 w-8 -translate-y-1/2 items-center justify-center rounded-full bg-ink-950/50 text-canvas backdrop-blur transition-colors duration-200 ease-out hover:bg-ink-950/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold-400/60 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
            aria-label="Slide berikutnya"
            @click="dsGoTo((dsActive + 1) % dsSlides.length)"
          >
            <ChevronRight class="h-4 w-4" :stroke-width="1.75" />
          </button>

          <div class="absolute inset-x-0 bottom-2 z-10 flex items-center justify-center gap-1">
            <button
              v-for="(s, i) in dsSlides"
              :key="s"
              type="button"
              class="group inline-flex h-6 w-6 items-center justify-center rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold-400/60 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
              :aria-label="`Ke slide ${i + 1}: ${s}`"
              :aria-current="i === dsActive ? 'true' : undefined"
              @click="dsGoTo(i)"
            >
              <span
                class="block h-1.5 rounded-full transition-[width,background-color] duration-200 ease-out"
                :class="i === dsActive ? 'w-6 bg-gold-400' : 'w-1.5 bg-canvas/40 group-hover:bg-canvas/70'"
              />
            </button>
          </div>
        </div>
        <p class="mt-3 text-xs text-ink-500">
          Arahkan kursor atau Tab masuk ke area slider untuk melihat auto-advance berhenti.
        </p>
      </div>
    </section>


    <!-- ================================= Section gelap ================================= -->
    <section id="dark-section" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Section gelap (on-dark)</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Landing page berselang terang-gelap supaya halaman tidak terbaca sebagai satu blok putih
        panjang. Dipakai di <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">ProcessGallery</code>, <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">WhyUsSection</code>,
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">ClosingCta</code>, hero, dan footer. Mengubah latar saja TIDAK cukup —
        seluruh token di dalamnya wajib ikut dibalik.
      </p>

      <div class="mt-4 overflow-x-auto">
        <table class="w-full min-w-[34rem] text-left text-xs">
          <thead class="text-[10px] uppercase tracking-[0.14em] text-ink-500">
            <tr>
              <th class="border-b border-hairline py-2 pr-4 font-medium">Peran</th>
              <th class="border-b border-hairline py-2 pr-4 font-medium">Di latar terang</th>
              <th class="border-b border-hairline py-2 font-medium">Di latar gelap</th>
            </tr>
          </thead>
          <tbody class="font-mono text-ink-700">
            <tr><td class="border-b border-hairline py-2 pr-4 font-sans text-ink-500">Latar</td><td class="border-b border-hairline py-2 pr-4">bg-canvas</td><td class="border-b border-hairline py-2">bg-ink-950</td></tr>
            <tr><td class="border-b border-hairline py-2 pr-4 font-sans text-ink-500">Judul</td><td class="border-b border-hairline py-2 pr-4">text-ink-950</td><td class="border-b border-hairline py-2">text-canvas</td></tr>
            <tr><td class="border-b border-hairline py-2 pr-4 font-sans text-ink-500">Body</td><td class="border-b border-hairline py-2 pr-4">text-ink-500</td><td class="border-b border-hairline py-2">text-canvas/70</td></tr>
            <tr><td class="border-b border-hairline py-2 pr-4 font-sans text-ink-500">Eyebrow</td><td class="border-b border-hairline py-2 pr-4">text-ink-500</td><td class="border-b border-hairline py-2">text-canvas/60</td></tr>
            <tr><td class="border-b border-hairline py-2 pr-4 font-sans text-ink-500">Garis</td><td class="border-b border-hairline py-2 pr-4">border-hairline</td><td class="border-b border-hairline py-2">border-white/10</td></tr>
            <tr><td class="border-b border-hairline py-2 pr-4 font-sans text-ink-500">Permukaan kartu</td><td class="border-b border-hairline py-2 pr-4">bg-canvas-alt</td><td class="border-b border-hairline py-2">bg-white/5</td></tr>
            <tr><td class="border-b border-hairline py-2 pr-4 font-sans text-ink-500">Aksen / ikon</td><td class="border-b border-hairline py-2 pr-4">text-brand-500</td><td class="border-b border-hairline py-2">text-gold-400</td></tr>
            <tr><td class="py-2 pr-4 font-sans text-ink-500">Offset focus ring</td><td class="py-2 pr-4">ring-offset-canvas</td><td class="py-2">ring-offset-ink-950</td></tr>
          </tbody>
        </table>
      </div>

      <p class="mt-4 max-w-2xl text-xs leading-relaxed text-ink-500">
        Aksen <strong class="text-ink-900">wajib pindah ke emas</strong> di latar gelap: crimson
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">brand-500</code> di atas <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">ink-950</code> kontrasnya jatuh di
        bawah ambang keterbacaan. Emas juga yang dipakai untuk focus ring di area gelap.
      </p>

      <div class="mt-6 rounded-lg border border-hairline bg-ink-950 p-6 md:p-8">
        <p class="flex items-center gap-2 text-[10px] font-medium uppercase tracking-[0.14em] text-canvas/60">
          <span class="h-px w-6 bg-gold-500" aria-hidden="true" />
          Eyebrow on-dark
        </p>
        <h3 class="mt-3 font-serif text-xl font-semibold tracking-tight text-canvas">Judul di latar gelap</h3>
        <p class="mt-2 max-w-md text-sm leading-relaxed text-canvas/70">
          Body memakai <code class="font-mono text-xs bg-white/10 px-1 py-0.5 rounded">text-canvas/70</code>. Garis pemisah
          <code class="font-mono text-xs bg-white/10 px-1 py-0.5 rounded">border-white/10</code>.
        </p>
        <div class="mt-5 flex flex-wrap items-center gap-3 border-t border-white/10 pt-5">
          <span class="inline-flex h-10 w-10 items-center justify-center rounded-md border border-white/10 bg-white/5 text-gold-400">
            <Sparkles class="h-5 w-5" :stroke-width="1.5" />
          </span>
          <a
            href="#dark-section"
            class="rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas transition-colors duration-200 ease-out hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold-400/60 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
          >CTA tetap crimson</a>
          <span class="text-xs text-canvas/50">
            Tombol solid boleh crimson — kontrasnya datang dari isian, bukan dari teks di atas hitam.
          </span>
        </div>
      </div>
    </section>

    <!-- ================================= Sorotan kursor ================================= -->
    <section id="spotlight" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Sorotan kursor (spotlight)</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Kartu grid di landing memakai sorotan lembut yang mengikuti kursor. Gayanya ada di
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">assets/css/tailwind.css</code>, posisinya dikirim
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">useSpotlight()</code>.
      </p>
      <ul class="mt-3 max-w-2xl list-disc space-y-1 pl-5 text-xs text-ink-500 leading-relaxed">
        <li>Pasang <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">@pointermove</code> di WADAH grid — satu listener untuk semua kartu, bukan satu per kartu.</li>
        <li>Beri kelas <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">spotlight</code> pada kartunya; isi yang harus berada di atas sorotan diberi <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">relative z-10</code>.</li>
        <li>Hanya aktif di perangkat berkursor (<code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">@media (hover: hover)</code>) — di layar sentuh, <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">:hover</code> menempel dan sorotannya tertinggal menyala.</li>
        <li>Mati saat <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">prefers-reduced-motion</code>.</li>
      </ul>

      <div class="mt-6 grid gap-4 sm:grid-cols-2" @pointermove="onPointerMove">
        <div class="spotlight rounded-lg border border-hairline bg-canvas p-6 transition-colors duration-200 ease-out hover:border-ink-300">
          <h3 class="relative z-10 text-sm font-semibold text-ink-950">Arahkan kursor ke sini</h3>
          <p class="relative z-10 mt-1 text-sm leading-relaxed text-ink-500">Sorotan mengikuti posisi kursor di dalam kartu.</p>
        </div>
        <div class="spotlight rounded-lg border border-hairline bg-canvas p-6 transition-colors duration-200 ease-out hover:border-ink-300">
          <h3 class="relative z-10 text-sm font-semibold text-ink-950">Intensitas 7%</h3>
          <p class="relative z-10 mt-1 text-sm leading-relaxed text-ink-500">Cukup terasa, tidak sampai mengubah warna kartu.</p>
        </div>
      </div>
    </section>

    <!-- ================================= Gerak berbasis scroll ================================= -->
    <section id="scroll-motion" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Gerak berbasis scroll</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Tiga efek di landing digerakkan posisi scroll lewat CSS <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">animation-timeline</code>,
        bukan listener di JavaScript: garis progres baca (<code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">.scroll-progress</code> di layout),
        hanyutan hero, dan hanyutan foto galeri proses. Animasinya berjalan di compositor, jadi tidak ada
        frame yang dihitung di main thread.
      </p>
      <ul class="mt-3 max-w-2xl list-disc space-y-1 pl-5 text-xs text-ink-500 leading-relaxed">
        <li>
          <strong class="text-ink-900">Jebakan wadah scroll.</strong>
          <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">view()</code> mengukur elemen terhadap wadah scroll TERDEKAT, dan
          <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">overflow-hidden</code> / <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">overflow-x-auto</code> sudah dihitung
          sebagai wadah scroll. Dipasang di dalamnya, animasi diam total tanpa memunculkan error apa pun.
          Solusinya: taruh <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">view-timeline-name</code> di elemen luar yang memang diukur
          terhadap dokumen, lalu rujuk namanya dari dalam.
        </li>
        <li>Semua dibungkus <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">@supports (animation-timeline: view())</code> — Safari melewatinya, elemen diam, tidak ada yang rusak.</li>
        <li>Dibungkus juga <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">@media (prefers-reduced-motion: no-preference)</code>.</li>
        <li>Keadaan diamnya wajib aman: garis progres mulai dari <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">scaleX(0)</code>, jadi kalau animasi tidak pernah jalan ia sekadar tak terlihat.</li>
      </ul>
      <p class="mt-3 text-xs text-ink-500">
        Contoh hidupnya ada di puncak halaman publik — garis tipis di atas navbar yang memanjang saat halaman digulir.
      </p>
    </section>

    <!-- ================================= Editor artikel ================================= -->
    <section id="article-editor" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Editor artikel — toolbar markdown &amp; panel SEO</h2>
      <p class="mt-2 max-w-2xl text-sm text-ink-500 leading-relaxed">
        Dipakai di <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/admin/artikel/[id]</code> dan
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">/admin/artikel/new</code>. Toolbar
        (<code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">AdminMarkdownToolbar</code>) memanipulasi
        teks lewat seleksi textarea langsung — heading/bold/italic/list/quote/link/gambar. Panel SEO
        (<code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">AdminSeoPanel</code>) menghitung skor 0-100
        dari 12 pengecekan gaya Rank Math, murni client-side lewat
        <code class="font-mono text-xs bg-canvas-alt px-1 py-0.5 rounded">useSeoAnalysis()</code>.
      </p>

      <div class="mt-6 grid gap-6 lg:grid-cols-3">
        <div class="lg:col-span-2">
          <label class="text-sm font-medium text-ink-900">Konten (Markdown)</label>
          <AdminMarkdownToolbar v-model="dsMarkdown" :rows="8" />
        </div>
        <AdminSeoPanel
          v-model:focus-keyword="dsFocusKeyword"
          v-model:secondary-keywords="dsSecondaryKeywords"
          v-model:meta-title="dsMetaTitle"
          v-model:meta-description="dsMetaDescription"
          title="Panduan Memilih Banner Outdoor"
          slug="panduan-memilih-banner-outdoor"
          :content-md="dsMarkdown"
          cover-alt-text="Contoh banner outdoor terpasang di toko"
          @update:score="dsSeoScore = $event"
        />
      </div>
    </section>

    <div class="rounded-lg border border-gold-200 bg-gold-50 p-4 text-xs text-gold-900 leading-relaxed">
      <p><strong>Update flow (§26.11):</strong> tambah pattern baru di halaman ini + di CLAUDE.md §26 dalam PR yang sama. Kalau halaman ini dan implementasi drift → halaman ini yang benar; implementasi refactor.</p>
    </div>
  </section>
</template>
