<script setup lang="ts">
/**
 * LocationCard — kartu lokasi toko: peta, alamat lengkap, patokan, dan tombol
 * petunjuk arah. Seluruh datanya dari `~/utils/business.ts` (sumber tunggal
 * NAP) — jangan menulis ulang alamat/koordinat literal di komponen ini.
 *
 * Petanya SENGAJA tidak bisa digeser (`pointer-events-none`) dan seluruh
 * bidangnya jadi satu tautan ke Google Maps. Dua alasan, keduanya nyata:
 *  1. Peta interaktif di tengah halaman menjebak scroll di HP — jari yang
 *     hendak menggulir halaman malah menggeser peta.
 *  2. Yang sebetulnya dibutuhkan pengunjung bukan menggeser peta di sini,
 *     tapi pindah ke aplikasi Maps untuk berangkat.
 *
 * Peta adalah PELENGKAP, bukan sumber informasi: alamat, patokan, dan tombol
 * arah tetap teks/tautan asli di bawahnya, jadi kalau embed Google gagal muat
 * (diblokir jaringan kantor, offline, `mapsEmbedUrl` null karena koordinat
 * belum diisi) kartunya masih menjawab "di mana tokonya, bagaimana ke sana".
 */
import { ExternalLink, MapPin, Navigation } from '@lucide/vue'
import { business, mapsDirectionsUrl, mapsEmbedUrl } from '~/utils/business'

const focusRing =
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas'
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-hairline bg-canvas">
    <!--
      Peta didesaturasi (§26.8: foto/visual dikalemkan supaya tidak berteriak
      lebih keras dari teks di sebelahnya) dan kembali berwarna penuh saat
      kursor masuk — isyarat halus bahwa bidang ini bisa diklik.
    -->
    <a
      :href="business.mapsUrl"
      target="_blank"
      rel="noopener noreferrer"
      :class="['group relative block aspect-[4/3] w-full bg-canvas-alt sm:aspect-[16/10]', focusRing]"
    >
      <iframe
        v-if="mapsEmbedUrl"
        :src="mapsEmbedUrl"
        :title="`Peta lokasi ${business.name}`"
        loading="lazy"
        referrerpolicy="no-referrer-when-downgrade"
        class="pointer-events-none h-full w-full border-0 opacity-95 grayscale-[0.55] transition duration-300 ease-out group-hover:opacity-100 group-hover:grayscale-0"
      />
      <!-- Keadaan tanpa peta: tetap terbaca, bukan kotak abu kosong. -->
      <div v-else class="flex h-full w-full items-center justify-center">
        <MapPin class="h-8 w-8 text-ink-400" :stroke-width="1.5" />
      </div>

      <!-- Kerai gelap tipis di kaki peta supaya label putih di bawah tetap terbaca. -->
      <div
        class="pointer-events-none absolute inset-x-0 bottom-0 h-24 bg-gradient-to-t from-ink-950/70 to-transparent"
        aria-hidden="true"
      />
      <span
        class="pointer-events-none absolute bottom-4 left-4 inline-flex items-center gap-2 rounded-md bg-ink-950/85 px-3 py-1.5 text-xs font-semibold text-canvas backdrop-blur-sm transition-colors duration-200 ease-out group-hover:bg-ink-950"
      >
        <ExternalLink class="h-3.5 w-3.5" :stroke-width="1.5" />
        Buka di Google Maps
      </span>
    </a>

    <div class="p-6 md:p-8">
      <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Lokasi</p>

      <p class="mt-3 font-serif text-xl font-medium leading-snug text-ink-950">
        {{ business.streetAddress }}
      </p>
      <p class="mt-1 text-sm leading-relaxed text-ink-500">
        {{ business.addressLocality }}, {{ business.addressRegion }} {{ business.postalCode }}
      </p>

      <!--
        Patokan & nama lama dipisah dari blok alamat: keduanya bukan bagian
        alamat pos (jangan ikut masuk JSON-LD `PostalAddress`), tapi justru ini
        yang dipakai orang untuk benar-benar menemukan tokonya di lapangan.
      -->
      <ul class="mt-5 space-y-2.5 border-t border-hairline pt-5 text-sm">
        <li class="flex gap-3">
          <Navigation class="mt-0.5 h-4 w-4 shrink-0 text-brand-500" :stroke-width="1.5" />
          <span class="leading-relaxed text-ink-700">{{ business.landmark }}</span>
        </li>
        <li v-if="business.formerName" class="flex gap-3">
          <MapPin class="mt-0.5 h-4 w-4 shrink-0 text-gold-600" :stroke-width="1.5" />
          <span class="leading-relaxed text-ink-500">
            Menempati bekas <span class="text-ink-700">{{ business.formerName }}</span> — kalau
            bertanya ke warga sekitar, nama itu masih lebih dikenal.
          </span>
        </li>
      </ul>

      <div class="mt-6 flex flex-wrap gap-3">
        <a
          :href="mapsDirectionsUrl"
          target="_blank"
          rel="noopener noreferrer"
          :class="[
            'inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas transition-colors duration-200 ease-out hover:bg-brand-600',
            focusRing,
          ]"
        >
          <Navigation class="h-4 w-4" :stroke-width="1.5" />
          Petunjuk Arah
        </a>
        <a
          :href="business.mapsUrl"
          target="_blank"
          rel="noopener noreferrer"
          :class="[
            'inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-4 py-2.5 text-sm font-semibold text-ink-900 transition-colors duration-200 ease-out hover:bg-canvas-alt',
            focusRing,
          ]"
        >
          <ExternalLink class="h-4 w-4" :stroke-width="1.5" />
          Lihat di Maps
        </a>
      </div>
    </div>
  </div>
</template>
