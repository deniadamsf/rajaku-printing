<script setup lang="ts">
/**
 * HeroScrollScrub — hero cinematic scroll-scrub desktop (CLAUDE.md §16 poin 1).
 *
 * Teknik: canvas image-sequence (100 frame WebP di /hero/frames) digambar sesuai
 * posisi scroll pakai GSAP ScrollTrigger (`scrub: true`). Bukan video/3D asli.
 *
 * - Frame pertama di-load & digambar segera supaya tidak blank; poster.webp jadi
 *   background sampai frame siap (fade-out setelah frame 1 tergambar).
 * - Sisa frame di-preload progresif di background (non-blocking).
 * - rAF-throttled per scroll update, canvas resize mengikuti devicePixelRatio.
 * - ScrollTrigger di-cleanup penuh saat unmount.
 * - Wajib dipanggil di dalam <ClientOnly> oleh parent (halaman index.vue), dan
 *   hanya untuk desktop (§18) — komponen ini sendiri tidak melakukan device check.
 * - Hormati `prefers-reduced-motion`: kalau reduced, tampilkan frame pertama/poster
 *   statis saja tanpa register ScrollTrigger.
 */
import { ArrowRight, Search } from '@lucide/vue'

const TOTAL_FRAMES = 100
// Poster fallback bisa diganti admin (/admin/site-media, slot hero_poster_desktop)
// tanpa deploy ulang. Frame sequence (di bawah) tetap aset statis — tidak ada di
// slot registry, cuma poster yang overridable.
const { resolve: resolveMedia } = useSiteMedia()
const POSTER = computed(() => resolveMedia('hero_poster_desktop'))
const framePath = (i: number) => `/hero/frames/f_${String(i).padStart(3, '0')}.webp`

const sectionRef = ref<HTMLElement | null>(null)
const canvasEl = ref<HTMLCanvasElement | null>(null)
const framesReady = ref(false)

let ctx: CanvasRenderingContext2D | null = null
const images: (HTMLImageElement | undefined)[] = new Array(TOTAL_FRAMES)
let currentIndex = 1
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let scrollTriggerInstance: any = null
let rafId: number | null = null
let pendingIndex: number | null = null
let reducedMotion = false

function loadImage(index: number): Promise<HTMLImageElement> {
  const cached = images[index - 1]
  if (cached) return Promise.resolve(cached)
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.decoding = 'async'
    img.onload = () => {
      images[index - 1] = img
      resolve(img)
    }
    img.onerror = () => reject(new Error(`gagal load frame ${index}`))
    img.src = framePath(index)
  })
}

function resizeCanvas() {
  const canvas = canvasEl.value
  if (!canvas) return
  const dpr = Math.min(window.devicePixelRatio || 1, 2)
  const rect = canvas.getBoundingClientRect()
  canvas.width = Math.max(1, Math.round(rect.width * dpr))
  canvas.height = Math.max(1, Math.round(rect.height * dpr))
  ctx = canvas.getContext('2d')
}

function drawFrame(index: number) {
  const canvas = canvasEl.value
  const img = images[index - 1]
  if (!canvas || !ctx || !img) return
  const cw = canvas.width
  const ch = canvas.height
  const cr = cw / ch
  const ir = img.width / img.height
  let sx = 0
  let sy = 0
  let sw = img.width
  let sh = img.height
  if (ir > cr) {
    sw = img.height * cr
    sx = (img.width - sw) / 2
  } else {
    sh = img.width / cr
    sy = (img.height - sh) / 2
  }
  ctx.clearRect(0, 0, cw, ch)
  ctx.drawImage(img, sx, sy, sw, sh, 0, 0, cw, ch)
}

function requestDraw(index: number) {
  pendingIndex = index
  if (rafId != null) return
  rafId = requestAnimationFrame(() => {
    rafId = null
    if (pendingIndex != null && images[pendingIndex - 1]) {
      drawFrame(pendingIndex)
      currentIndex = pendingIndex
    }
    pendingIndex = null
  })
}

function onResize() {
  resizeCanvas()
  drawFrame(currentIndex)
  scrollTriggerInstance?.refresh()
}

