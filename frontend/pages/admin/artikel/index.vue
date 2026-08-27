<script setup lang="ts">
import type { Article, ArticleStatus } from '~/types/cms'
import type { DataTableColumn } from '~/components/admin/DataTable.vue'
import { ApiError } from '~/composables/useApi'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Artikel — Admin' })

const auth = useAuthStore()
const cms = useCms()

const canCreate = computed(() => auth.hasPermission('article.create'))
const canPublish = computed(() => auth.hasPermission('article.publish'))

const page = ref(1)
const limit = ref(10)
const statusFilter = ref<ArticleStatus | ''>('')
const search = ref('')

const items = ref<Article[]>([])
const total = ref(0)
const loading = ref(false)
const errorMsg = ref<string | null>(null)

/**
 * Error khusus dialog konfirmasi hapus artikel. Dipisah dari `errorMsg`
 * (banner halaman) karena banner ada di belakang overlay dialog — kegagalan
 * hapus jadi tidak terlihat sampai dialog ditutup manual.
 */
const modalError = ref<string | null>(null)

async function fetchList() {
  loading.value = true
  errorMsg.value = null
  try {
    const res = await cms.listAdmin({
      page: page.value,
      limit: limit.value,
      status: statusFilter.value || undefined,
      q: search.value.trim() || undefined,
    })
    items.value = res.items
    total.value = res.total
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal memuat data'
  } finally {
    loading.value = false
  }
}

onMounted(fetchList)
watch([page, statusFilter], fetchList)

let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(search, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    fetchList()
  }, 300)
})

const columns: DataTableColumn[] = [
  { key: 'title', label: 'Judul' },
  { key: 'slug', label: 'Slug', class: 'hidden lg:table-cell text-ink-500' },
  { key: 'status', label: 'Status', class: 'w-28' },
  { key: 'seo_score', label: 'Skor SEO', class: 'w-24 hidden md:table-cell' },
  { key: 'updated_at', label: 'Diupdate', class: 'w-40 hidden md:table-cell' },
  { key: 'actions', label: '', class: 'w-32 text-right' },
]

// Badge skor SEO — palet semantic sama seperti `StatusBadge.vue` (emerald/amber/brand).
function scoreBadgeClass(score: number): string {
  if (score >= 80) return 'bg-emerald-50 text-emerald-800 ring-emerald-600/20'
  if (score >= 50) return 'bg-amber-50 text-amber-800 ring-amber-600/20'
  return 'bg-brand-50 text-brand-700 ring-brand-600/20'
}

function fmtDate(s: string): string {
  try {
    return new Date(s).toLocaleString('id-ID', {
      day: '2-digit', month: 'short', year: 'numeric',
      hour: '2-digit', minute: '2-digit',
    })
  } catch {
    return s
  }
}

// --- Actions ---
const busyID = ref<string | null>(null)
const confirmOpen = ref(false)
const pendingDelete = ref<Article | null>(null)

async function onPublish(a: Article) {
  busyID.value = a.id
  try {
    await cms.publish(a.id)
    await fetchList()
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal publish'
  } finally {
    busyID.value = null
  }
}
async function onArchive(a: Article) {
  busyID.value = a.id
  try {
    await cms.archive(a.id)
    await fetchList()
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal archive'
  } finally {
    busyID.value = null
  }
}
function askDelete(a: Article) {
  pendingDelete.value = a
  modalError.value = null
  confirmOpen.value = true
}
async function confirmDelete() {
  const a = pendingDelete.value
  if (!a) return
  busyID.value = a.id
  modalError.value = null
  try {
    await cms.remove(a.id)
    confirmOpen.value = false
    pendingDelete.value = null
    await fetchList()
  } catch (e: unknown) {
    modalError.value = e instanceof ApiError ? e.message : 'Gagal hapus'
  } finally {
    busyID.value = null
  }
}
</script>

