<script setup lang="ts">
/**
 * ShowcaseBannerViewer — mockup 3D interaktif (CLAUDE.md §16 poin 2).
 *
 * Geometri sengaja sederhana: satu `PlaneGeometry` dengan texture map "contoh
 * desain banner" yang di-generate lewat Canvas2D (karena belum ada aset foto
 * desain final asli). User memutar banner dengan drag pointer; TIDAK
 * auto-rotate (menghormati prefers-reduced-motion & supaya terasa seperti
 * kontrol produk, bukan dekorasi).
 *
 * Interaktivitas (naik kelas dari versi awal — single plane statis):
 *  - Pilihan **contoh desain** (preset): 3 variasi komposisi/warna yang
 *    tetap dalam palet brand (crimson/gold/ink/canvas), digambar ulang ke
 *    Canvas2D — bukan aset foto (belum tersedia file desain asli).
 *  - Pilihan **rasio/ukuran banner**: lanskap 3:1, potret 1:2, persegi 1:1 —
 *    mengubah geometri plane supaya user membayangkan proporsi banner asli.
 *  - Ganti preset/rasio membangun `CanvasTexture` baru dan men-dispose yang
 *    lama SETELAHNYA (bukan sebelum) supaya tidak ada frame flicker ke
 *    material kosong.
 *
 * Wajib lazy-load: <ClientOnly> + IntersectionObserver (`useIntersectionObserver`
 * dari @vueuse/core) — scene TresJS baru di-mount saat wrapper betul-betul masuk
 * viewport. Render mode `on-demand` supaya GPU tidak kerja terus-menerus saat
 * banner diam.
 *
 * Fallback WebGL-tidak-didukung ditampilkan eksplisit (bukan blank/crash).
 * Cleanup saat unmount: texture (manual dispose), IntersectionObserver `stop()`;
 * geometry/material bawaan TresJS di-dispose otomatis oleh @tresjs/core saat
 * node dilepas dari scene.
 */
import { CanvasTexture, DoubleSide, SRGBColorSpace, Vector3 } from 'three'
import { RotateCcw } from '@lucide/vue'

// Instance Vector3 langsung (bukan array literal) — tipe prop `position` bawaan
// TresJS/vue-tsc di versi ini strict ke `Vector3 | Readonly<Vector3 | undefined>`,
// array `[x,y,z]` tidak lolos type-check meski diterima runtime.
const CAMERA_POSITION = new Vector3(0, 0, 2.5)

const RATIOS = [
  { key: 'landscape', label: 'Lanskap 3:1', aspect: 3 },
  { key: 'portrait', label: 'Potret 1:2', aspect: 0.5 },
  { key: 'square', label: 'Persegi 1:1', aspect: 1 },
] as const
type RatioKey = (typeof RATIOS)[number]['key']

const PRESETS = [
  { key: 'promo', label: 'Promo Toko' },
  { key: 'event', label: 'Event & Acara' },
  { key: 'ucapan', label: 'Ucapan' },
] as const
type PresetKey = (typeof PRESETS)[number]['key']

const selectedRatio = ref<RatioKey>('landscape')
const selectedPreset = ref<PresetKey>('promo')

const wrapperRef = ref<HTMLElement | null>(null)
const isVisible = ref(false)
const webglSupported = ref(true)
const texture = shallowRef<CanvasTexture | null>(null)

const rotX = ref(-0.12)
const rotY = ref(0.4)
const DEFAULT_ROT_X = -0.12
const DEFAULT_ROT_Y = 0.4

let dragging = false
let lastX = 0
let lastY = 0

function clamp(v: number, min: number, max: number) {
  return Math.max(min, Math.min(max, v))
}

