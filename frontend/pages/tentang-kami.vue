<script setup lang="ts">
/**
 * /tentang-kami — Profil perusahaan.
 *
 * Data NAP & jam buka SATU-SATUNYA dari `~/utils/business.ts` (masih
 * placeholder TODO(rajaku) — jangan tulis ulang alamat/telepon literal di
 * sini). JSON-LD `AboutPage` merujuk (`about.@id`) ke LocalBusiness yang
 * sudah dipasang global di `layouts/default.vue` — tidak duplikasi @id.
 *
 * Mascot/logo (§26.8) hanya boleh muncul di halaman ini/footer, bukan di
 * admin/checkout — di sini dipakai `logo-full.webp` SATU KALI sebagai elemen
 * brand di section cerita. favicon.svg tetap dipakai sebagai monogram kecil
 * terpisah di header (bukan mascot, sekadar brand mark).
 *
 * Gambar dari `useSiteMedia()` (slot `tentang_workshop` & `tentang_tim`,
 * didaftarkan modul lain) dipakai untuk band foto lebar + section split
 * gambar/teks — dengan fallback statis eksplisit di sisi pemanggil ini
 * (bukan mengedit useSiteMedia.ts) supaya halaman tetap bergambar penuh
 * walau backend/API site-media mati. `await siteMediaReady` supaya SSR awal
 * langsung dapat gambar final, bukan fallback yang lalu "berkedip" ganti.
 *
 * Section "cara kami bekerja" adalah timeline vertikal berbasis alur order
 * asli (CLAUDE.md §4) — bukan klaim/statistik karangan.
 *
 * Momen orkestrasi utama halaman ini: blok cerita (emblem logo + foto tim)
 * reveal bersamaan sekali saat masuk viewport. Section lain hanya fade
 * tunggal tanpa stagger — halaman ini harus tetap terasa tenang.
 */
import {
  CheckCircle2,
  ClipboardList,
  Clock,
  Mail,
  PackageCheck,
  Phone,
  Printer,
  ShieldCheck,
  Sparkles,
  Truck,
  Users,
  Wallet,
} from '@lucide/vue'
import { motion } from 'motion-v'
import { business } from '~/utils/business'

// Fallback statis WAJIB di sisi pemanggil (bukan di useSiteMedia.ts — file
// itu tidak boleh disentuh di pekerjaan ini). Kalau slot belum diisi admin
// atau API site-media gagal, `resolve()` sudah jatuh ke fallback bawaan
// useSiteMedia.ts sendiri; `|| '/proses/...'` di sini jaga-jaga tambahan
// untuk kasus slot mengembalikan string kosong.
const { resolve: resolveMedia, ready: siteMediaReady } = useSiteMedia()
await siteMediaReady
const workshopImage = computed(() => resolveMedia('tentang_workshop') || '/proses/proses-04.webp')
const timImage = computed(() => resolveMedia('tentang_tim') || '/proses/proses-02.webp')

const prefersReducedMotion = usePrefersReducedMotion()
const fadeTransition = computed(() =>
  prefersReducedMotion.value ? { duration: 0 } : { duration: 0.5, ease: [0.22, 1, 0.36, 1] as const },
)
function storyDelay(step: number) {
  return prefersReducedMotion.value ? 0 : step * 0.1
}

definePageMeta({ layout: 'default' })

const config = useRuntimeConfig()
const canonical = `${config.public.appBaseUrl.replace(/\/$/, '')}/tentang-kami`
const metaDesc =
  'Kenali Rajaku Printing — percetakan banner dan digital printing large-format di Trenggalek, Jawa Timur. Cerita, keunggulan, dan cara menghubungi kami.'

useSeoMeta({
  title: 'Tentang Kami',
  description: metaDesc,
  ogTitle: 'Tentang Kami — Rajaku Printing',
  ogDescription: metaDesc,
  ogType: 'website',
  ogUrl: canonical,
  twitterCard: 'summary',
})

