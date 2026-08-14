<script setup lang="ts">
/**
 * ShowcaseBannerViewer — mockup 3D interaktif (CLAUDE.md §16 poin 2).
 *
 * Geometri sengaja sederhana: satu `PlaneGeometry` dengan texture map "contoh
 * desain banner" yang di-generate lewat Canvas2D (karena belum ada aset foto
 * produk asli — lihat brief tugas). User memutar banner dengan drag pointer;
 * TIDAK auto-rotate (menghormati prefers-reduced-motion & supaya terasa seperti
 * kontrol produk, bukan dekorasi).
 *
 * Wajib lazy-load: <ClientOnly> + IntersectionObserver (`useIntersectionObserver`
 * dari @vueuse/core) — scene TresJS baru di-mount saat wrapper betul-betul masuk
 * viewport. Render mode `on-demand` supaya GPU tidak kerja terus-menerus saat
 * banner diam.
 *
 * Fallback WebGL-tidak-didukung ditampilkan eksplisit (bukan blank/crash).
 * Cleanup: texture di-dispose manual saat unmount; geometry/material bawaan
 * TresJS di-dispose otomatis oleh @tresjs/core saat node dilepas dari scene.
 */
import { CanvasTexture, DoubleSide, SRGBColorSpace, Vector3 } from 'three'
import { RotateCcw } from '@lucide/vue'

// Instance Vector3 langsung (bukan array literal) — tipe prop `position` bawaan
// TresJS/vue-tsc di versi ini strict ke `Vector3 | Readonly<Vector3 | undefined>`,
// array `[x,y,z]` tidak lolos type-check meski diterima runtime.
const CAMERA_POSITION = new Vector3(0, 0, 2.5)

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
  // tujuannya, yaitu menilai desain sebelum order (§16 poin 2).
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

// Bangun texture "contoh desain banner" di canvas 2D — dua-tone brand + ink
// sesuai §26.8 (bukan foto asli, bukan warna-warni).
function buildBannerTexture(): CanvasTexture {
  const canvas = document.createElement('canvas')
  canvas.width = 1024
  canvas.height = 576
  const ctx = canvas.getContext('2d')!

  const gradient = ctx.createLinearGradient(0, 0, canvas.width, canvas.height)
  gradient.addColorStop(0, '#8B1A1A')
  gradient.addColorStop(1, '#2B0505')
  ctx.fillStyle = gradient
  ctx.fillRect(0, 0, canvas.width, canvas.height)

  ctx.strokeStyle = 'rgba(250, 250, 249, 0.35)'
  ctx.lineWidth = 6
  ctx.strokeRect(24, 24, canvas.width - 48, canvas.height - 48)

  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillStyle = '#FAFAF9'
  ctx.font = '600 72px Georgia, "Times New Roman", serif'
  ctx.fillText('RAJAKU PRINTING', canvas.width / 2, canvas.height / 2 - 24)

  ctx.fillStyle = '#C9A44A'
  ctx.font = '400 28px Arial, sans-serif'
  ctx.fillText('CONTOH DESAIN BANNER', canvas.width / 2, canvas.height / 2 + 48)

  const tex = new CanvasTexture(canvas)
  tex.colorSpace = SRGBColorSpace
  tex.needsUpdate = true
  return tex
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
    texture.value = buildBannerTexture()
  }
})

onUnmounted(() => {
  texture.value?.dispose()
  texture.value = null
})
</script>

<template>
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
            <TresPlaneGeometry :args="[2, 1.125]" />
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
</template>
