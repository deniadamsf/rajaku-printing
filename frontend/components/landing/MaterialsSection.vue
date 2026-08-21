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
  <section id="bahan" class="mx-auto max-w-6xl px-4 py-16 md:py-24">
    <div class="grid gap-10 md:grid-cols-2 md:items-center md:gap-16">
      <motion.div
        class="relative aspect-[4/3] overflow-hidden rounded-lg border border-hairline bg-canvas-alt"
        :initial="{ opacity: 0, y: prefersReduced ? 0 : 16 }"
        :while-in-view="{ opacity: 1, y: 0 }"
        :in-view-options="{ once: true, margin: '-100px' }"
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

        <motion.dl
          class="mt-8 divide-y divide-hairline border-t border-hairline"
          :variants="container"
          initial="hidden"
          while-in-view="show"
          :in-view-options="{ once: true, margin: '-100px' }"
        >
          <motion.div v-for="m in materials" :key="m.id" :variants="item" class="py-4">
            <dt class="flex flex-wrap items-baseline gap-2 text-sm font-semibold text-ink-950">
              {{ m.name }}
              <span class="font-mono text-[10px] font-normal text-ink-500">{{ m.code }}</span>
            </dt>
            <dd v-if="m.description" class="mt-1 text-sm leading-relaxed text-ink-500">
              {{ m.description }}
            </dd>
          </motion.div>
        </motion.dl>
      </div>
    </div>
  </section>
</template>