function onPointerDown(e: PointerEvent) {
  dragging = true
  lastX = e.clientX
  lastY = e.clientY
  ;(e.target as HTMLElement)?.setPointerCapture?.(e.pointerId)
}
function onPointerMove(e: PointerEvent) {
  if (!dragging) return
  const dx = e.clientX - lastX
  const dy = e.clientY - lastY
  lastX = e.clientX
  lastY = e.clientY
  // rotY WAJIB di-clamp seperti rotX. Tanpa batas, drag ~150px sudah memutar
  // banner ke ~90° (tepat menyamping, tipis seperti garis) dan lewat itu yang
  // tampil sisi belakang yang tercermin — preview jadi tak terpakai untuk
  // tujuannya, yaitu menilai desain sebelum order (§16 poin 2). JANGAN dihapus.
  rotY.value = clamp(rotY.value + dx * 0.008, -1, 1)
  rotX.value = clamp(rotX.value + dy * 0.008, -0.7, 0.7)
}
function onPointerUp() {
  dragging = false
}
function resetView() {
  rotX.value = DEFAULT_ROT_X
  rotY.value = DEFAULT_ROT_Y
}

// Ukuran plane (world unit) menyesuaikan rasio terpilih, sisi terpanjang
// dikunci ke `maxDim` supaya widget tetap pas di frame kamera yang sama
// untuk ketiga rasio.
const planeSize = computed<[number, number]>(() => {
  const aspect = RATIOS.find((r) => r.key === selectedRatio.value)!.aspect
  const maxDim = 1.6
  return aspect >= 1 ? [maxDim, maxDim / aspect] : [maxDim * aspect, maxDim]
})

type Drawer = (ctx: CanvasRenderingContext2D, w: number, h: number) => void

/**
 * Gambar teks di tengah, font otomatis dikecilkan supaya muat `maxWidth`.
 *
 * Ukuran font di drawer dihitung dari `min(w, h)`. Pada rasio potret (kanvas
 * 512×1024) dan persegi, `min` = lebar, sehingga judul panjang seperti
 * "UNDANGAN ACARA" meluber keluar tepi banner. Mengukur lebar teks lebih dulu
 * jauh lebih andal daripada menebak koefisien font per rasio.
 */
function drawFittedText(
  ctx: CanvasRenderingContext2D,
  text: string,
  cx: number,
  cy: number,
  maxWidth: number,
  size: number,
  font: (px: number) => string,
) {
  ctx.font = font(size)
  const measured = ctx.measureText(text).width
  if (measured > maxWidth) {
    ctx.font = font(Math.max(8, Math.floor(size * (maxWidth / measured))))
  }
  ctx.fillText(text, cx, cy)
}

const PRESET_DRAWERS: Record<PresetKey, Drawer> = {
  promo(ctx, w, h) {
    const gradient = ctx.createLinearGradient(0, 0, w, h)
    gradient.addColorStop(0, '#8B1A1A')
    gradient.addColorStop(1, '#2B0505')
    ctx.fillStyle = gradient
    ctx.fillRect(0, 0, w, h)

    const min = Math.min(w, h)
    const margin = min * 0.05
    ctx.strokeStyle = 'rgba(250, 250, 249, 0.35)'
    ctx.lineWidth = Math.max(4, min * 0.012)
    ctx.strokeRect(margin, margin, w - margin * 2, h - margin * 2)

    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillStyle = '#FAFAF9'
    const safe = w - margin * 2 - min * 0.06
    const titleSize = Math.round(min * 0.13)
    drawFittedText(
      ctx, 'RAJAKU PRINTING', w / 2, h / 2 - titleSize * 0.35, safe, titleSize,
      (p) => `600 ${p}px Georgia, "Times New Roman", serif`,
    )

    ctx.fillStyle = '#C9A44A'
    const subSize = Math.round(titleSize * 0.4)
    drawFittedText(
      ctx, 'PROMO SPESIAL BULAN INI', w / 2, h / 2 + titleSize * 0.55, safe, subSize,
      (p) => `400 ${p}px Arial, sans-serif`,
    )
  },
  event(ctx, w, h) {
    const gradient = ctx.createLinearGradient(0, 0, 0, h)
    gradient.addColorStop(0, '#171717')
    gradient.addColorStop(1, '#0A0A0A')
    ctx.fillStyle = gradient
    ctx.fillRect(0, 0, w, h)

    const min = Math.min(w, h)
    const gold = '#B08D57'
    const pad = min * 0.08
    ctx.strokeStyle = gold
    ctx.lineWidth = Math.max(3, min * 0.01)
    ctx.beginPath()
    ctx.moveTo(pad, pad)
    ctx.lineTo(w - pad, pad)
    ctx.moveTo(pad, h - pad)
    ctx.lineTo(w - pad, h - pad)
    ctx.stroke()

    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillStyle = '#FAFAF9'
    const safe = w - pad * 2 - min * 0.04
    const titleSize = Math.round(min * 0.12)
    drawFittedText(
      ctx, 'UNDANGAN ACARA', w / 2, h / 2 - titleSize * 0.3, safe, titleSize,
      (p) => `600 ${p}px Georgia, serif`,
    )

    ctx.fillStyle = gold
    const subSize = Math.round(titleSize * 0.36)
    drawFittedText(
      ctx, 'Dicetak oleh Rajaku Printing', w / 2, h / 2 + titleSize * 0.55, safe, subSize,
      (p) => `400 ${p}px Arial, sans-serif`,
    )
  },
  ucapan(ctx, w, h) {
    ctx.fillStyle = '#FAFAF9'
    ctx.fillRect(0, 0, w, h)

    const min = Math.min(w, h)
    const pad = min * 0.06
    ctx.strokeStyle = '#B08D57'
    ctx.lineWidth = Math.max(3, min * 0.008)
    ctx.strokeRect(pad, pad, w - pad * 2, h - pad * 2)

    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillStyle = '#8B1A1A'
    const safe = w - pad * 2 - min * 0.04
    const titleSize = Math.round(min * 0.1)
    drawFittedText(
      ctx, 'SELAMAT & SUKSES', w / 2, h / 2 - titleSize * 0.3, safe, titleSize,
      (p) => `600 ${p}px Georgia, serif`,
    )

    ctx.fillStyle = '#171717'
    const subSize = Math.round(titleSize * 0.34)
    drawFittedText(
      ctx, 'Rajaku Printing', w / 2, h / 2 + titleSize * 0.5, safe, subSize,
      (p) => `400 ${p}px Arial, sans-serif`,
    )
  },
}