useHead({
  link: [{ rel: 'canonical', href: canonical }],
  script: [
    {
      type: 'application/ld+json',
      innerHTML: JSON.stringify({
        '@context': 'https://schema.org',
        '@type': 'AboutPage',
        name: 'Tentang Rajaku Printing',
        url: canonical,
        description: metaDesc,
        about: { '@id': `${config.public.appBaseUrl.replace(/\/$/, '')}/#business` },
      }),
    },
  ],
})

// Timeline "cara kami bekerja" — berbasis alur order asli §4, disederhanakan
// jadi 6 langkah supaya terbaca cepat (bukan daftar 20 status state machine
// mentah). Tidak ada klaim durasi/statistik yang tidak tercatat di sistem.
const workflowSteps = [
  {
    icon: ClipboardList,
    title: 'Order masuk',
    desc: 'Pesanan tercatat lewat website (upload desain / request desain) atau langsung di tempat.',
  },
  {
    icon: Wallet,
    title: 'Ongkir & pembayaran',
    desc: 'Admin hitung ongkir kalau dikirim, lalu pelanggan bayar transfer/QRIS/cash dan tim memverifikasi bukti transfer.',
  },
  {
    icon: ShieldCheck,
    title: 'Verifikasi desain',
    desc: 'File yang diunggah dicek, atau brief request desain dikerjakan tim sampai disetujui.',
  },
  {
    icon: Printer,
    title: 'Proses cetak',
    desc: 'Banner dicetak dengan mesin large-format sesuai bahan dan ukuran yang dipilih.',
  },
  {
    icon: CheckCircle2,
    title: 'Quality control',
    desc: 'Hasil cetak diperiksa dulu sebelum lanjut ke tahap pengambilan/pengiriman.',
  },
  {
    icon: PackageCheck,
    title: 'Siap diambil / dikirim',
    desc: 'Pelanggan ambil langsung di tempat, atau kami kirimkan ke alamat yang tercatat.',
  },
]

const values = [
  {
    icon: ShieldCheck,
    title: 'Transparan tiap tahap',
    desc: 'Status order, ongkir, dan verifikasi pembayaran bisa dipantau lewat nomor resi — tidak ada informasi yang disembunyikan.',
  },
  {
    icon: Sparkles,
    title: 'Kualitas cetak konsisten',
    desc: 'Kalibrasi mesin large-format rutin supaya warna hasil cetak sesuai desain, dari pesanan pertama sampai berikutnya.',
  },
  {
    icon: Users,
    title: 'Fleksibel sesuai kebutuhan',
    desc: 'Bawa desain sendiri atau serahkan ke tim kami — cocok untuk usaha kecil maupun kebutuhan event mendadak.',
  },
  {
    icon: Truck,
    title: 'Pickup atau dikirim',
    desc: 'Ambil langsung di tempat untuk hemat waktu, atau biar kami kirimkan ke alamat Anda.',
  },
]

function waLink(): string {
  return `https://wa.me/${business.whatsapp}`
}
</script>

