<script setup lang="ts">
/**
 * ProcessLightbox — modal galeri untuk `ProcessGallery` (deliverable §2 brief).
 *
 * Aksesibilitas wajib (tidak boleh dikorbankan demi animasi):
 *  - Esc menutup, panah kiri/kanan navigasi, klik backdrop menutup (`@click.self`
 *    di root overlay — cuma trigger kalau target klik = overlay itu sendiri,
 *    bukan bubble dari konten).
 *  - Focus trap manual saat terbuka (Tab/Shift+Tab dikunci di dalam dialog).
 *  - Focus dikembalikan ke pemicu saat ditutup — dijamin oleh caller (`ProcessGallery`)
 *    yang eksplisit `.focus()` tombol pemicu sebelum membuka (Safari tidak selalu
 *    memberi focus otomatis ke <button> saat diklik, jadi tidak bisa mengandalkan
 *    `document.activeElement` semata).
 *
 * Motion: root overlay ADALAH satu-satunya child langsung `<AnimatePresence>`
 * (backdrop dim + konten dianimasikan sebagai satu blok fade+scale halus — scale
 * kecil di layer solid-color penuh viewport tidak terlihat sebagai artefak).
 * Exit lebih cepat dari enter (150ms vs 300ms) sesuai brief §motion poin 5.
 */
import { ChevronLeft, ChevronRight, X } from '@lucide/vue'
import { AnimatePresence, motion } from 'motion-v'

interface LightboxItem {
  src: string
  alt: string
  caption: string
}

const props = defineProps<{
  open: boolean
  items: LightboxItem[]
  index: number
}>()

const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'update:index', v: number): void
}>()

const prefersReduced = usePrefersReducedMotion()
const dialogRef = ref<HTMLElement | null>(null)

// Elemen yang memicu pembukaan, disimpan supaya focus bisa dikembalikan ke sana
// saat modal ditutup. Tanpa ini focus jatuh ke <body> dan pengguna keyboard
// kehilangan posisinya di galeri — harus tab dari awal dokumen lagi.
let triggerEl: HTMLElement | null = null

const current = computed(() => props.items[props.index])

function close() {
  emit('update:open', false)
}
function prev() {
  if (props.items.length === 0) return
  emit('update:index', (props.index - 1 + props.items.length) % props.items.length)
}
function next() {
  if (props.items.length === 0) return
  emit('update:index', (props.index + 1) % props.items.length)
}

function focusableEls(): HTMLElement[] {
  const root = dialogRef.value
  if (!root) return []
  return Array.from(
    root.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])',
    ),
  )
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    close()
    return
  }
  if (e.key === 'ArrowLeft') {
    e.preventDefault()
    prev()
    return
  }
  if (e.key === 'ArrowRight') {
    e.preventDefault()
    next()
    return
  }
  if (e.key === 'Tab') {
    const els = focusableEls()
    if (els.length === 0) return
    const first = els[0]
    const last = els[els.length - 1]
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }
}

watch(
  () => props.open,
  async (isOpen) => {
    if (!import.meta.client) return
    if (isOpen) {
      triggerEl = document.activeElement as HTMLElement | null
      // Kunci scroll di <html> DAN <body>. Body saja tidak cukup: scroll
      // container halaman ini adalah documentElement, jadi `body{overflow:hidden}`
      // sendirian tidak menghentikan apa pun — sudah diverifikasi, halaman tetap
      // ter-scroll di belakang modal yang terbuka.
      document.documentElement.style.overflow = 'hidden'
      document.body.style.overflow = 'hidden'
      window.addEventListener('keydown', onKeydown)
      await nextTick()
      dialogRef.value?.focus()
    } else {
      document.documentElement.style.overflow = ''
      document.body.style.overflow = ''
      window.removeEventListener('keydown', onKeydown)
      triggerEl?.focus?.()
      triggerEl = null
    }
  },
)

onUnmounted(() => {
  if (import.meta.client) {
    // Modal bisa ter-unmount saat masih terbuka (mis. navigasi rute) — jangan
    // tinggalkan body terkunci scroll-nya.
    document.documentElement.style.overflow = ''
    document.body.style.overflow = ''
    window.removeEventListener('keydown', onKeydown)
  }
})
</script>

<template>
  <Teleport to="body">
    <AnimatePresence>
      <motion.div
        v-if="open && current"
        key="process-lightbox"
        class="fixed inset-0 z-50 flex items-center justify-center bg-ink-950/90 p-4 md:p-8"
        :initial="{ opacity: 0, scale: prefersReduced ? 1 : 0.97 }"
        :animate="{
          opacity: 1,
          scale: 1,
          transition: { duration: prefersReduced ? 0 : 0.3, ease: [0.22, 1, 0.36, 1] },
        }"
        :exit="{
          opacity: 0,
          scale: prefersReduced ? 1 : 0.98,
          transition: { duration: prefersReduced ? 0 : 0.15, ease: [0.22, 1, 0.36, 1] },
        }"
        @click.self="close"
      >
        <div
          ref="dialogRef"
          role="dialog"
          aria-modal="true"
          :aria-label="current.alt"
          tabindex="-1"
          class="relative w-full max-w-4xl focus:outline-none"
        >
          <img
            :src="current.src"
            :alt="current.alt"
            width="1000"
            height="562"
            class="w-full rounded-lg object-contain"
          >

          <div class="mt-4 text-center">
            <p class="text-sm text-canvas/80">{{ current.caption }}</p>
            <p class="mt-1 font-mono text-xs text-canvas/50">
              {{ String(index + 1).padStart(2, '0') }} / {{ String(items.length).padStart(2, '0') }}
            </p>
          </div>

          <button
            type="button"
            class="absolute -top-3 -right-3 inline-flex h-10 w-10 items-center justify-center rounded-md border border-canvas/20 bg-ink-950 text-canvas transition-colors hover:bg-ink-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
            aria-label="Tutup"
            @click="close"
          >
            <X class="h-5 w-5" :stroke-width="1.5" />
          </button>

          <button
            v-if="items.length > 1"
            type="button"
            class="absolute left-2 top-1/2 -translate-y-1/2 inline-flex h-10 w-10 items-center justify-center rounded-md border border-canvas/20 bg-ink-950/80 text-canvas transition-colors hover:bg-ink-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
            aria-label="Gambar sebelumnya"
            @click="prev"
          >
            <ChevronLeft class="h-5 w-5" :stroke-width="1.5" />
          </button>
          <button
            v-if="items.length > 1"
            type="button"
            class="absolute right-2 top-1/2 -translate-y-1/2 inline-flex h-10 w-10 items-center justify-center rounded-md border border-canvas/20 bg-ink-950/80 text-canvas transition-colors hover:bg-ink-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
            aria-label="Gambar selanjutnya"
            @click="next"
          >
            <ChevronRight class="h-5 w-5" :stroke-width="1.5" />
          </button>
        </div>
      </motion.div>
    </AnimatePresence>
  </Teleport>
</template>
