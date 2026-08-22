<script setup lang="ts">
/**
 * MarqueeBand — pita berjalan berisi jenis cetakan yang dilayani, dipasang
 * tepat di bawah hero sebagai jembatan dari hero gelap ke konten terang.
 *
 * Kenapa ada: peralihan hero → section pertama sebelumnya adalah potongan
 * datar (gelap langsung putih kosong), dan halaman terasa diam sampai user
 * menggulir jauh. Pita ini mengisi peralihan itu dengan gerak yang terus
 * hidup tanpa meminta perhatian.
 *
 * Isinya DATA KATALOG ASLI (kunci `useAsyncData` sama dengan ProductSlider,
 * jadi tidak ada request tambahan) — bukan daftar kata hiasan. Kalau katalog
 * kosong atau gagal dimuat, komponen tidak merender apa pun.
 *
 * Gerak pakai CSS keyframes, bukan motion-v: ini animasi tanpa henti sepanjang
 * halaman terbuka, dan keyframes CSS berjalan di compositor tanpa membangunkan
 * JavaScript tiap frame. `prefers-reduced-motion` menghentikannya total (pita
 * tetap terbaca, hanya diam), dan gerak berhenti saat kursor menyentuhnya.
 */
import type { CatalogProduct } from '~/types/catalog'

const catalog = useCatalog()

// Kunci sama dengan LandingProductSlider — Nuxt berbagi hasilnya, tidak
// menembak endpoint katalog dua kali.
const { data } = await useAsyncData('landing-product-slider', () => catalog.listProducts())

const items = computed<string[]>(() => {
  const names = (data.value?.products ?? []).map((p: CatalogProduct) => p.name)
  return names.length > 0 ? names : []
})
</script>

<template>
  <!--
    `aria-hidden` + tanpa tabindex: pita ini murni dekorasi berulang. Daftar
    produk yang sesungguhnya (bisa dibaca pembaca layar & mesin pencari) ada
    lengkap di LandingServicesSection.
  -->
  <div
    v-if="items.length > 0"
    class="relative overflow-hidden border-y border-white/10 bg-ink-950 py-3"
    aria-hidden="true"
  >
    <div class="marquee flex w-max items-center gap-8 whitespace-nowrap px-4">
      <!--
        Daftar digandakan dua kali dan animasinya berhenti tepat di -50%,
        sehingga salinan kedua mendarat persis di posisi salinan pertama —
        itulah yang membuat putarannya tak terlihat sambungannya.
      -->
      <template v-for="pass in 2" :key="pass">
        <span
          v-for="name in items"
          :key="`${pass}-${name}`"
          class="flex items-center gap-8 text-sm font-medium tracking-tight text-canvas/60"
        >
          {{ name }}
          <span class="h-1 w-1 shrink-0 rounded-full bg-gold-500" />
        </span>
      </template>
    </div>
  </div>
</template>

<style scoped>
.marquee {
  animation: marquee-slide 38s linear infinite;
}

.marquee:hover {
  animation-play-state: paused;
}

@keyframes marquee-slide {
  from {
    transform: translateX(0);
  }
  to {
    transform: translateX(-50%);
  }
}

@media (prefers-reduced-motion: reduce) {
  .marquee {
    animation: none;
  }
}
</style>
