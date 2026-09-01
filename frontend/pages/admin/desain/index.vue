<script setup lang="ts">
/**
 * /admin/desain — worklist untuk staff desain (§6).
 *
 * 3 tab kerja:
 *   1. Verifikasi Upload — customer submit file siap-cetak, tunggu approval.
 *      (design_source=upload, status=dibayar, ada customer_upload file)
 *   2. Kerjakan Request — customer minta jasa desain, staff perlu kerjakan.
 *      (design_source=request, status=dibayar OR desain_dikerjakan)
 *   3. Menunggu Approval Customer — draft sudah upload, tunggu customer.
 *      (status=menunggu_approval_desain)
 *
 * Backend /admin/orders tidak filter design_source, jadi filter client-side
 * setelah fetch dengan status filter. Volume order-per-status kecil, aman.
 */
import { orderPrimaryProductLabel, type Order } from '~/types/order'
import type { DataTableColumn } from '~/components/admin/DataTable.vue'
import { ApiError } from '~/composables/useApi'
import { FileCheck2, Palette, Hourglass } from '@lucide/vue'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Desain — Admin' })

const orderSvc = useOrder()

interface Stage {
  key: 'verify' | 'work' | 'wait'
  label: string
  icon: typeof FileCheck2
  hint: string
  /** Backend status(es) untuk fetch. */
  statuses: Array<'dibayar' | 'desain_dikerjakan' | 'menunggu_approval_desain'>
  /**
   * Filter client-side by order.design_source (§32.5.2 — bisa 'mixed' di
   * order banyak-item yang barisnya campur upload+request). Array kosong =
   * tidak filter (semua design_source tampil).
   */
  designSources: Array<'upload' | 'request' | 'mixed'>
}

const stages: Stage[] = [
  {
    key: 'verify',
    label: 'Verifikasi Upload',
    icon: FileCheck2,
    hint: 'Customer sudah upload file siap-cetak. Cek isi file & tandai verified untuk lanjut proses cetak.',
    statuses: ['dibayar'],
    // Bukan 'mixed' — StaffVerifyUpload (aksi seluruh order, §32.6) menolak
    // kalau ADA SATU SAJA item yang design_source-nya 'request'.
    designSources: ['upload'],
  },
  {
    key: 'work',
    label: 'Kerjakan Request',
    icon: Palette,
    hint: 'Customer minta jasa desain. Cek brief + aset lalu upload draft per baris untuk approval.',
    statuses: ['dibayar', 'desain_dikerjakan'],
    // 'mixed' IKUT di sini — order campuran tetap butuh staff mengerjakan
    // baris 'request'-nya, walau sebagian barisnya sudah 'upload' siap cetak.
    designSources: ['request', 'mixed'],
  },
  {
    key: 'wait',
    label: 'Menunggu Approval Customer',
    icon: Hourglass,
    hint: 'Draft sudah dikirim ke customer. Follow up manual kalau lama tidak ada respon.',
    statuses: ['menunggu_approval_desain'],
    designSources: [],
  },
]

const active = ref<Stage>(stages[0])
const items = ref<Order[]>([])
const loading = ref(false)
const errorMsg = ref<string | null>(null)

async function fetchList() {
  loading.value = true
  errorMsg.value = null
  try {
    // Fetch tiap status paralel, gabung, dedupe by resi.
    const all = await Promise.all(
      active.value.statuses.map((s) => orderSvc.listAdmin({ status: s, pageSize: 100 })),
    )
    const combined = all.flatMap((p) => p.items)

    // Filter design_source client-side kalau stage specify.
    const sources = active.value.designSources
    const filtered = sources.length
      ? combined.filter((o) => sources.includes(o.design_source as 'upload' | 'request' | 'mixed'))
      : combined

    // Dedupe by id (paranoia — shouldn't happen since status unik per order).
    const map = new Map<string, Order>()
    for (const o of filtered) map.set(o.id, o)
    items.value = [...map.values()]
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal memuat data'
  } finally {
    loading.value = false
  }
}

/**
 * Ringkasan design_source yang jujur untuk order campuran (§32.5.2) — jangan
 * tampilkan satu label 'upload'/'request' untuk order 'mixed', itu
 * menyembunyikan bahwa ada baris lain yang butuh alur berbeda.
 */