// Bangun texture "contoh desain banner" di canvas 2D — palet brand saja
// (§26.8, bukan foto asli, bukan warna-warni). Resolusi kanvas mengikuti
// rasio terpilih supaya teks/border tidak gepeng.
function buildBannerTexture(preset: PresetKey, aspect: number): CanvasTexture {
  const canvas = document.createElement('canvas')
  const base = 1024
  if (aspect >= 1) {
    canvas.width = base
    canvas.height = Math.round(base / aspect)
  } else {
    canvas.height = base
    canvas.width = Math.round(base * aspect)
  }
  const ctx = canvas.getContext('2d')!
  PRESET_DRAWERS[preset](ctx, canvas.width, canvas.height)

  const tex = new CanvasTexture(canvas)
  tex.colorSpace = SRGBColorSpace
  tex.needsUpdate = true
  return tex
}

// Ganti texture: buat yang baru dulu, baru dispose yang lama — mencegah
// frame kosong sekejap kalau build sempat lambat. Sesuai catatan brief:
// "texture lama WAJIB di-dispose supaya tidak bocor memori GPU".
function rebuildTexture() {
  const aspect = RATIOS.find((r) => r.key === selectedRatio.value)!.aspect
  const next = buildBannerTexture(selectedPreset.value, aspect)
  const prev = texture.value
  texture.value = next
  prev?.dispose()
}

onMounted(() => {
  try {
    const c = document.createElement('canvas')
    const gl = c.getContext('webgl2') || c.getContext('webgl') || c.getContext('experimental-webgl')
    webglSupported.value = !!gl
  } catch {
    webglSupported.value = false
  }
})

const { stop } = useIntersectionObserver(
  wrapperRef,
  ([entry]) => {
    if (entry?.isIntersecting) {
      isVisible.value = true
      stop()
    }
  },
  { threshold: 0.15 },
)

const shouldRenderScene = computed(() => isVisible.value && webglSupported.value)

watch(shouldRenderScene, (v) => {
  if (v && !texture.value) {
    rebuildTexture()
  }
})

watch([selectedPreset, selectedRatio], () => {
  if (shouldRenderScene.value) {
    rebuildTexture()
  }
})

onUnmounted(() => {
  texture.value?.dispose()
  texture.value = null
})
</script>

