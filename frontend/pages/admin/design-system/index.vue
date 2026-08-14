<script setup lang="ts">
/**
 * /admin/design-system — Living reference untuk CLAUDE.md §26.
 * Semua token diambil dari tailwind.config.ts (di-hardcode di sini karena
 * Tailwind config tidak runtime-introspectable; kalau config berubah,
 * update halaman ini juga — flow §26.11).
 */
import { Check, Copy, CreditCard, Package, Palette as PaletteIcon } from '@lucide/vue'

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

// --- Interactive badge sample ---
const badgeStates = [
  { label: 'draft', tone: 'amber' },
  { label: 'published', tone: 'green' },
  { label: 'pending', tone: 'amber' },
  { label: 'approved', tone: 'green' },
  { label: 'rejected', tone: 'rose' },
  { label: 'archived', tone: 'slate' },
  { label: 'dikirim', tone: 'sky' },
]

// --- Motion demo ---
const spinning = ref(false)
function triggerSpin() {
  spinning.value = true
  setTimeout(() => (spinning.value = false), 2000)
}

// --- Input demo state ---
const demoInput = ref('')
const demoTextarea = ref('')
</script>

<template>
  <section>
    <AdminPageHeader
      title="Design System"
      subtitle="Living reference untuk CLAUDE.md §26. Klik swatch/token untuk copy classname."
    />

    <!-- Table of contents -->
    <nav class="mb-10 grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs font-medium">
      <a href="#palet" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 transition-colors">Palet warna</a>
      <a href="#typography" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 transition-colors">Tipografi</a>
      <a href="#buttons" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 transition-colors">Buttons</a>
      <a href="#inputs" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 transition-colors">Inputs</a>
      <a href="#badges" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 transition-colors">Badges</a>
      <a href="#cards" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 transition-colors">Cards</a>
      <a href="#icons" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 transition-colors">Icons</a>
      <a href="#motion" class="rounded-md border border-hairline bg-canvas px-3 py-2 text-ink-700 hover:border-ink-300 hover:text-brand-500 transition-colors">Motion</a>
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
          :class="[s.className, 'group aspect-square rounded-md p-2 text-left text-[10px] font-medium ring-1 ring-inset ring-black/5 hover:ring-black/20 transition-all flex flex-col justify-end']"
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
          :class="[s.className, 'group aspect-square rounded-md p-2 text-left text-[10px] font-medium ring-1 ring-inset ring-black/5 hover:ring-black/20 transition-all flex flex-col justify-end']"
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
          :class="[s.className, 'group aspect-square rounded-md p-2 text-left text-[10px] font-medium ring-1 ring-inset ring-black/5 hover:ring-black/20 transition-all flex flex-col justify-end']"
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
            <button class="inline-flex items-center rounded-md border border-hairline bg-canvas px-4 py-2 text-sm font-semibold text-ink-500 hover:text-ink-900 transition-colors">
              Ghost link
            </button>
          </div>
        </div>

        <div>
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">Destructive (pakai brand-500, bukan rose)</p>
          <div class="flex flex-wrap gap-3 items-center">
            <button class="inline-flex items-center rounded-md border border-brand-200 bg-canvas px-4 py-2 text-sm font-semibold text-brand-700 hover:bg-brand-50 hover:border-brand-300 transition-colors">
              Hapus
            </button>
            <button class="inline-flex items-center rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 transition-colors">
              Konfirmasi hapus
            </button>
          </div>
        </div>

        <div>
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">With icon (Lucide monoline)</p>
          <div class="flex flex-wrap gap-3 items-center">
            <button class="inline-flex items-center gap-2 rounded-md bg-ink-950 px-4 py-2 text-sm font-semibold text-canvas hover:bg-ink-900 transition-colors">
              <CreditCard class="h-4 w-4" :stroke-width="1.75" />
              Verifikasi bukti
            </button>
          </div>
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
            />
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
            />
            <p class="mt-1 text-xs text-brand-700">Slug hanya boleh a-z, 0-9, dan dash.</p>
          </div>

          <div>
            <label class="block text-sm font-medium text-ink-500">Disabled</label>
            <input
              type="text"
              value="Read-only value"
              disabled
              class="mt-1 block w-full rounded-md border border-hairline bg-canvas-alt px-3 py-2 text-sm text-ink-500 cursor-not-allowed"
            />
          </div>
        </div>
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
    </section>

    <!-- ================================= Cards ================================= -->
    <section id="cards" class="mb-16 scroll-mt-20">
      <h2 class="font-serif text-xl md:text-2xl font-semibold tracking-tight text-ink-950">Cards</h2>
      <div class="mt-6 grid gap-4 md:grid-cols-3">
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <h3 class="text-sm font-semibold text-ink-900">Static card</h3>
          <p class="mt-1 text-xs text-ink-500 leading-relaxed">Container info yang tidak diklik. Default state — no shadow, hairline border.</p>
        </div>

        <a href="#" class="group rounded-lg border border-hairline bg-canvas p-6 hover:border-ink-300 transition-colors">
          <h3 class="text-sm font-semibold text-ink-900 group-hover:text-brand-500 transition-colors">Interactive link card</h3>
          <p class="mt-1 text-xs text-ink-500 leading-relaxed">Hover: border shift to ink-300, title shift to brand-500.</p>
        </a>

        <div class="rounded-lg bg-ink-950 p-6 text-canvas">
          <h3 class="font-serif text-lg font-semibold">Dark card</h3>
          <p class="mt-1 text-xs text-ink-300 leading-relaxed">Untuk highlight premium — kombinasi dengan aksen <span class="text-gold-400">gold-400</span>.</p>
        </div>
      </div>
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
            class="inline-flex items-center gap-2 rounded-md bg-ink-950 px-4 py-2 text-sm font-semibold text-canvas hover:bg-ink-900 transition-colors"
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

    <div class="rounded-lg border border-gold-200 bg-gold-50 p-4 text-xs text-gold-900 leading-relaxed">
      <p><strong>Update flow (§26.11):</strong> tambah pattern baru di halaman ini + di CLAUDE.md §26 dalam PR yang sama. Kalau halaman ini dan implementasi drift → halaman ini yang benar; implementasi refactor.</p>
    </div>
  </section>
</template>
