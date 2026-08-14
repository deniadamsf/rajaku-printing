<script setup lang="ts">
/**
 * WhyUsSection — poin keunggulan singkat. Ikon Lucide monoline, tanpa emoji.
 */
import { BadgeCheck, Gauge, ShieldCheck, Sparkles } from '@lucide/vue'

const points = [
  {
    icon: Gauge,
    title: 'Proses cepat',
    desc: 'Estimasi pengerjaan jelas sejak order masuk, tanpa nunggu tanpa kabar.',
  },
  {
    icon: ShieldCheck,
    title: 'Transparan tiap tahap',
    desc: 'Status order & bukti pembayaran terverifikasi, bisa dilacak kapan saja lewat resi.',
  },
  {
    icon: Sparkles,
    title: 'Hasil presisi warna',
    desc: 'Kalibrasi mesin large-format rutin — warna cetak konsisten sesuai desain.',
  },
  {
    icon: BadgeCheck,
    title: 'Fleksibel: cetak sendiri atau dibuatkan',
    desc: 'Upload desain siap cetak, atau serahkan ke tim desain kami untuk dikerjakan.',
  },
]
</script>

<template>
  <section id="kenapa-rajaku" class="mx-auto max-w-6xl px-4 py-16 md:py-24">
    <div class="max-w-2xl">
      <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Kenapa Rajaku</p>
      <h2 class="mt-3 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
        Dipercaya untuk hasil cetak yang konsisten
      </h2>
    </div>

    <ul class="mt-10 grid gap-6 sm:grid-cols-2">
      <li v-for="p in points" :key="p.title" class="flex gap-4">
        <span
          class="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-hairline text-brand-500"
        >
          <component :is="p.icon" class="h-5 w-5" :stroke-width="1.5" />
        </span>
        <div>
          <h3 class="text-sm font-sans font-semibold text-ink-950">{{ p.title }}</h3>
          <p class="mt-1 text-sm leading-relaxed text-ink-500">{{ p.desc }}</p>
        </div>
      </li>
    </ul>
  </section>
</template>