function designSourceSummary(o: Order): string {
  if (o.design_source !== 'mixed') return o.design_source
  const uploadCount = o.items.filter((it) => it.design_source === 'upload').length
  const requestCount = o.items.filter((it) => it.design_source === 'request').length
  return `${uploadCount} upload · ${requestCount} request`
}

onMounted(fetchList)
watch(active, fetchList)

const columns: DataTableColumn[] = [
  { key: 'created_at', label: 'Masuk', class: 'w-32 hidden md:table-cell' },
  { key: 'resi', label: 'Resi', class: 'w-36' },
  { key: 'product', label: 'Produk' },
  { key: 'design_source', label: 'Tipe', class: 'w-28 hidden lg:table-cell' },
  { key: 'action', label: '', class: 'w-32 text-right' },
]

function fmtDate(s: string): string {
  try {
    return new Date(s).toLocaleString('id-ID', {
      day: '2-digit', month: 'short',
      hour: '2-digit', minute: '2-digit',
    })
  } catch {
    return s
  }
}
</script>

<template>
  <section>
    <AdminPageHeader
      title="Desain"
      subtitle="Verifikasi upload customer atau kerjakan request desain sampai approval."
    />

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />

    <!-- Stage tabs -->
    <div class="mb-5 grid grid-cols-1 sm:grid-cols-3 gap-2">
      <button
        v-for="s in stages"
        :key="s.key"
        type="button"
        :class="[
          'flex items-center gap-2 rounded-md border px-3 py-2 text-xs font-medium text-left transition-colors',
          active.key === s.key
            ? 'border-brand-500 bg-brand-50 text-brand-800'
            : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300 hover:text-ink-950',
        ]"
        @click="active = s"
      >
        <component :is="s.icon" class="h-4 w-4 shrink-0" :stroke-width="1.75" />
        <span class="min-w-0 truncate">{{ s.label }}</span>
      </button>
    </div>

    <!-- Hint -->
    <div class="mb-5 rounded-lg border border-hairline bg-canvas p-4 flex items-start gap-3">
      <component :is="active.icon" class="h-4 w-4 mt-0.5 text-ink-500 shrink-0" :stroke-width="1.75" />
      <div>
        <p class="text-sm font-medium text-ink-900">{{ active.label }}</p>
        <p class="text-xs text-ink-500 mt-0.5 leading-relaxed">{{ active.hint }}</p>
      </div>
      <span class="ml-auto text-xs text-ink-500 shrink-0">{{ items.length }} order</span>
    </div>

    <AdminDataTable
      :columns="columns"
      :items="items"
      :loading="loading"
      row-key="id"
      empty-message="Tidak ada order pada tahap ini."
    >
      <template #cell-created_at="{ row }">
        <span class="text-xs text-ink-500">{{ fmtDate((row as Order).created_at) }}</span>
      </template>
      <template #cell-resi="{ row }">
        <NuxtLink
          :to="`/admin/desain/${(row as Order).resi}`"
          class="font-mono text-xs text-ink-900 hover:text-brand-500 transition-colors"
        >
          {{ (row as Order).resi }}
        </NuxtLink>
      </template>
      <template #cell-product="{ row }">
        <p class="text-sm text-ink-900">{{ orderPrimaryProductLabel(row as Order) }}</p>
        <p v-if="(row as Order).items[0]" class="text-xs text-ink-500">
          {{ (row as Order).items[0].width_cm }}×{{ (row as Order).items[0].height_cm }}cm
          · {{ (row as Order).items[0].material_name }} · {{ (row as Order).items[0].quantity }} pcs
        </p>
      </template>
      <template #cell-design_source="{ row }">
        <span class="text-xs uppercase text-ink-600">{{ designSourceSummary(row as Order) }}</span>
      </template>
      <template #cell-action="{ row }">
        <NuxtLink
          :to="`/admin/desain/${(row as Order).resi}`"
          class="inline-flex items-center gap-1.5 rounded-md bg-ink-950 px-3 py-1.5 text-xs font-semibold text-canvas hover:bg-ink-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors"
        >
          Buka ›
        </NuxtLink>
      </template>
    </AdminDataTable>
  </section>
</template>
