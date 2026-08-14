<script setup lang="ts">
/**
 * ClosingCta — ajakan order penutup + info kontak (NAP) singkat.
 *
 * Data NAP diambil dari `~/utils/business.ts` (satu-satunya sumber, §business.ts
 * docblock) — bukan ditulis ulang di sini. Section ini jadi tempat "kontak"
 * dipakai di landing page, karena `layouts/default.vue` sengaja tidak diubah
 * strukturnya (hanya boleh tambah link nav & JSON-LD).
 */
import { ArrowRight, Clock, MapPin, Phone, Search } from '@lucide/vue'
import { business } from '~/utils/business'
</script>

<template>
  <section class="mx-auto max-w-6xl px-4 py-16 md:py-24">
    <div class="rounded-lg border border-hairline bg-ink-950 p-8 md:p-12">
      <div class="grid gap-10 md:grid-cols-[1.3fr_1fr] md:items-center">
        <div class="text-center md:text-left">
          <h2 class="text-2xl md:text-3xl font-serif font-semibold tracking-tight text-canvas">
            Siap cetak banner Anda?
          </h2>
          <p class="mt-3 max-w-md text-sm leading-relaxed text-canvas/70 mx-auto md:mx-0">
            Mulai order sekarang, atau lacak progres pesanan yang sudah berjalan
            pakai nomor resi.
          </p>

          <div class="mt-8 flex flex-col sm:flex-row items-center justify-center md:justify-start gap-3">
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
        </div>

        <dl class="space-y-4 border-t border-canvas/15 pt-6 md:border-t-0 md:border-l md:pt-0 md:pl-10">
          <div class="flex items-start gap-3">
            <MapPin class="mt-0.5 h-4 w-4 shrink-0 text-gold-400" :stroke-width="1.5" />
            <dd class="text-sm leading-relaxed text-canvas/80">
              {{ business.streetAddress }}, {{ business.addressLocality }},
              {{ business.addressRegion }} {{ business.postalCode }}
            </dd>
          </div>
          <div class="flex items-start gap-3">
            <Phone class="mt-0.5 h-4 w-4 shrink-0 text-gold-400" :stroke-width="1.5" />
            <dd class="font-mono text-xs text-canvas/80">{{ business.telephone }}</dd>
          </div>
          <div class="flex items-start gap-3">
            <Clock class="mt-0.5 h-4 w-4 shrink-0 text-gold-400" :stroke-width="1.5" />
            <dd class="text-sm leading-relaxed text-canvas/80">
              {{ business.openingHours[0]?.label }}
            </dd>
          </div>
        </dl>
      </div>
    </div>
  </section>
</template>
