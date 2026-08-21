<script setup lang="ts">
/**
 * FaqSection — 6 pertanyaan yang benar-benar dijawab oleh alur sistem ini
 * (CLAUDE.md §4-§8, §12, §13). Tidak ada jawaban yang mengarang fitur/statistik
 * yang tidak ada.
 *
 * Accordion aksesibel: `<button aria-expanded>` + `aria-controls`, panel punya
 * `role="region"` + `aria-labelledby`. Animasi buka-tutup pakai teknik CSS
 * "grid-rows trick" (`grid-template-rows: 0fr → 1fr` + `overflow-hidden`) —
 * tidak perlu JS mengukur tinggi konten (`scrollHeight`), jadi tidak ada
 * layout thrashing/jank saat toggle. Menghormati `usePrefersReducedMotion()`
 * dengan menonaktifkan durasi transisi (bukan skip animasi diam-diam).
 *
 * JSON-LD `FAQPage` dipasang lewat `useHead` — isinya persis sama dengan yang
 * dirender di layar (satu sumber data `faqs`, bukan disalin ulang).
 */
import { ChevronDown } from '@lucide/vue'
import { motion } from 'motion-v'

const prefersReduced = usePrefersReducedMotion()

interface FaqItem {
  question: string
  answer: string
}

const faqs: FaqItem[] = [
  {
    question: 'Bagaimana cara order banner di Rajaku Printing?',
    answer:
      'Pilih produk, ukuran, dan bahan di halaman Order — harga terhitung otomatis sesuai katalog. Anda bisa checkout tanpa akun (guest) atau login dulu. Setelah order masuk, tim kami mengonfirmasi ongkir (jika dikirim) sebelum total ditagihkan untuk dibayar.',
  },
  {
    question: 'Apa bedanya upload desain sendiri dengan minta dibuatkan?',
    answer:
      'Upload desain sendiri: kirim file yang sudah siap cetak, kami verifikasi lalu lanjut ke proses cetak. Request desain: kirim aset (logo/foto) dan brief, tim desain kami yang mengerjakan — ada biaya jasa desain terpisah — dan Anda perlu menyetujui hasilnya dulu sebelum dicetak.',
  },
  {
    question: 'Format file apa saja yang diterima untuk desain?',
    answer:
      'Kami menerima CDR, AI, PDF, JPG, dan PNG. Catatan: file CDR dan AI tidak bisa dipratinjau langsung di browser — staf kami mengunduh dan membukanya secara manual untuk verifikasi. File PDF, JPG, dan PNG bisa dipratinjau langsung di sistem.',
  },
  {
    question: 'Bagaimana cara pembayaran dan bagaimana proses verifikasinya?',
    answer:
      'Pembayaran dilakukan manual via transfer bank atau QRIS sesuai nomor rekening/kode QRIS yang kami tampilkan. Setelah transfer, unggah bukti pembayaran — staf kami memverifikasi dan status pesanan Anda diperbarui begitu terkonfirmasi.',
  },
  {
    question: 'Apakah ada ongkos kirim, dan bisa ambil sendiri (pickup)?',
    answer:
      'Ongkos kirim dihitung manual oleh admin setelah order masuk, sebelum tagihan pembayaran final dikirim ke Anda — bukan otomatis. Anda juga bisa memilih ambil sendiri (pickup) di lokasi kami untuk melewati ongkir sama sekali.',
  },
  {
    question: 'Bagaimana cara melacak status pesanan?',
    answer:
      'Setiap pesanan mendapat nomor resi unik (format RJK-XXXXXXXX) begitu order dibuat. Masukkan nomor resi tersebut di halaman Lacak Resi untuk melihat riwayat status pesanan Anda — tanpa perlu login.',
  },
]

const openIndex = ref<number | null>(0)

function toggle(i: number) {
  openIndex.value = openIndex.value === i ? null : i
}

const faqJsonLd = computed(() =>
  JSON.stringify({
    '@context': 'https://schema.org',
    '@type': 'FAQPage',
    mainEntity: faqs.map((f) => ({
      '@type': 'Question',
      name: f.question,
      acceptedAnswer: {
        '@type': 'Answer',
        text: f.answer,
      },
    })),
  }),
)

useHead({
  script: [
    {
      key: 'faq-jsonld',
      type: 'application/ld+json',
      innerHTML: faqJsonLd,
    },
  ],
})
</script>

<template>
  <section id="faq" class="bg-canvas-alt">
    <div class="mx-auto max-w-6xl px-4 py-16 md:py-24">
      <div class="max-w-2xl">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">FAQ</p>
        <h2 class="mt-3 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
          Pertanyaan yang sering ditanyakan
        </h2>
      </div>

      <motion.div
        class="mt-10 max-w-3xl divide-y divide-hairline border-t border-hairline"
        :initial="{ opacity: 0, y: prefersReduced ? 0 : 16 }"
        :while-in-view="{ opacity: 1, y: 0 }"
        :in-view-options="{ once: true, margin: '-100px' }"
        :transition="{ duration: prefersReduced ? 0 : 0.5, ease: [0.22, 1, 0.36, 1] }"
      >
        <div v-for="(f, i) in faqs" :key="f.question">
          <h3>
            <button
              :id="`faq-trigger-${i}`"
              type="button"
              class="flex w-full items-center justify-between gap-4 py-5 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt rounded-sm"
              :aria-expanded="openIndex === i"
              :aria-controls="`faq-panel-${i}`"
              @click="toggle(i)"
            >
              <span class="text-sm font-semibold text-ink-950 sm:text-base">{{ f.question }}</span>
              <ChevronDown
                class="h-4 w-4 shrink-0 text-ink-500 transition-transform duration-200 ease-out"
                :class="{ 'rotate-180': openIndex === i }"
                :stroke-width="1.5"
              />
            </button>
          </h3>
          <div
            :id="`faq-panel-${i}`"
            role="region"
            :aria-labelledby="`faq-trigger-${i}`"
            class="grid transition-[grid-template-rows] ease-out"
            :class="[
              openIndex === i ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]',
              prefersReduced ? 'duration-0' : 'duration-300',
            ]"
          >
            <div class="overflow-hidden">
              <p class="max-w-2xl pb-5 text-sm leading-relaxed text-ink-500">{{ f.answer }}</p>
            </div>
          </div>
        </div>
      </motion.div>
    </div>
  </section>
</template>
