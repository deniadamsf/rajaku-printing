<script setup lang="ts">
/**
 * GalleryLightbox — modal lihat-besar untuk galeri showcase.
 *
 * Controlled via `v-model:index` (number | null; null = tertutup). Kenapa
 * bukan boolean `open` + `index` terpisah: satu sumber kebenaran, tidak ada
 * state transisi ambigu (mis. open=true tapi index=null).
 *
 * Aksesibilitas (wajib per brief, tidak boleh dikorbankan demi animasi):
 *  - Esc menutup, panah kiri/kanan navigasi (wrap-around).
 *  - Klik backdrop menutup.
 *  - Focus trap manual di dalam panel (tidak ada @vueuse/integrations di
 *    project ini untuk `useFocusTrap`, jadi query focusable element sendiri
 *    — pola sama sederhananya dengan yang dipakai di banyak modal aksesibel).
 *  - Focus balik ke elemen pemicu: `document.activeElement` ditangkap saat
 *    modal dibuka (elemen itu adalah thumbnail yang baru diklik/di-Enter),
 *    dikembalikan fokusnya saat modal ditutup.
 *  - Body scroll dikunci selama modal terbuka.
 *
 * Motion: AnimatePresence + motion-v. Enter fade+scale halus (~260ms, easing
 * editorial), exit LEBIH CEPAT (~140ms) — lihat CLAUDE.md prinsip motion §5.
 * Hormati prefers-reduced-motion lewat `usePrefersReducedMotion()`.
 */
import { AnimatePresence, motion } from 'motion-v'
import { ChevronLeft, ChevronRight, X } from '@lucide/vue'

export interface LightboxItem {
  src: string
  alt: string
  title?: string
  desc?: string
}

const props = defineProps<{
  items: LightboxItem[]
  index: number | null
}>()

const emit = defineEmits<{
  (e: 'update:index', v: number | null): void
}>()

const prefersReducedMotion = usePrefersReducedMotion()

const panelRef = ref<HTMLElement | null>(null)
const closeBtnRef = ref<HTMLElement | null>(null)
let triggerEl: HTMLElement | null = null

const isOpen = computed(() => props.index !== null)
const current = computed(() => (props.index !== null ? props.items[props.index] : null))

function close() {
  emit('update:index', null)
}

function go(delta: number) {
  if (props.index === null) return
  const total = props.items.length
  const next = (props.index + delta + total) % total
  emit('update:index', next)
}

function getFocusable(container: HTMLElement): HTMLElement[] {
  return Array.from(
    container.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((el) => el.offsetParent !== null)
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    close()
    return
  }
  if (e.key === 'ArrowLeft') {
    e.preventDefault()
    go(-1)
    return
  }
  if (e.key === 'ArrowRight') {
    e.preventDefault()
    go(1)
    return
  }
  if (e.key === 'Tab') {
    const container = panelRef.value
    if (!container) return
    const focusables = getFocusable(container)
    if (focusables.length === 0) return
    const first = focusables[0]
    const last = focusables[focusables.length - 1]
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }
}

watch(isOpen, async (open) => {
  if (open) {
    triggerEl = document.activeElement as HTMLElement | null
    // Kunci scroll di <html> DAN <body>. Body saja tidak cukup: scroll container
    // halaman ini adalah documentElement, jadi `body{overflow:hidden}` sendirian
    // tidak menghentikan apa pun — halaman tetap ter-scroll di belakang modal.
    document.documentElement.style.overflow = 'hidden'
    document.body.style.overflow = 'hidden'
    window.addEventListener('keydown', onKeydown)
    await nextTick()
    closeBtnRef.value?.focus()
  } else {
    document.documentElement.style.overflow = ''
    document.body.style.overflow = ''
    window.removeEventListener('keydown', onKeydown)
    triggerEl?.focus?.()
    triggerEl = null
  }
})

onUnmounted(() => {
  // Modal bisa ter-unmount saat masih terbuka (mis. navigasi rute) — jangan
  // tinggalkan dokumen terkunci scroll-nya.
  document.documentElement.style.overflow = ''
  document.body.style.overflow = ''
  window.removeEventListener('keydown', onKeydown)
})

const enterTransition = computed(() =>
  prefersReducedMotion.value
    ? { duration: 0 }
    : { duration: 0.28, ease: [0.22, 1, 0.36, 1] as [number, number, number, number] },
)
const exitTransition = computed(() =>
  prefersReducedMotion.value ? { duration: 0 } : { duration: 0.14, ease: 'easeIn' as const },
)
</script>

<template>
  <Teleport to="body">
    <AnimatePresence>
      <div v-if="isOpen && current" class="fixed inset-0 z-50 flex items-center justify-center p-4 md:p-8">
        <motion.div
          class="absolute inset-0 bg-ink-950/90"
          aria-hidden="true"
          :initial="{ opacity: 0 }"
          :animate="{ opacity: 1 }"
          :exit="{ opacity: 0 }"
          :transition="enterTransition"
          @click="close"
        />

        <motion.div
          ref="panelRef"
          role="dialog"
          aria-modal="true"
          :aria-label="current.title || 'Pratinjau gambar'"
          class="relative flex max-h-full w-full max-w-4xl flex-col items-center"
          :initial="{ opacity: 0, scale: 0.96 }"
          :animate="{ opacity: 1, scale: 1 }"
          :exit="{ opacity: 0, scale: 0.98, transition: exitTransition }"
          :transition="enterTransition"
        >
          <button
            ref="closeBtnRef"
            type="button"
            class="absolute -top-2 right-0 z-10 inline-flex h-10 w-10 items-center justify-center rounded-md text-canvas/80 transition-colors hover:text-canvas focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950 md:-top-3 md:right-0"
            aria-label="Tutup pratinjau"
            @click="close"
          >
            <X class="h-6 w-6" :stroke-width="1.5" />
          </button>

          <div class="relative w-full overflow-hidden rounded-lg bg-canvas-alt">
            <img
              :src="current.src"
              :alt="current.alt"
              width="1000"
              height="562"
              class="max-h-[70vh] w-full object-contain"
            >

            <template v-if="items.length > 1">
              <button
                type="button"
                class="absolute left-2 top-1/2 -translate-y-1/2 inline-flex h-10 w-10 items-center justify-center rounded-md bg-ink-950/40 text-canvas backdrop-blur transition-colors hover:bg-ink-950/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
                aria-label="Gambar sebelumnya"
                @click="go(-1)"
              >
                <ChevronLeft class="h-5 w-5" :stroke-width="1.5" />
              </button>
              <button
                type="button"
                class="absolute right-2 top-1/2 -translate-y-1/2 inline-flex h-10 w-10 items-center justify-center rounded-md bg-ink-950/40 text-canvas backdrop-blur transition-colors hover:bg-ink-950/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
                aria-label="Gambar berikutnya"
                @click="go(1)"
              >
                <ChevronRight class="h-5 w-5" :stroke-width="1.5" />
              </button>
            </template>
          </div>

          <div v-if="current.title || current.desc" class="mt-4 max-w-lg text-center">
            <p v-if="current.title" class="text-sm font-semibold text-canvas">{{ current.title }}</p>
            <p v-if="current.desc" class="mt-1 text-xs leading-relaxed text-canvas/70">{{ current.desc }}</p>
          </div>

          <p v-if="items.length > 1" class="mt-2 font-mono text-[10px] text-canvas/50">
            {{ (index ?? 0) + 1 }} / {{ items.length }}
          </p>
        </motion.div>
      </div>
    </AnimatePresence>
  </Teleport>
</template>
