<script setup lang="ts">
/**
 * AuthShell — kerangka dua kolom untuk halaman login & daftar.
 *
 * Sebelumnya kedua halaman itu berupa form sempit yang mengambang di tengah
 * halaman kosong — pola yang membuat situs terasa belum jadi. Panel kiri yang
 * gelap memberi bobot brand dan mengisi ruang mati, tanpa mengganggu form:
 * panel HANYA muncul di layar besar (`lg:`), sementara mobile tetap dapat form
 * terpusat seperti sebelumnya.
 *
 * Isi panel wajib berupa pernyataan yang memang benar tentang sistem ini
 * (satu form login untuk customer & staff §10, riwayat lintas channel §11,
 * lacak resi §5) — bukan klaim pemasaran yang tidak bisa dibuktikan.
 */
import type { Component } from 'vue'

defineProps<{
  /** Judul besar di panel brand (kiri, desktop saja). */
  panelTitle: string
  /** Poin pendukung di panel brand. `icon` diisi komponen ikon Lucide. */
  panelPoints: ReadonlyArray<{ icon: Component; text: string }>
}>()
</script>

<template>
  <section class="grid lg:min-h-[calc(100vh-3.5rem)] lg:grid-cols-2">
    <!-- Panel brand — dekoratif + orientasi, disembunyikan di bawah lg. -->
    <div
      class="relative hidden overflow-hidden bg-ink-950 px-12 py-14 lg:flex lg:flex-col lg:justify-between"
    >
      <div class="absolute inset-0" aria-hidden="true">
        <ArtOrnament variant="arc" :opacity="0.55" />
      </div>

      <p class="relative font-serif text-lg tracking-tight text-canvas">
        Rajaku <span class="font-normal text-gold-500">Printing</span>
      </p>

      <div class="relative max-w-sm">
        <h2
          class="text-3xl font-serif font-semibold leading-tight tracking-tight text-canvas xl:text-4xl"
        >
          {{ panelTitle }}
        </h2>
        <ul class="mt-8 space-y-5">
          <li v-for="point in panelPoints" :key="point.text" class="flex gap-3">
            <component
              :is="point.icon"
              class="mt-0.5 h-5 w-5 shrink-0 text-gold-400"
              :stroke-width="1.5"
            />
            <span class="text-sm leading-relaxed text-canvas/75">{{ point.text }}</span>
          </li>
        </ul>
      </div>

      <p class="relative text-[10px] font-medium uppercase tracking-[0.14em] text-canvas/45">
        Trenggalek &middot; Jawa Timur
      </p>
    </div>

    <!-- Kolom form -->
    <div class="flex items-center justify-center px-4 py-16 md:py-20">
      <div class="w-full max-w-md">
        <slot />
      </div>
    </div>
  </section>
</template>
