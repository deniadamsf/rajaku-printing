<script setup lang="ts">
/**
 * HowItWorksSection — 4 langkah alur order (§6, §7, §5). Ikon Lucide monoline,
 * bukan angka besar warna-warni — tone tetap premium minimalist.
 *
 * Motion: reveal stagger disiplin (`once: true`), hover lift halus di kartu
 * (nested <div>, bukan <motion.li> itu sendiri — hindari konflik transform
 * dengan entrance motion-v, brief §4).
 */
import { CloudUpload, CreditCard, MapPin, Ruler } from '@lucide/vue'
import { motion } from 'motion-v'

const { container, item } = useRevealVariants()

const steps = [
  {
    icon: Ruler,
    title: 'Pilih produk & ukuran',
    desc: 'Tentukan jenis banner, bahan, dan ukuran — harga terhitung otomatis sesuai katalog.',
  },
  {
    icon: CloudUpload,
    title: 'Upload desain atau minta dibuatkan',
    desc: 'Sudah punya file siap cetak? Upload langsung. Belum punya? Tim desain kami bantu buatkan.',
  },
  {
    icon: CreditCard,
    title: 'Bayar & verifikasi',
    desc: 'Transfer/QRIS sesuai total, upload bukti bayar — staf kami verifikasi dan lanjut proses.',
  },
  {
    icon: MapPin,
    title: 'Lacak sampai selesai',
    desc: 'Pantau progres cetak lewat nomor resi sampai status siap diambil atau dikirim.',
  },
]
</script>

<template>
  <section id="cara-order" class="bg-canvas-alt">
    <div class="mx-auto max-w-6xl px-4 py-16 md:py-24">
      <div class="max-w-2xl">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Alur order</p>
        <h2 class="mt-3 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
          Cara order, 4 langkah sederhana
        </h2>
      </div>

      <motion.ol
        class="mt-10 grid gap-6 sm:grid-cols-2 lg:grid-cols-4"
        :variants="container"
        initial="hidden"
        while-in-view="show"
        :in-view-options="{ once: true, margin: '-100px' }"
      >
        <motion.li v-for="(s, i) in steps" :key="s.title" :variants="item">
          <div
            class="h-full rounded-lg border border-hairline bg-canvas p-6 transition-[border-color,box-shadow,transform] duration-200 ease-out hover:-translate-y-0.5 hover:border-ink-300 hover:shadow-sm md:p-8"
          >
            <div class="flex items-center justify-between">
              <span
                class="inline-flex h-10 w-10 items-center justify-center rounded-md bg-brand-50 text-brand-500"
              >
                <component :is="s.icon" class="h-5 w-5" :stroke-width="1.5" />
              </span>
              <span class="font-mono text-xs text-ink-400">{{ String(i + 1).padStart(2, '0') }}</span>
            </div>
            <h3 class="mt-5 text-sm font-sans font-semibold text-ink-950">{{ s.title }}</h3>
            <p class="mt-2 text-sm leading-relaxed text-ink-500">{{ s.desc }}</p>
          </div>
        </motion.li>
      </motion.ol>
    </div>
  </section>
</template>
