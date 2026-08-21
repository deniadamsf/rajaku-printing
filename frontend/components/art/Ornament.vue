<script setup lang="ts">
/**
 * ArtOrnament — lapisan ornamen SVG tipis untuk latar section, supaya bidang
 * kosong terasa disusun (bukan sekadar kosong) tanpa menambah gambar raster.
 *
 * Selalu dekoratif: `aria-hidden`, `pointer-events-none`, dan tidak pernah
 * membawa informasi. Pemanggil menentukan posisi/ukuran lewat class wrapper
 * (biasanya `absolute inset-0`), komponen ini hanya menggambar polanya.
 *
 * Varian:
 *  - `grid`     — kisi presisi halus, untuk section terang.
 *  - `halftone` — raster titik yang memudar, referensi proses cetak.
 *  - `arc`      — busur konsentris emas tipis, untuk section gelap (hero/CTA).
 *
 * Opacity default sengaja rendah (§26: ornamen tidak boleh berebut perhatian
 * dengan teks). Naikkan lewat prop hanya kalau memang latar polos gelap.
 */
const props = withDefaults(
  defineProps<{
    variant?: 'grid' | 'halftone' | 'arc'
    /** 0–1, dipetakan ke atribut opacity SVG. */
    opacity?: number
  }>(),
  { variant: 'grid', opacity: 0.5 },
)

// id pattern wajib unik per instance — dua ornamen di satu halaman dengan id
// sama membuat browser memakai definisi pertama untuk keduanya.
const uid = useId()
const patternId = `ornament-${uid}`
const fadeId = `ornament-fade-${uid}`

const isGrid = computed(() => props.variant === 'grid')
</script>

<template>
  <svg
    class="pointer-events-none h-full w-full"
    :opacity="opacity"
    preserveAspectRatio="xMidYMid slice"
    viewBox="0 0 800 400"
    xmlns="http://www.w3.org/2000/svg"
    aria-hidden="true"
    focusable="false"
  >
    <defs>
      <pattern
        v-if="variant !== 'arc'"
        :id="patternId"
        :width="isGrid ? 32 : 18"
        :height="isGrid ? 32 : 18"
        patternUnits="userSpaceOnUse"
      >
        <path v-if="isGrid" d="M32 0H0v32" fill="none" class="stroke-ink-200" stroke-width="1" />
        <circle v-else cx="9" cy="9" r="1.6" class="fill-ink-300" />
      </pattern>
      <linearGradient :id="fadeId" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stop-color="white" stop-opacity="0.9" />
        <stop offset="100%" stop-color="white" stop-opacity="0" />
      </linearGradient>
      <mask :id="`${fadeId}-mask`">
        <rect width="800" height="400" :fill="`url(#${fadeId})`" />
      </mask>
    </defs>

    <rect
      v-if="variant !== 'arc'"
      width="800"
      height="400"
      :fill="`url(#${patternId})`"
      :mask="`url(#${fadeId}-mask)`"
    />

    <g v-else fill="none" class="stroke-gold-500" stroke-width="1">
      <circle cx="680" cy="60" r="120" opacity="0.35" />
      <circle cx="680" cy="60" r="180" opacity="0.25" />
      <circle cx="680" cy="60" r="250" opacity="0.16" />
      <circle cx="680" cy="60" r="330" opacity="0.1" />
    </g>
  </svg>
</template>
