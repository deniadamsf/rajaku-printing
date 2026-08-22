<script setup lang="ts">
/**
 * ClosingCta — ajakan order penutup.
 *
 * Daftar NAP (alamat/telepon/jam) DIPINDAH ke footer (22 Agustus 2026): footer
 * kini gelap dan menempel persis di bawah section ini, sehingga alamat & jam
 * tampil dua kali berturut-turut di layar yang sama. Footer yang menang karena
 * ia hadir di SEMUA halaman, bukan cuma landing. Yang tersisa di sini murni
 * ajakan bertindak — satu kolom terpusat, tanpa kolom kedua yang mengalihkan
 * perhatian dari tombolnya. Nomor WA tetap di sini sebagai jalur cepat tanya.
 *
 * Brand moment (deliverable §4 brief): `logo-mark.webp` (kepala raja bermahkota)
 * ditampilkan halus di atas headline penutup — satu-satunya tempat mascot muncul
 * di landing selain hero, sesuai batas §26.8 (bukan di navbar/checkout).
 * Reveal fade+y sekali saat masuk viewport, bukan bagian dari momen orkestrasi
 * utama (hero) — tenang & singkat.
 */
import { ArrowRight, MessageCircle, Search } from '@lucide/vue'
import { motion } from 'motion-v'
import { business } from '~/utils/business'

const prefersReduced = usePrefersReducedMotion()

// Nomor WA dari sumber tunggal `business.whatsapp` (§13: sudah format 62xxx,
// syarat wajib supaya tautan wa.me valid). Pesan awal di-encode di sini,
// jangan ditulis manual dengan %20 — gampang salah dan sulit diaudit.
const waHref = computed(() => {
  const text = encodeURIComponent(
    'Halo Rajaku Printing, saya mau tanya soal cetak banner.',
  )
  return `https://wa.me/${business.whatsapp}?text=${text}`
})

// Bisa diganti admin (/admin/site-media, slot brand_logo_mark) tanpa deploy
// ulang. `await ready` — komponen ini SSR normal (tidak di dalam <ClientOnly>).
const { resolve: resolveMedia, ready: siteMediaReady } = useSiteMedia()
await siteMediaReady
const logoMark = computed(() => resolveMedia('brand_logo_mark'))
</script>

<template>
  <!--
    Penutup halaman sengaja full-bleed gelap, bukan kartu gelap di atas canvas
    terang: section terakhir yang melebar penuh memberi "titik akhir" yang tegas
    dan menutup ritme terang-gelap halaman. Ornamen busur emas dipasang sangat
    tipis di belakang — dekorasi, tidak boleh sampai bersaing dengan teks.
  -->
  <section class="relative overflow-hidden bg-ink-950">
    <div class="absolute inset-0" aria-hidden="true">
      <ArtOrnament variant="arc" :opacity="0.5" />
    </div>

    <motion.div
      class="relative mx-auto max-w-6xl px-4 py-20 md:py-28"
      :initial="{ opacity: 0, y: prefersReduced ? 0 : 16 }"
      :while-in-view="{ opacity: 1, y: 0 }"
      :in-view-options="{ once: true, margin: '-40px' }"
      :transition="{ duration: prefersReduced ? 0 : 0.5, ease: [0.22, 1, 0.36, 1] }"
    >
      <div class="mx-auto max-w-2xl text-center">
        <div>
          <img
            :src="logoMark"
            alt=""
            aria-hidden="true"
            width="512"
            height="512"
            loading="lazy"
            class="mx-auto h-12 w-12 opacity-90"
          >
          <h2 class="mt-5 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-canvas">
            Siap cetak banner Anda?
          </h2>
          <p class="mx-auto mt-3 max-w-md text-sm leading-relaxed text-canvas/70">
            Mulai order sekarang, atau lacak progres pesanan yang sudah berjalan
            pakai nomor resi.
          </p>

          <div class="mt-8 flex flex-col items-center justify-center gap-3 sm:flex-row">
            <NuxtLink
              to="/order"
              class="inline-flex w-full sm:w-auto items-center justify-center gap-2 rounded-md bg-brand-500 px-6 py-3 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
            >
              Order Banner
              <ArrowRight class="h-4 w-4" :stroke-width="1.5" />
            </NuxtLink>
            <NuxtLink
              to="/lacak"
              class="inline-flex w-full sm:w-auto items-center justify-center gap-2 rounded-md border border-canvas/25 px-6 py-3 text-sm font-semibold text-canvas transition-colors hover:bg-canvas/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
            >
              <Search class="h-4 w-4" :stroke-width="1.5" />
              Lacak Resi
            </NuxtLink>
          </div>

          <p class="mt-6 text-sm text-canvas/60">
            Mau tanya-tanya dulu?
            <a
              :href="waHref"
              target="_blank"
              rel="noopener noreferrer"
              class="ml-1 inline-flex items-center gap-1.5 rounded-sm font-medium text-gold-400 transition-colors hover:text-gold-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold-400/50 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
            >
              <MessageCircle class="h-4 w-4" :stroke-width="1.5" />
              Chat WhatsApp
            </a>
          </p>
        </div>

      </div>
    </motion.div>
  </section>
</template>
