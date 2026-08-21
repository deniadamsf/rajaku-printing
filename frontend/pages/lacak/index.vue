<script setup lang="ts">
/**
 * /lacak — Landing untuk input resi. Redirect ke /lacak/:resi setelah user
 * kirim.
 */
import { MessageCircle, Monitor, Receipt, Search } from '@lucide/vue'

// Tiga tempat resi benar-benar muncul di sistem ini: notifikasi WhatsApp (§13),
// struk thermal untuk order di tempat (§12), dan halaman konfirmasi setelah
// order online selesai dibuat. Jangan menambah sumber yang tidak ada.
const resiSources = [
  {
    icon: MessageCircle,
    title: 'Pesan WhatsApp',
    desc: 'Kami mengirim nomor resi ke nomor WhatsApp yang Anda isi saat order.',
  },
  {
    icon: Receipt,
    title: 'Struk pembelian',
    desc: 'Untuk pesanan di tempat, nomor resi tercetak di struk yang diberikan kasir.',
  },
  {
    icon: Monitor,
    title: 'Halaman konfirmasi',
    desc: 'Setelah order online tercatat, nomor resi langsung tampil di layar konfirmasi.',
  },
]

definePageMeta({ layout: 'default' })

useSeoMeta({
  title: 'Lacak Pesanan',
  description: 'Cek status pesanan Anda dengan memasukkan nomor resi RJK-XXXXXXXX.',
  robots: 'noindex,nofollow',
})

const resi = ref('')

// Normalisasi format resi — kapital + trim + auto-add prefix RJK- kalau user
// hanya input suffix (8 alfanumerik).
const normalizedResi = computed(() => {
  const raw = resi.value.trim().toUpperCase().replace(/\s+/g, '')
  if (!raw) return ''
  if (raw.startsWith('RJK-')) return raw
  if (/^[A-Z0-9]+$/.test(raw)) return `RJK-${raw}`
  return raw
})

const canSubmit = computed(() => /^RJK-[A-Z0-9]+$/.test(normalizedResi.value))

async function onSubmit() {
  if (!canSubmit.value) return
  await navigateTo(`/lacak/${normalizedResi.value}`)
}
</script>

<template>
  <main class="relative overflow-hidden">
    <!--
      Halaman satu-form. Sebelumnya berupa kolom sempit di kiri layar kosong;
      sekarang kartu pencarian jadi pusat halaman di atas ornamen kisi, dengan
      tiga keterangan "di mana menemukan resi" yang mengisi ruang sekaligus
      menjawab pertanyaan paling sering muncul di jalur ini.
    -->
    <div class="pointer-events-none absolute inset-x-0 top-0 h-96" aria-hidden="true">
      <ArtOrnament variant="grid" :opacity="0.6" />
    </div>

    <div class="relative mx-auto max-w-2xl px-4 py-16 md:py-24">
      <div class="text-center">
        <p
          class="flex items-center justify-center gap-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500"
        >
          <Search class="h-3.5 w-3.5" :stroke-width="1.75" />
          Lacak Pesanan
        </p>
        <h1
          class="mt-3 font-serif text-3xl md:text-4xl font-semibold tracking-tight text-ink-950"
        >
          Cek status pesanan Anda
        </h1>
        <p class="mx-auto mt-3 max-w-md text-sm leading-relaxed text-ink-500">
          Masukkan nomor resi yang Anda terima setelah order. Tidak perlu login.
        </p>
      </div>

      <div class="mt-10 rounded-lg border border-hairline bg-canvas p-6 shadow-sm md:p-8">
        <form class="space-y-4" @submit.prevent="onSubmit">
          <div>
            <label for="lacak-resi" class="block text-sm font-medium text-ink-900">
              Nomor resi
            </label>
            <input
              id="lacak-resi"
              v-model="resi"
              type="text"
              required
              autofocus
              autocomplete="off"
              spellcheck="false"
              placeholder="RJK-8F3K2A9X"
              class="mt-2 block w-full rounded-md border border-hairline bg-canvas px-4 py-3 font-mono text-base uppercase tracking-[0.12em] text-ink-900 placeholder-ink-400 transition-colors focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20"
            >
            <p v-if="resi && !canSubmit" class="mt-2 text-xs text-brand-700">
              Format resi tidak valid. Contoh: <span class="font-mono">RJK-8F3K2A9X</span>.
            </p>
            <p v-else class="mt-2 text-xs text-ink-500">
              Format: <span class="font-mono text-ink-700">RJK-XXXXXXXX</span>. Awalan
              <span class="font-mono text-ink-700">RJK-</span> otomatis ditambahkan.
            </p>
          </div>

          <button
            type="submit"
            :disabled="!canSubmit"
            class="inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-3 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Search class="h-4 w-4" :stroke-width="1.75" />
            Cek status
          </button>
        </form>
      </div>

      <div class="mt-10">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
          Di mana nomor resi saya?
        </p>
        <ul class="mt-4 grid gap-4 sm:grid-cols-3">
          <li
            v-for="source in resiSources"
            :key="source.title"
            class="rounded-lg border border-hairline bg-canvas p-5"
          >
            <component :is="source.icon" class="h-5 w-5 text-brand-500" :stroke-width="1.5" />
            <h2 class="mt-3 text-sm font-semibold text-ink-950">{{ source.title }}</h2>
            <p class="mt-1 text-sm leading-relaxed text-ink-500">{{ source.desc }}</p>
          </li>
        </ul>
      </div>

      <p class="mt-8 text-center text-sm text-ink-500">
        Belum pernah order?
        <NuxtLink
          to="/order"
          class="rounded-sm font-medium text-brand-500 transition-colors hover:text-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
        >
          Order banner sekarang
        </NuxtLink>
        — nomor resi terbit begitu pesanan tercatat.
      </p>
    </div>
  </main>
</template>
