<script setup lang="ts">
/**
 * /admin/site-media — kelola gambar landing page per slot (modul `sitemedia`).
 *
 * Setiap slot registry SELALU muncul (backend ListAdmin mengembalikan semua
 * slot, terisi maupun kosong — lihat `AdminSiteMediaItem`). Slot kosong tampil
 * dengan penanda "belum diatur" dan frontend publik otomatis jatuh ke aset
 * statis bawaan (`useSiteMedia.ts`).
 *
 * Cache-busting: backend menyajikan file di URL yang SAMA per slot
 * (`.../site-media/file/:slot`), jadi setelah upload/hapus kita tambahkan
 * query `?v=timestamp` ke `<img src>` supaya admin melihat hasil sebenarnya,
 * bukan gambar lama dari cache browser.
 *
 * Design: patuh CLAUDE.md §26 — pola sama dengan upload cover artikel
 * (`pages/admin/artikel/[id].vue`) dan setting page (`pages/admin/pengaturan`).
 */
import { Images, Upload, RotateCcw, Loader2, TriangleAlert, ImageOff } from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import { useAdminSiteMedia, type AdminSiteMediaItem } from '~/composables/useAdminSiteMedia'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Media Landing Page — Rajaku Admin', robots: 'noindex,nofollow' })

const auth = useAuthStore()
const siteMediaApi = useAdminSiteMedia()

const canManage = computed(() => auth.hasPermission('sitemedia.manage'))

const items = ref<AdminSiteMediaItem[]>([])
const loading = ref(false)
const errorMsg = ref('')
const successMsg = ref('')

// Busy state per-slot — supaya kartu lain tetap interaktif saat satu slot
// sedang upload/hapus, bukan mengunci seluruh halaman.
const uploadingSlot = ref<string | null>(null)
const deletingSlot = ref<string | null>(null)

// Cache-buster per slot, dibump tiap upload/hapus sukses (lihat docblock atas).
const cacheBust = reactive<Record<string, number>>({})

function previewSrc(item: AdminSiteMediaItem): string | null {
  if (!item.url) return null
  const v = cacheBust[item.slot]
  return v ? `${item.url}?v=${v}` : item.url
}

const MAX_BYTES = 5 * 1024 * 1024
const ALLOWED_MIME = ['image/jpeg', 'image/png', 'image/webp']

async function fetchItems() {
  loading.value = true
  errorMsg.value = ''
  try {
    const res = await siteMediaApi.list()
    items.value = res.items
  } catch (e) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal memuat media landing page'
  } finally {
    loading.value = false
  }
}

const fileInputRefs: Record<string, HTMLInputElement | null> = {}
function setFileInputRef(slot: string, el: Element | null) {
  fileInputRefs[slot] = (el as HTMLInputElement) ?? null
}

async function onFileChange(item: AdminSiteMediaItem, ev: Event) {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return

  errorMsg.value = ''
  successMsg.value = ''

  // Validasi klien — kenyamanan (jangan tunggu upload besar cuma buat
  // ditolak server), BUKAN pengganti validasi server yang tetap berlaku.
  if (!ALLOWED_MIME.includes(file.type)) {
    errorMsg.value = `${item.label}: format harus JPEG, PNG, atau WebP.`
    input.value = ''
    return
  }
  if (file.size > MAX_BYTES) {
    errorMsg.value = `${item.label}: ukuran file melebihi 5 MB.`
    input.value = ''
    return
  }

  uploadingSlot.value = item.slot
  try {
    await siteMediaApi.upload(item.slot, file)
    // Refetch penuh — response POST tidak membawa field `url` siap pakai
    // (lihat docblock useAdminSiteMedia.ts).
    await fetchItems()
    cacheBust[item.slot] = Date.now()
    successMsg.value = `${item.label} berhasil diganti.`
    setTimeout(() => (successMsg.value = ''), 3000)
  } catch (e) {
    errorMsg.value = e instanceof ApiError ? e.message : `Gagal upload ${item.label}`
  } finally {
    uploadingSlot.value = null
    input.value = ''
  }
}

// Konfirmasi "kembalikan ke bawaan" — destruktif, wajib konfirmasi (§26.7).
const confirmSlot = ref<AdminSiteMediaItem | null>(null)
const confirmOpen = computed({
  get: () => !!confirmSlot.value,
  set: (v: boolean) => {
    if (!v) confirmSlot.value = null
  },
})

function askReset(item: AdminSiteMediaItem) {
  confirmSlot.value = item
}

async function confirmReset() {
  const item = confirmSlot.value
  if (!item) return
  deletingSlot.value = item.slot
  errorMsg.value = ''
  successMsg.value = ''
  try {
    await siteMediaApi.remove(item.slot)
    await fetchItems()
    cacheBust[item.slot] = Date.now()
    successMsg.value = `${item.label} dikembalikan ke gambar bawaan.`
    setTimeout(() => (successMsg.value = ''), 3000)
  } catch (e) {
    errorMsg.value = e instanceof ApiError ? e.message : `Gagal mengembalikan ${item.label}`
  } finally {
    deletingSlot.value = null
    confirmSlot.value = null
  }
}

