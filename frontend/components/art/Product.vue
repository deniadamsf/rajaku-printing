<script setup lang="ts">
/**
 * ArtProduct — artwork vektor per jenis produk cetak, dipakai sebagai visual
 * kartu produk / tile galeri selama foto asli belum diunggah admin.
 *
 * Kenapa vektor, bukan foto stok: seluruh stok foto proyek ini berasal dari satu
 * klip mesin cetak yang sama, jadi memakainya berulang di 8 kartu produk justru
 * memperkuat kesan "kosong & template". Artwork ini memberi tiap produk siluet
 * yang berbeda, tetap satu bahasa visual, dan bebas dari klaim palsu (ini jelas
 * ilustrasi, bukan foto hasil kerja — lihat catatan di `utils/artwork.ts`).
 *
 * Kepatuhan §26:
 *  - Warna HANYA dari token (`fill-brand-*`, `fill-gold-*`, `fill-ink-*`,
 *    `fill-canvas*`) — tidak ada hex hardcode, termasuk di gradient (stop pakai
 *    `stop-color="currentColor"` yang diwarnai class token).
 *  - Flat 2-tone + satu aksen emas, garis monoline — bukan kartun warna-warni.
 *  - Tanpa teks di dalam SVG (teks kecil di gambar tidak terbaca screen reader
 *    dan pecah saat diskalakan) — informasi selalu di HTML sekitarnya.
 *
 * Ukuran mengikuti container (`h-full w-full`), rasio 4:3 dari viewBox. Pemanggil
 * yang butuh rasio lain cukup membungkus dengan `aspect-*` + `object-cover`.
 */
import { ARTWORK_LABELS, type ArtworkVariant } from '~/utils/artwork'

const props = withDefaults(
  defineProps<{
    variant?: ArtworkVariant
    /** Override aria-label. Kosongkan untuk pakai label bawaan per varian. */
    label?: string
    /** Sembunyikan dari screen reader — untuk artwork yang murni dekorasi. */
    decorative?: boolean
  }>(),
  { variant: 'umum', label: '', decorative: false },
)

// id <defs> wajib unik per instance: satu halaman bisa merender 8 artwork
// sekaligus, dan id duplikat bikin browser memakai pattern instance pertama
// untuk semuanya (bug halus yang baru kelihatan saat grid penuh).
const uid = useId()
const gridId = `art-grid-${uid}`
const glowId = `art-glow-${uid}`

const ariaLabel = computed(() => props.label || ARTWORK_LABELS[props.variant])
</script>