<template>
  <main>
    <!-- Header -->
    <section class="mx-auto max-w-6xl px-4 pt-16 pb-8 md:pt-24 md:pb-10">
      <div class="flex items-start gap-4">
        <img
          src="/favicon.svg"
          alt="Monogram Rajaku Printing"
          width="40"
          height="40"
          loading="eager"
          class="mt-1 h-10 w-10 shrink-0 rounded-md ring-1 ring-inset ring-gold-500/30"
        >
        <div>
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Tentang Kami</p>
          <h1 class="mt-2 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
            Rajaku Printing
          </h1>
        </div>
      </div>
      <p class="mt-5 max-w-2xl text-sm md:text-base leading-relaxed text-ink-700">
        {{ business.description }} Kami melayani pelanggan perorangan, usaha kecil, hingga
        penyelenggara acara yang butuh hasil cetak rapi dengan proses yang jelas dari order
        sampai banner siap diambil atau dikirim.
      </p>
    </section>

    <!-- Band gambar lebar: ruang produksi -->
    <section class="mx-auto max-w-6xl px-4 pb-16 md:pb-24">
      <motion.div
        class="overflow-hidden rounded-lg border border-hairline"
        :initial="{ opacity: 0, y: 16 }"
        :while-in-view="{ opacity: 1, y: 0 }"
        :in-view-options="{ once: true, margin: '-80px' }"
        :transition="fadeTransition"
      >
        <img
          :src="workshopImage"
          alt="Ruang produksi Rajaku Printing, tempat mesin cetak large-format bekerja"
          width="1600"
          height="686"
          loading="eager"
          class="aspect-[4/3] w-full object-cover md:aspect-[21/9]"
        >
      </motion.div>
    </section>

    <!-- Cerita singkat — mascot logo (§26.8, satu-satunya kemunculan di halaman ini) -->
    <section class="mx-auto max-w-6xl px-4 pb-16 md:pb-24">
      <div class="rounded-lg border border-hairline bg-canvas p-6 md:p-10">
        <div class="grid gap-8 md:grid-cols-[220px_1fr] md:items-center">
          <motion.div
            class="mx-auto flex h-40 w-40 items-center justify-center rounded-lg border border-hairline bg-ink-950 p-6 md:mx-0 md:h-full md:w-full"
            :initial="{ opacity: 0, scale: 0.96 }"
            :while-in-view="{ opacity: 1, scale: 1 }"
            :in-view-options="{ once: true, margin: '-80px' }"
            :transition="{ ...fadeTransition, delay: storyDelay(0) }"
          >
            <img
              src="/brand/logo-full.webp"
              alt="Logo Rajaku Printing"
              width="1200"
              height="1153"
              loading="lazy"
              class="h-full w-full object-contain"
            >
          </motion.div>

          <motion.div
            :initial="{ opacity: 0, y: 12 }"
            :while-in-view="{ opacity: 1, y: 0 }"
            :in-view-options="{ once: true, margin: '-80px' }"
            :transition="{ ...fadeTransition, delay: storyDelay(1) }"
          >
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Cerita kami</p>
            <h2 class="mt-3 max-w-2xl text-lg md:text-xl font-serif font-medium leading-relaxed text-ink-950">
              Dibangun dari kebutuhan sederhana: banner yang tepat waktu, warnanya presisi, dan
              prosesnya tidak bikin bingung.
            </h2>
            <p class="mt-4 max-w-2xl text-sm leading-relaxed text-ink-600">
              Rajaku Printing berangkat dari pengalaman melayani kebutuhan cetak sehari-hari di
              {{ business.addressLocality }} dan sekitarnya — mulai dari banner toko, spanduk acara,
              sampai kebutuhan cetak mendadak yang tidak bisa menunggu lama. Setiap order kami
              perlakukan dengan proses yang sama: jelas di awal soal harga dan bahan, transparan soal
              status pengerjaan, dan konsisten soal kualitas hasil cetak.
            </p>
          </motion.div>
        </div>
      </div>
    </section>

    <!-- Split gambar + teks: standar kerja, bukan biografi -->
    <section class="border-y border-hairline bg-canvas-alt">
      <div class="mx-auto max-w-6xl px-4 py-16 md:py-24">
        <motion.div
          class="grid gap-8 md:grid-cols-2 md:items-center"
          :initial="{ opacity: 0, y: 16 }"
          :while-in-view="{ opacity: 1, y: 0 }"
          :in-view-options="{ once: true, margin: '-100px' }"
          :transition="fadeTransition"
        >
          <div class="overflow-hidden rounded-lg border border-hairline order-2 md:order-1">
            <img
              :src="timImage"
              alt="Tim Rajaku Printing bekerja memeriksa hasil cetak"
              width="1000"
              height="750"
              loading="lazy"
              class="aspect-[4/3] w-full object-cover"
            >
          </div>
          <div class="order-1 md:order-2">
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Cara kami bekerja</p>
            <h2 class="mt-3 text-lg md:text-xl font-sans font-semibold text-ink-950">
              Tiap pesanan diperiksa manusia, bukan cuma mesin
            </h2>
            <p class="mt-3 text-sm leading-relaxed text-ink-600">
              File desain yang Anda kirim dicek dulu sebelum masuk antrean cetak — supaya format,
              ukuran, dan resolusinya sesuai untuk hasil large-format. Setelah dicetak, hasilnya
              melewati pemeriksaan kualitas sebelum dinyatakan siap diambil atau dikirim.
            </p>
            <p class="mt-3 text-sm leading-relaxed text-ink-600">
              Pembayaran diverifikasi manual oleh tim kami dari bukti transfer yang Anda unggah —
              tidak ada proses otomatis yang bisa salah baca nominal. Kalau ada yang perlu
              direvisi, kami hubungi lewat WhatsApp sebelum lanjut ke tahap berikutnya.
            </p>
          </div>
        </motion.div>
      </div>
    </section>

    <!-- Timeline: cara kami bekerja (alur order §4) -->
    <section class="mx-auto max-w-6xl px-4 py-16 md:py-24">
      <motion.div
        class="max-w-2xl"
        :initial="{ opacity: 0, y: 12 }"
        :while-in-view="{ opacity: 1, y: 0 }"
        :in-view-options="{ once: true, margin: '-80px' }"
        :transition="fadeTransition"
      >
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Alur pesanan</p>
        <h2 class="mt-3 text-lg md:text-xl font-sans font-semibold text-ink-950">
          Dari order masuk sampai siap di tangan Anda
        </h2>
      </motion.div>

      <motion.ol
        class="relative mt-10 max-w-2xl border-l border-hairline pl-8 space-y-8"
        :initial="{ opacity: 0, y: 16 }"
        :while-in-view="{ opacity: 1, y: 0 }"
        :in-view-options="{ once: true, margin: '-100px' }"
        :transition="fadeTransition"
      >
        <li v-for="(step, idx) in workflowSteps" :key="step.title" class="relative">
          <span
            class="absolute -left-[41px] flex h-8 w-8 items-center justify-center rounded-full border border-hairline bg-canvas text-brand-500"
          >
            <component :is="step.icon" class="h-4 w-4" :stroke-width="1.5" />
          </span>
          <p class="font-mono text-[10px] text-ink-400">Langkah {{ idx + 1 }}</p>
          <h3 class="mt-0.5 text-sm font-sans font-semibold text-ink-950">{{ step.title }}</h3>
          <p class="mt-1 text-sm leading-relaxed text-ink-500">{{ step.desc }}</p>
        </li>
      </motion.ol>
    </section>

    <!-- Keunggulan / values -->
    <section class="bg-canvas-alt border-y border-hairline">
      <div class="mx-auto max-w-6xl px-4 py-16 md:py-24">
        <div class="max-w-2xl">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Kenapa memilih kami</p>
          <h2 class="mt-3 text-lg md:text-xl font-sans font-semibold text-ink-950">
            Nilai yang kami pegang di setiap order
          </h2>
        </div>

        <motion.ul
          class="mt-10 grid gap-6 sm:grid-cols-2"
          :initial="{ opacity: 0, y: 16 }"
          :while-in-view="{ opacity: 1, y: 0 }"
          :in-view-options="{ once: true, margin: '-100px' }"
          :transition="fadeTransition"
        >
          <li v-for="v in values" :key="v.title" class="flex gap-4">
            <span
              class="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-hairline bg-canvas text-brand-500"
            >
              <component :is="v.icon" class="h-5 w-5" :stroke-width="1.5" />
            </span>
            <div>
              <h3 class="text-sm font-sans font-semibold text-ink-950">{{ v.title }}</h3>
              <p class="mt-1 text-sm leading-relaxed text-ink-500">{{ v.desc }}</p>
            </div>
          </li>
        </motion.ul>
      </div>
    </section>

    <!-- Kontak & jam buka -->
    <section class="mx-auto max-w-6xl px-4 py-16 md:py-24">
      <motion.div
        class="grid gap-10 md:grid-cols-2"
        :initial="{ opacity: 0, y: 16 }"
        :while-in-view="{ opacity: 1, y: 0 }"
        :in-view-options="{ once: true, margin: '-100px' }"
        :transition="fadeTransition"
      >
        <div>
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Kontak</p>
          <h2 class="mt-3 text-lg md:text-xl font-sans font-semibold text-ink-950">
            Hubungi atau kunjungi kami
          </h2>

          <dl class="mt-6 space-y-4">
            <div class="flex items-start gap-3">
              <Phone class="mt-0.5 h-4 w-4 shrink-0 text-brand-500" :stroke-width="1.5" />
              <dd>
                <a
                  :href="`tel:${business.telephone}`"
                  class="font-mono text-xs text-ink-700 hover:text-brand-500 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas rounded-sm"
                >
                  {{ business.telephone }}
                </a>
              </dd>
            </div>
            <div class="flex items-start gap-3">
              <Mail class="mt-0.5 h-4 w-4 shrink-0 text-brand-500" :stroke-width="1.5" />
              <dd>
                <a
                  :href="`mailto:${business.email}`"
                  class="text-sm text-ink-700 hover:text-brand-500 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas rounded-sm"
                >
                  {{ business.email }}
                </a>
              </dd>
            </div>
            <div class="flex items-start gap-3">
              <Clock class="mt-0.5 h-4 w-4 shrink-0 text-brand-500" :stroke-width="1.5" />
              <dd class="space-y-1">
                <p v-for="h in business.openingHours" :key="h.label" class="text-sm leading-relaxed text-ink-700">
                  {{ h.label }}
                </p>
              </dd>
            </div>
          </dl>

          <div class="mt-6 flex flex-wrap gap-2">
            <span
              v-for="area in business.serviceArea"
              :key="area"
              class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium text-ink-600 bg-canvas-alt ring-1 ring-inset ring-hairline"
            >
              {{ area }}
            </span>
          </div>

          <a
            :href="waLink()"
            target="_blank"
            rel="noopener noreferrer"
            class="mt-8 inline-flex items-center justify-center gap-2 rounded-md bg-brand-500 px-6 py-3 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          >
            Chat via WhatsApp
          </a>
        </div>

        <!--
          Alamat & peta tinggal DI SINI saja (LocationCard), tidak diulang di
          daftar kontak sebelahnya — alamat yang tercetak dua kali dalam satu
          layar terbaca sebagai kelalaian, bukan sebagai penegasan.
        -->
        <LocationCard />
      </motion.div>
    </section>

    <!--
      Ajakan order dipindah keluar dari kolom kanan section kontak (dulu kartu
      gelap kecil di sebelah alamat) jadi pita gelap selebar layar tepat
      sebelum footer. Ini pola penutup yang sama dengan landing (§26.12):
      section gelap menempel ke footer gelap, jadi kaki halaman terbaca sebagai
      satu penutup utuh, bukan tambalan.
    -->
    <section class="bg-ink-950">
      <motion.div
        class="mx-auto max-w-2xl px-4 py-16 text-center md:py-20"
        :initial="{ opacity: 0, y: 16 }"
        :while-in-view="{ opacity: 1, y: 0 }"
        :in-view-options="{ once: true, margin: '-40px' }"
        :transition="fadeTransition"
      >
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-gold-400">Siap dibantu</p>
        <h2 class="mt-3 font-serif text-xl md:text-2xl font-semibold text-canvas">
          Punya kebutuhan cetak? Mulai order kapan saja.
        </h2>
        <p class="mx-auto mt-3 max-w-md text-sm leading-relaxed text-canvas/70">
          Order online lewat website, atau datang langsung ke toko kami —
          {{ business.landmark }} — untuk konsultasi bahan dan ukuran.
        </p>
        <NuxtLink
          to="/order"
          class="mt-8 inline-flex items-center justify-center gap-2 rounded-md bg-brand-500 px-6 py-3 text-sm font-semibold text-canvas transition-colors duration-200 ease-out hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
        >
          Order Banner
        </NuxtLink>
      </motion.div>
    </section>
  </main>
</template>
