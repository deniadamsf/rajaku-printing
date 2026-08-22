<script setup lang="ts">
/**
 * MaterialsSection — split 2 kolom: bahan cetak nyata dari modul catalog (§9).
 *
 * Kiri: foto workshop (slot sitemedia `bahan_detail`, fallback
 * `/proses/proses-07.webp` — detail panel kontrol & tabung tinta, relevan
 * dengan tema "bahan & kualitas cetak").
 * Kanan: daftar bahan NYATA dari `useCatalog().listMaterials()` (kode + nama +
 * deskripsi apa adanya dari backend — tidak ada spek dikarang).
 *
 * Anti-kosong: kalau fetch gagal/kosong, tampilkan 4 bahan yang memang ada di
 * seed sistem (`backend/migrations/000002_catalog_schema.up.sql`) — deskripsi
 * disalin apa adanya dari migration, bukan karangan baru.
 */
import { motion } from 'motion-v'
import { FALLBACK_MATERIALS } from '~/utils/catalog-fallback'
import type { CatalogMaterial } from '~/types/catalog'

const catalog = useCatalog()
const { container, item } = useRevealVariants()
// Sorotan kursor, pola yang sama dengan kartu produk & kartu langkah.
const { onPointerMove } = useSpotlight()
const prefersReduced = usePrefersReducedMotion()

const { data } = await useAsyncData('landing-materials-section', async () => {
  try {
    const res = await catalog.listMaterials()
    return res.materials.length > 0 ? res.materials : FALLBACK_MATERIALS
  } catch {
    // Backend mati/gagal → daftar bahan nyata yang memang ada di seed sistem,
    // bukan kotak error di isi utama halaman.
    return FALLBACK_MATERIALS
  }
})

const materials = computed<ReadonlyArray<CatalogMaterial>>(() => data.value ?? FALLBACK_MATERIALS)

const { resolve: resolveMedia, ready: siteMediaReady } = useSiteMedia()
await siteMediaReady
const detailImage = computed(() => resolveMedia('bahan_detail'))
</script>

<template>
  <section id="bahan" class="mx-auto max-w-6xl px-4 py-12 md:py-20">
    <div class="grid gap-10 md:grid-cols-2 md:items-start md:gap-16">
      <motion.div
        class="relative aspect-[4/3] overflow-hidden rounded-lg border border-hairline bg-canvas-alt"
        :initial="{ opacity: 0, y: prefersReduced ? 0 : 16 }"
        :while-in-view="{ opacity: 1, y: 0 }"
        :in-view-options="{ once: true, margin: '-40px' }"
        :transition="{ duration: prefersReduced ? 0 : 0.5, ease: [0.22, 1, 0.36, 1] }"
      >
        <img
          :src="detailImage"
          alt="Detail panel kontrol dan tabung tinta mesin cetak large-format Rajaku Printing"
          width="800"
          height="800"
          loading="lazy"
          class="h-full w-full object-cover"
        >
      </motion.div>

      <div>
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Bahan Cetak</p>
        <h2 class="mt-3 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
          Bahan yang kami pakai
        </h2>
        <p class="mt-3 text-sm leading-relaxed text-ink-500">
          Bahan dipilih sesuai kebutuhan outdoor/indoor — tersedia untuk dipilih langsung saat
          Anda menghitung estimasi harga atau order.
        </p>

        <!--
          Daftar bahan dulunya satu kolom bergaris pemisah — dengan 7-8 bahan,
          bentuk itu memanjang jauh ke bawah dan membuat kolom sebelahnya
          (foto) menggantung sendirian di ruang kosong.

          Sekarang kartu dua kolom pada layar lebar: tinggi section turun
          drastis, tiap bahan terbaca sebagai unit sendiri, dan kodenya (yang
          dipakai pelanggan saat memesan) mendapat tempat yang jelas.
        -->
        <motion.dl
          class="mt-8 grid grid-cols-2 gap-3"
          :variants="container"
          initial="hidden"
          while-in-view="show"
          :in-view-options="{ once: true, margin: '-40px' }"
          @pointermove="onPointerMove"
        >
          <motion.div
            v-for="m in materials"
            :key="m.id"
            :variants="item"
            class="spotlight rounded-lg border border-hairline bg-canvas p-4 transition-colors duration-200 ease-out hover:border-ink-300"
          >
            <dt class="relative z-10 flex flex-col gap-1 sm:flex-row sm:items-baseline sm:justify-between sm:gap-2">
              <span class="text-sm font-semibold leading-snug text-ink-950">{{ m.name }}</span>
              <span class="shrink-0 font-mono text-[10px] font-normal text-ink-400">{{ m.code }}</span>
            </dt>
            <!--
              Deskripsi disembunyikan di layar terkecil, BUKAN dikecilkan
              terus-menerus: di lebar dua kolom pada HP, kartunya tinggal
              ~160px dan deskripsi penuh membuat tiap kartu beda tinggi jauh
              serta terbaca sebagai gumpalan teks. Yang dibutuhkan pengunjung
              di sini nama & kode bahannya; keterangan lengkap tetap ada di
              halaman katalog.
            -->
            <dd
              v-if="m.description"
              class="relative z-10 mt-1.5 hidden text-xs leading-relaxed text-ink-500 sm:block"
            >
              {{ m.description }}
            </dd>
          </motion.div>
        </motion.dl>
      </div>
    </div>
  </section>
</template>