<template>
  <div class="space-y-4">
    <!-- Kontrol: rasio & contoh desain -->
    <div class="flex flex-wrap items-start gap-4">
      <div class="flex flex-col gap-1.5">
        <span class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Rasio banner</span>
        <div class="inline-flex rounded-md border border-hairline bg-canvas p-0.5">
          <button
            v-for="r in RATIOS"
            :key="r.key"
            type="button"
            class="rounded px-2.5 py-1.5 text-xs font-medium transition-colors duration-200 ease-out focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
            :class="selectedRatio === r.key ? 'bg-brand-500 text-canvas' : 'text-ink-600 hover:text-ink-950'"
            :aria-pressed="selectedRatio === r.key"
            @click="selectedRatio = r.key"
          >
            {{ r.label }}
          </button>
        </div>
      </div>

      <div class="flex flex-col gap-1.5">
        <span class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Contoh desain</span>
        <div class="inline-flex rounded-md border border-hairline bg-canvas p-0.5">
          <button
            v-for="p in PRESETS"
            :key="p.key"
            type="button"
            class="rounded px-2.5 py-1.5 text-xs font-medium transition-colors duration-200 ease-out focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
            :class="selectedPreset === p.key ? 'bg-brand-500 text-canvas' : 'text-ink-600 hover:text-ink-950'"
            :aria-pressed="selectedPreset === p.key"
            @click="selectedPreset = p.key"
          >
            {{ p.label }}
          </button>
        </div>
      </div>
    </div>

    <div
      ref="wrapperRef"
      class="relative aspect-[4/3] w-full touch-none select-none overflow-hidden rounded-lg border border-hairline bg-canvas-alt"
      :class="shouldRenderScene ? 'cursor-grab active:cursor-grabbing' : ''"
      @pointerdown="onPointerDown"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp"
      @pointerleave="onPointerUp"
    >
      <ClientOnly>
        <template v-if="shouldRenderScene && texture">
          <TresCanvas clear-color="#F5F5F4" render-mode="on-demand" :antialias="true" class="h-full w-full">
            <TresPerspectiveCamera make-default :position="CAMERA_POSITION" :fov="42" />
            <TresMesh :rotation="[rotX, rotY, 0]">
              <TresPlaneGeometry :args="planeSize" />
              <TresMeshBasicMaterial :map="texture" :side="DoubleSide" />
            </TresMesh>
          </TresCanvas>

          <button
            type="button"
            class="absolute bottom-3 right-3 inline-flex items-center gap-1.5 rounded-md border border-hairline bg-canvas/90 px-3 py-1.5 text-xs font-medium text-ink-700 backdrop-blur transition-colors hover:border-ink-300 hover:text-ink-950 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
            @click="resetView"
          >
            <RotateCcw class="h-3.5 w-3.5" :stroke-width="1.75" />
            Reset
          </button>
        </template>

        <!-- WebGL tidak didukung device ini — fallback eksplisit, bukan blank. -->
        <div
          v-else-if="!webglSupported"
          class="flex h-full flex-col items-center justify-center gap-2 px-6 text-center"
        >
          <p class="text-sm font-medium text-ink-700">Preview 3D tidak tersedia di perangkat ini</p>
          <p class="text-xs leading-relaxed text-ink-500">
            Browser Anda belum mendukung WebGL. Hubungi kami langsung untuk melihat contoh
            desain banner.
          </p>
        </div>

        <!-- Belum masuk viewport / belum siap — placeholder ringan tanpa memuat scene. -->
        <div v-else class="flex h-full items-center justify-center">
          <div class="h-8 w-8 animate-pulse rounded-full border-2 border-ink-300 border-t-transparent" />
        </div>

        <template #fallback>
          <div class="flex h-full items-center justify-center">
            <div class="h-8 w-8 animate-pulse rounded-full border-2 border-ink-300 border-t-transparent" />
          </div>
        </template>
      </ClientOnly>

      <p
        v-if="shouldRenderScene && texture"
        class="pointer-events-none absolute left-3 top-3 rounded-full bg-canvas/90 px-2.5 py-1 text-[10px] font-medium uppercase tracking-[0.1em] text-ink-500 backdrop-blur"
      >
        Geser untuk memutar
      </p>
    </div>
  </div>
</template>