<template>
  <svg
    viewBox="0 0 400 300"
    xmlns="http://www.w3.org/2000/svg"
    class="h-full w-full"
    :role="decorative ? undefined : 'img'"
    :aria-label="decorative ? undefined : ariaLabel"
    :aria-hidden="decorative ? 'true' : undefined"
    focusable="false"
  >
    <defs>
      <pattern :id="gridId" width="16" height="16" patternUnits="userSpaceOnUse">
        <path d="M16 0H0v16" fill="none" class="stroke-ink-200" stroke-width="1" />
      </pattern>
      <radialGradient :id="glowId" cx="50%" cy="45%" r="65%">
        <stop offset="0%" class="text-gold-200" stop-color="currentColor" />
        <stop offset="100%" class="text-canvas" stop-color="currentColor" />
      </radialGradient>
    </defs>

    <!-- Panggung bersama semua varian: bidang hangat + grid presisi halus. -->
    <rect width="400" height="300" class="fill-canvas-alt" />
    <rect width="400" height="300" :fill="`url(#${gridId})`" opacity="0.5" />

    <!-- Tanda register cetak di empat sudut — signature percetakan, aksen emas. -->
    <g class="stroke-gold-500" stroke-width="1.25" opacity="0.7" fill="none">
      <path d="M22 30v-8h8M370 22h8v8M378 270v8h-8M30 278h-8v-8" />
    </g>

    <ellipse cx="200" cy="262" rx="112" ry="7" class="fill-ink-300" opacity="0.35" />

    <!-- ================= Spanduk / banner horizontal ================= -->
    <g v-if="variant === 'spanduk'">
      <path d="M52 62h296" class="stroke-ink-950" stroke-width="4" stroke-linecap="round" />
      <path d="M86 62v18M314 62v18" class="stroke-ink-500" stroke-width="2" />
      <path d="M78 84h256v126c-40 10-216 10-256 0Z" class="fill-ink-950" opacity="0.08" />
      <path d="M72 78h256v126c-40 10-216 10-256 0Z" class="fill-brand-500" />
      <path d="M72 78h256v7H72Z" class="fill-brand-600" />
      <rect x="96" y="104" width="140" height="18" rx="2" class="fill-canvas" opacity="0.95" />
      <rect x="96" y="130" width="96" height="10" rx="2" class="fill-canvas" opacity="0.55" />
      <rect x="96" y="152" width="52" height="5" rx="2" class="fill-gold-400" />
      <rect x="252" y="104" width="56" height="56" rx="2" class="fill-ink-950" opacity="0.22" />
      <g class="fill-canvas stroke-ink-950" stroke-width="1.5">
        <circle cx="86" cy="92" r="4.5" />
        <circle cx="314" cy="92" r="4.5" />
        <circle cx="86" cy="192" r="4.5" />
        <circle cx="314" cy="192" r="4.5" />
      </g>
    </g>

    <!-- ================= X-Banner ================= -->
    <g v-else-if="variant === 'x-banner'">
      <path
        d="M140 76l120 172M260 76L140 248"
        class="stroke-ink-400"
        stroke-width="3.5"
        stroke-linecap="round"
      />
      <path d="M200 48v202" class="stroke-ink-300" stroke-width="2" />
      <path d="M148 250h104" class="stroke-ink-950" stroke-width="4.5" stroke-linecap="round" />
      <rect x="158" y="50" width="96" height="192" class="fill-ink-950" opacity="0.08" />
      <rect x="152" y="44" width="96" height="192" class="fill-ink-950" />
      <rect x="166" y="62" width="68" height="11" rx="1.5" class="fill-canvas" opacity="0.9" />
      <rect x="166" y="80" width="46" height="6" rx="1.5" class="fill-canvas" opacity="0.45" />
      <rect x="166" y="100" width="68" height="76" class="fill-brand-500" />
      <rect x="166" y="190" width="36" height="4" rx="2" class="fill-gold-400" />
      <rect x="166" y="204" width="54" height="4" rx="2" class="fill-canvas" opacity="0.25" />
      <g class="fill-gold-400">
        <circle cx="156" cy="48" r="2.5" />
        <circle cx="244" cy="48" r="2.5" />
      </g>
    </g>

    <!-- ================= Roll-up banner ================= -->
    <g v-else-if="variant === 'roll-up'">
      <path d="M262 46v190" class="stroke-ink-300" stroke-width="2.5" />
      <path d="M246 46h16" class="stroke-ink-300" stroke-width="2.5" />
      <rect x="146" y="50" width="116" height="188" class="fill-ink-950" opacity="0.06" />
      <rect
        x="138"
        y="42"
        width="116"
        height="194"
        class="fill-canvas stroke-ink-200"
        stroke-width="1"
      />
      <rect x="138" y="42" width="116" height="9" class="fill-ink-950" />
      <rect x="154" y="68" width="72" height="13" rx="2" class="fill-ink-950" opacity="0.85" />
      <rect x="154" y="90" width="52" height="7" rx="2" class="fill-ink-400" />
      <rect x="154" y="112" width="84" height="70" class="fill-brand-500" />
      <rect x="154" y="196" width="36" height="4" rx="2" class="fill-gold-400" />
      <rect x="154" y="210" width="62" height="5" rx="2" class="fill-ink-200" />
      <rect x="132" y="236" width="128" height="18" rx="5" class="fill-ink-950" />
      <rect x="144" y="254" width="16" height="6" rx="2" class="fill-ink-800" />
      <rect x="232" y="254" width="16" height="6" rx="2" class="fill-ink-800" />
    </g>

    <!-- ================= Baliho ================= -->
    <g v-else-if="variant === 'baliho'">
      <rect x="118" y="150" width="9" height="106" class="fill-ink-950" />
      <rect x="273" y="150" width="9" height="106" class="fill-ink-950" />
      <path d="M127 202l146-24M127 178l146 24" class="stroke-ink-400" stroke-width="2.5" />
      <rect x="96" y="50" width="220" height="112" class="fill-ink-950" opacity="0.08" />
      <rect x="90" y="44" width="220" height="112" class="fill-ink-950" />
      <rect x="98" y="52" width="204" height="96" class="fill-brand-500" />
      <rect x="112" y="68" width="112" height="16" rx="2" class="fill-canvas" opacity="0.95" />
      <rect x="112" y="92" width="76" height="9" rx="2" class="fill-canvas" opacity="0.5" />
      <rect x="112" y="112" width="44" height="5" rx="2" class="fill-gold-400" />
      <rect x="240" y="68" width="48" height="60" rx="2" class="fill-ink-950" opacity="0.25" />
      <!-- Lampu sorot + kerucut cahaya tipis ke papan (aksen emas, bukan glow norak). -->
      <g class="fill-gold-400" opacity="0.16">
        <path d="M146 52h22l14 40h-50ZM232 52h22l14 40h-50Z" />
      </g>
      <g class="fill-ink-800">
        <rect x="146" y="30" width="22" height="9" rx="2.5" />
        <rect x="232" y="30" width="22" height="9" rx="2.5" />
      </g>
      <path d="M157 39v5M243 39v5" class="stroke-ink-800" stroke-width="2.5" />
      <path d="M60 256h280" class="stroke-ink-200" stroke-width="2" />
    </g>

    <!-- ================= Backdrop panggung ================= -->
    <g v-else-if="variant === 'backdrop'">
      <path
        d="M84 224l-22 34M316 224l22 34"
        class="stroke-ink-950"
        stroke-width="5"
        stroke-linecap="round"
      />
      <rect x="90" y="54" width="228" height="164" class="fill-ink-950" opacity="0.08" />
      <rect
        x="78"
        y="46"
        width="244"
        height="178"
        rx="2"
        fill="none"
        class="stroke-ink-950"
        stroke-width="5"
      />
      <rect x="88" y="56" width="224" height="158" class="fill-brand-500" />
      <circle cx="252" cy="108" r="34" class="fill-gold-400" opacity="0.9" />
      <rect x="108" y="86" width="110" height="16" rx="2" class="fill-canvas" opacity="0.95" />
      <rect x="108" y="112" width="74" height="9" rx="2" class="fill-canvas" opacity="0.5" />
      <rect x="108" y="150" width="146" height="6" rx="3" class="fill-canvas" opacity="0.22" />
      <rect x="108" y="166" width="102" height="6" rx="3" class="fill-canvas" opacity="0.22" />
    </g>

    <!-- ================= Stiker vinyl ================= -->
    <g v-else-if="variant === 'stiker'">
      <rect x="112" y="58" width="188" height="188" rx="4" class="fill-ink-950" opacity="0.07" />
      <rect
        x="106"
        y="52"
        width="188"
        height="188"
        rx="4"
        class="fill-canvas stroke-ink-200"
        stroke-width="1.5"
      />
      <circle cx="152" cy="98" r="22" class="fill-brand-500" />
      <circle cx="200" cy="98" r="22" class="fill-ink-950" />
      <circle cx="248" cy="98" r="22" class="fill-gold-400" />
      <g fill="none" class="stroke-ink-300" stroke-width="1.5" stroke-dasharray="4 4">
        <circle cx="152" cy="150" r="22" />
        <circle cx="200" cy="150" r="22" />
        <circle cx="248" cy="150" r="22" />
        <circle cx="152" cy="202" r="22" />
      </g>
      <circle cx="200" cy="202" r="22" class="fill-brand-500" opacity="0.35" />
      <path
        d="M294 196v44h-44c18-12 32-26 44-44Z"
        class="fill-ink-100 stroke-ink-300"
        stroke-width="1.5"
      />
    </g>

    <!-- ================= Backlite / lightbox ================= -->
    <g v-else-if="variant === 'backlite'">
      <path
        d="M132 200l-12 54M268 200l12 54"
        class="stroke-ink-950"
        stroke-width="5"
        stroke-linecap="round"
      />
      <rect x="94" y="56" width="224" height="150" rx="3" class="fill-ink-950" opacity="0.08" />
      <rect x="88" y="50" width="224" height="150" rx="3" class="fill-ink-950" />
      <rect x="98" y="60" width="204" height="130" :fill="`url(#${glowId})`" />
      <rect x="118" y="82" width="96" height="14" rx="2" class="fill-ink-950" opacity="0.75" />
      <rect x="118" y="106" width="64" height="8" rx="2" class="fill-ink-950" opacity="0.4" />
      <rect x="118" y="132" width="40" height="5" rx="2" class="fill-brand-500" />
      <circle cx="262" cy="112" r="26" class="fill-brand-500" opacity="0.85" />
      <g class="stroke-gold-400" stroke-width="1.5" opacity="0.7">
        <path d="M74 96h-12M74 128h-12M326 96h12M326 128h12" stroke-linecap="round" />
      </g>
    </g>

    <!-- ================= Umum: roll bahan + hasil cetak ================= -->
    <g v-else>
      <rect x="96" y="198" width="212" height="44" rx="3" class="fill-ink-950" opacity="0.06" />
      <rect
        x="90"
        y="192"
        width="212"
        height="44"
        rx="3"
        class="fill-canvas stroke-ink-200"
        stroke-width="1.5"
      />
      <rect x="106" y="204" width="92" height="8" rx="2" class="fill-ink-300" />
      <rect x="106" y="218" width="60" height="6" rx="2" class="fill-ink-200" />
      <rect x="252" y="204" width="34" height="4" rx="2" class="fill-gold-400" />
      <rect x="120" y="112" width="160" height="70" class="fill-brand-500" />
      <ellipse cx="280" cy="147" rx="17" ry="35" class="fill-brand-400" />
      <ellipse cx="280" cy="147" rx="7" ry="15" class="fill-canvas" />
      <ellipse cx="120" cy="147" rx="17" ry="35" class="fill-brand-600" />
      <path d="M132 112h136" class="stroke-canvas" stroke-width="3" opacity="0.35" />
      <path d="M132 172h136" class="stroke-ink-950" stroke-width="3" opacity="0.15" />
    </g>
  </svg>
</template>