<template>
  <section>
    <AdminPageHeader
      title="Artikel"
      subtitle="Kelola konten artikel SEO. Gambar upload otomatis dikonversi ke WebP."
    >
      <template #actions>
        <NuxtLink
          v-if="canCreate"
          to="/admin/artikel/new"
          class="inline-flex items-center rounded-md bg-brand-500 px-3 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt transition-colors"
        >
          + Artikel Baru
        </NuxtLink>
      </template>
    </AdminPageHeader>

    <div class="mb-4 flex flex-wrap items-center gap-2">
      <div class="inline-flex rounded-md border border-hairline overflow-hidden text-sm">
        <button
          v-for="opt in [
            { v: '', label: 'Semua' },
            { v: 'draft', label: 'Draft' },
            { v: 'published', label: 'Published' },
            { v: 'archived', label: 'Archived' },
          ]"
          :key="opt.v"
          type="button"
          :class="[
            'px-3 py-1.5 border-r border-hairline last:border-r-0 transition-colors',
            statusFilter === opt.v ? 'bg-ink-950 text-canvas' : 'bg-canvas text-ink-700 hover:bg-canvas-alt',
          ]"
          @click="statusFilter = opt.v as ArticleStatus | ''; page = 1"
        >
          {{ opt.label }}
        </button>
      </div>
      <input
        v-model="search"
        type="search"
        placeholder="Cari judul…"
        class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
      >
    </div>

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />

    <AdminDataTable
      :columns="columns"
      :items="items"
      :loading="loading"
      row-key="id"
      empty-message="Belum ada artikel. Klik “Artikel Baru” untuk mulai."
    >
      <template #cell-title="{ row }">
        <NuxtLink
          :to="`/admin/artikel/${(row as Article).id}`"
          class="font-medium text-ink-900 hover:text-brand-500 transition-colors"
        >
          {{ (row as Article).title }}
        </NuxtLink>
      </template>
      <template #cell-slug="{ row }">
        <span class="font-mono text-xs">{{ (row as Article).slug }}</span>
      </template>
      <template #cell-status="{ row }">
        <AdminStatusBadge :status="(row as Article).status" />
      </template>
      <template #cell-seo_score="{ row }">
        <span
          v-if="(row as Article).seo_score != null"
          class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset"
          :class="scoreBadgeClass((row as Article).seo_score!)"
        >{{ (row as Article).seo_score }}</span>
        <span v-else class="text-xs text-ink-400">—</span>
      </template>
      <template #cell-updated_at="{ row }">
        <span class="text-xs text-ink-500">{{ fmtDate((row as Article).updated_at) }}</span>
      </template>
      <template #cell-actions="{ row }">
        <div class="flex justify-end items-center gap-1">
          <button
            v-if="canPublish && (row as Article).status !== 'published'"
            type="button"
            class="inline-flex items-center rounded-md border border-emerald-300 bg-canvas px-2 py-1 text-xs font-medium text-emerald-800 hover:bg-emerald-50 transition-colors disabled:opacity-40"
            :disabled="busyID === (row as Article).id"
            @click="onPublish(row as Article)"
          >
            Publish
          </button>
          <button
            v-if="canPublish && (row as Article).status === 'published'"
            type="button"
            class="inline-flex items-center rounded-md border border-hairline bg-canvas px-2 py-1 text-xs text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-40"
            :disabled="busyID === (row as Article).id"
            @click="onArchive(row as Article)"
          >
            Archive
          </button>
          <button
            v-if="canPublish"
            type="button"
            class="inline-flex items-center rounded-md border border-brand-200 bg-canvas px-2 py-1 text-xs font-medium text-brand-700 hover:bg-brand-50 hover:border-brand-300 transition-colors disabled:opacity-40"
            :disabled="busyID === (row as Article).id"
            @click="askDelete(row as Article)"
          >
            Hapus
          </button>
        </div>
      </template>
    </AdminDataTable>

    <AdminPagination
      v-if="total > 0"
      :page="page"
      :limit="limit"
      :total="total"
      @update:page="(v) => (page = v)"
    />

    <AdminConfirmDialog
      v-model:open="confirmOpen"
      :title="`Hapus artikel “${pendingDelete?.title || ''}”?`"
      message="Aksi ini permanen dan tidak bisa di-undo. Konten & gambar cover tidak dikembalikan."
      confirm-label="Ya, hapus"
      variant="danger"
      :loading="busyID !== null"
      :error="modalError"
      @confirm="confirmDelete"
      @cancel="pendingDelete = null"
    />
  </section>
</template>