function formatBytes(n: number | null): string {
  if (!n) return '—'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(2)} MB`
}

function formatDate(iso: string | null): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' })
}

onMounted(fetchItems)
</script>

<template>
  <section>
    <AdminPageHeader
      title="Media Landing Page"
      subtitle="Ganti gambar hero, logo, dan proses cetak di landing page tanpa deploy ulang. Slot kosong otomatis memakai gambar bawaan."
    />

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />
    <AlertMessage v-if="successMsg" variant="success" :message="successMsg" class="mb-4" />

    <div
      v-if="!canManage"
      class="mb-6 flex items-start gap-2 rounded-md border border-hairline bg-canvas-alt p-4 text-xs text-ink-500"
    >
      <TriangleAlert class="mt-0.5 h-4 w-4 shrink-0" :stroke-width="1.5" />
      <p>Anda tidak punya izin mengelola media landing page — halaman ini tampil sebagai referensi saja.</p>
    </div>

    <div v-if="loading" class="rounded-lg border border-hairline bg-canvas p-8 text-center">
      <Loader2 class="mx-auto h-5 w-5 animate-spin text-ink-400" :stroke-width="1.5" />
      <p class="mt-2 text-xs text-ink-500">Memuat media…</p>
    </div>

    <div v-else class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
      <div v-for="item in items" :key="item.slot" class="rounded-lg border border-hairline bg-canvas p-6">
        <div class="flex items-start gap-2">
          <Images class="mt-0.5 h-4 w-4 shrink-0 text-ink-400" :stroke-width="1.5" />
          <div class="min-w-0">
            <h3 class="text-sm font-semibold text-ink-900">{{ item.label }}</h3>
            <p class="mt-1 text-xs leading-relaxed text-ink-500">{{ item.description }}</p>
          </div>
        </div>

        <div class="mt-3 flex items-center justify-between text-[10px] text-ink-500">
          <span class="font-mono">{{ item.slot }}</span>
          <span>Disarankan <span class="font-mono">{{ item.suggested_width_px }}×{{ item.suggested_height_px }}px</span></span>
        </div>

        <div class="mt-3 aspect-[4/3] overflow-hidden rounded-md border border-hairline bg-canvas-alt">
          <img
            v-if="previewSrc(item)"
            :src="previewSrc(item)!"
            :alt="item.label"
            class="h-full w-full object-cover"
          >
          <div v-else class="flex h-full flex-col items-center justify-center gap-1.5 text-ink-400">
            <ImageOff class="h-6 w-6" :stroke-width="1.5" />
            <span class="text-[10px]">Belum diatur — memakai gambar bawaan</span>
          </div>
        </div>

        <dl v-if="item.url" class="mt-3 space-y-0.5 text-[10px] text-ink-500">
          <div class="flex justify-between gap-2">
            <dt class="shrink-0">Berkas</dt>
            <dd class="truncate font-mono text-ink-700" :title="item.original_name || ''">{{ item.original_name }}</dd>
          </div>
          <div class="flex justify-between gap-2">
            <dt class="shrink-0">Ukuran</dt>
            <dd class="font-mono text-ink-700">{{ formatBytes(item.size_bytes) }} · {{ item.width_px }}×{{ item.height_px }}px</dd>
          </div>
          <div class="flex justify-between gap-2">
            <dt class="shrink-0">Diperbarui</dt>
            <dd class="text-ink-700">{{ formatDate(item.updated_at) }}</dd>
          </div>
        </dl>

        <div class="mt-4 flex items-center gap-2">
          <label
            :class="[
              'inline-flex flex-1 items-center justify-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-xs font-semibold text-canvas transition-colors hover:bg-brand-600 focus-within:outline-none focus-within:ring-2 focus-within:ring-brand-500/40 focus-within:ring-offset-2 focus-within:ring-offset-canvas',
              !canManage || uploadingSlot === item.slot ? 'pointer-events-none opacity-50' : 'cursor-pointer',
            ]"
          >
            <Loader2 v-if="uploadingSlot === item.slot" class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
            <Upload v-else class="h-3.5 w-3.5" :stroke-width="1.75" />
            {{ uploadingSlot === item.slot ? 'Mengunggah…' : item.url ? 'Ganti gambar' : 'Unggah gambar' }}
            <input
              :ref="(el) => setFileInputRef(item.slot, el as Element | null)"
              type="file"
              accept="image/jpeg,image/png,image/webp"
              class="hidden"
              :disabled="!canManage || uploadingSlot === item.slot"
              @change="onFileChange(item, $event)"
            >
          </label>
          <button
            type="button"
            :disabled="!canManage || !item.url || deletingSlot === item.slot"
            class="inline-flex items-center justify-center gap-1.5 rounded-md border border-hairline bg-canvas px-3 py-2 text-xs font-medium text-ink-700 transition-colors hover:border-ink-300 hover:bg-canvas-alt focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-40"
            :aria-label="`Kembalikan ${item.label} ke gambar bawaan`"
            @click="askReset(item)"
          >
            <Loader2 v-if="deletingSlot === item.slot" class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
            <RotateCcw v-else class="h-3.5 w-3.5" :stroke-width="1.75" />
          </button>
        </div>
      </div>
    </div>

    <AdminConfirmDialog
      v-model:open="confirmOpen"
      title="Kembalikan ke gambar bawaan?"
      :message="
        confirmSlot
          ? `${confirmSlot.label} akan berhenti memakai gambar unggahan dan kembali ke aset statis bawaan. File yang sedang dipakai akan dihapus dari server.`
          : ''
      "
      confirm-label="Ya, kembalikan"
      variant="danger"
      :loading="!!deletingSlot"
      @confirm="confirmReset"
    />
  </section>
</template>
