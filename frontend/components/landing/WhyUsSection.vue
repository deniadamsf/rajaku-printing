<script setup lang="ts">
/**
 * WhyUsSection — split layout: foto workshop di kiri (deliverable §F brief,
 * "kurang gambar") + 4 poin keunggulan yang sudah ada di kanan. Ikon Lucide
 * monoline, tanpa emoji. Reveal stagger disiplin (`once: true`) — poin ini
 * tenang, bukan momen utama, tidak ada animasi baru ditambahkan.
 */
import { BadgeCheck, Gauge, ShieldCheck, Sparkles } from '@lucide/vue'
import { motion } from 'motion-v'

const { container, item } = useRevealVariants()
const prefersReduced = usePrefersReducedMotion()

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
    <div class="grid gap-10 md:grid-cols-2 md:items-center md:gap-16">
      <motion.div
        class="relative aspect-[4/3] overflow-hidden rounded-lg border border-hairline bg-canvas-alt md:order-2"
        :initial="{ opacity: 0, y: prefersReduced ? 0 : 16 }"
        :while-in-view="{ opacity: 1, y: 0 }"
        :in-view-options="{ once: true, margin: '-100px' }"
        :transition="{ duration: prefersReduced ? 0 : 0.5, ease: [0.22, 1, 0.36, 1] }"
      >
        <img
          src="/proses/proses-08.webp"
          alt="Carriage print-head mesin cetak large-format Rajaku Printing bergerak di atas banner"
          width="1000"
          height="750"
          loading="lazy"
          class="h-full w-full object-cover"
        >
      </motion.div>

      <div class="md:order-1">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Kenapa Rajaku</p>
        <h2 class="mt-3 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
          Dipercaya untuk hasil cetak yang konsisten
        </h2>

        <motion.ul
          class="mt-8 space-y-6"
          :variants="container"
          initial="hidden"
          while-in-view="show"
          :in-view-options="{ once: true, margin: '-100px' }"
        >
          <motion.li v-for="p in points" :key="p.title" :variants="item" class="flex gap-4">
            <span
              class="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-hairline text-brand-500"
            >
              <component :is="p.icon" class="h-5 w-5" :stroke-width="1.5" />
            </span>
            <div>
              <h3 class="text-sm font-sans font-semibold text-ink-950">{{ p.title }}</h3>
              <p class="mt-1 text-sm leading-relaxed text-ink-500">{{ p.desc }}</p>
            </div>
          </motion.li>
        </motion.ul>
      </div>
    </div>
  </section>
</template>