onMounted(async () => {
  reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches

  await nextTick()
  resizeCanvas()

  try {
    await loadImage(1)
    drawFrame(1)
    framesReady.value = true
  } catch {
    // Frame pertama gagal load — biarkan poster.webp tampil sebagai fallback.
    return
  }

  window.addEventListener('resize', onResize, { passive: true })

  if (reducedMotion) return // statis di frame 1, tanpa scrub

  // Preload sisa frame di background — tidak blocking interaksi/scroll awal.
  void (async () => {
    for (let i = 2; i <= TOTAL_FRAMES; i++) {
      try {
        await loadImage(i)
      } catch {
        // Lewati frame yang gagal — requestDraw akan skip kalau belum ter-load.
      }
    }
  })()

  const { gsap } = await import('gsap')
  const { ScrollTrigger } = await import('gsap/ScrollTrigger')
  gsap.registerPlugin(ScrollTrigger)

  scrollTriggerInstance = ScrollTrigger.create({
    trigger: sectionRef.value as Element,
    start: 'top top',
    end: 'bottom bottom',
    scrub: true,
    onUpdate: (self: { progress: number }) => {
      const idx = Math.min(
        TOTAL_FRAMES,
        Math.max(1, Math.round(self.progress * (TOTAL_FRAMES - 1)) + 1),
      )
      requestDraw(idx)
    },
  })
})

onUnmounted(() => {
  if (rafId != null) cancelAnimationFrame(rafId)
  window.removeEventListener('resize', onResize)
  scrollTriggerInstance?.kill()
  scrollTriggerInstance = null
})
</script>

<template>
  <section ref="sectionRef" class="relative h-[300vh]">
    <div class="sticky top-0 h-screen w-full overflow-hidden bg-ink-950">
      <!-- Poster — background sampai frame pertama siap -->
      <img
        :src="POSTER"
        alt="Proses cetak banner large-format Rajaku Printing"
        width="1280"
        height="720"
        fetchpriority="high"
        class="absolute inset-0 h-full w-full object-cover transition-opacity duration-500 ease-out"
        :class="framesReady ? 'opacity-0' : 'opacity-100'"
      />

      <!-- Canvas image-sequence, di-scrub oleh GSAP ScrollTrigger -->
      <canvas
        ref="canvasEl"
        class="absolute inset-0 h-full w-full transition-opacity duration-500 ease-out"
        :class="framesReady ? 'opacity-100' : 'opacity-0'"
        aria-hidden="true"
      />

      <!--
        Wash overlay monochrome (ink) supaya teks tetap terbaca.
        Nilai via- sengaja tinggi (50%): frame sequence ini bergerak dari body
        mesin yang gelap ke banner cetak merah-hijau yang terang, dan headline
        duduk di tengah viewport — scrim lemah bikin teks hilang di frame akhir.
        Jangan turunkan tanpa cek ulang kontras di frame ~60-100.

        Catatan saat mengubah nilai opacity di sini: kelas gradient bisa tampil di
        DOM tapi CSS-nya kosong kalau Tailwind belum sempat regenerate — scrim
        hilang diam-diam dan teks jadi tak terbaca tanpa error apa pun. Setelah
        mengubah, cek `getComputedStyle(el).backgroundImage` benar-benar berisi
        linear-gradient, jangan percaya kelasnya ada di DOM saja.
      -->
      <div class="absolute inset-0 bg-gradient-to-t from-ink-950/90 via-ink-950/50 to-ink-950/25" />
      <div class="absolute inset-0 bg-gradient-to-b from-ink-950/40 via-transparent to-transparent" />

      <div class="relative z-10 flex h-full flex-col items-center justify-end px-4 pb-24 text-center md:pb-32">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-canvas/70">
          Percetakan Banner &middot; Trenggalek
        </p>
        <h1
          class="mt-4 max-w-3xl text-5xl md:text-7xl font-serif font-semibold tracking-tight leading-[1.05] text-canvas"
        >
          Cetak Banner, <span class="text-gold-400">Presisi</span> Setiap Warna
        </h1>
        <p class="mx-auto mt-6 max-w-xl text-base leading-relaxed text-canvas/80">
          Order online, upload desain sendiri atau minta dibuatkan tim kami, lalu
          lacak progres cetaknya sampai siap diambil.
        </p>

        <div class="mt-10 flex flex-col sm:flex-row items-center justify-center gap-3">
          <NuxtLink
            to="/order"
            class="inline-flex w-full sm:w-auto items-center justify-center gap-2 rounded-md bg-brand-500 px-6 py-3 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
          >
            Order Banner
            <ArrowRight class="h-4 w-4" :stroke-width="1.5" />
          </NuxtLink>

          <NuxtLink
            to="/lacak"
            class="inline-flex w-full sm:w-auto items-center justify-center gap-2 rounded-md border border-canvas/25 bg-transparent px-6 py-3 text-sm font-semibold text-canvas transition-colors hover:bg-canvas/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
          >
            <Search class="h-4 w-4" :stroke-width="1.5" />
            Lacak Resi
          </NuxtLink>
        </div>
      </div>
    </div>
  </section>
</template>
