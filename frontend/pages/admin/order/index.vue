<script setup lang="ts">
import { orderPrimaryProductLabel, type Order, type OrderChannel, type OrderStatus } from '~/types/order'
import type { DataTableColumn } from '~/components/admin/DataTable.vue'
import { ApiError } from '~/composables/useApi'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Order — Admin' })

const orderSvc = useOrder()

const status = ref<OrderStatus | ''>('')
const channel = ref<OrderChannel | ''>('')
const search = ref('')
const page = ref(1)
const pageSize = 10

const items = ref<Order[]>([])
const total = ref(0)
const loading = ref(false)
const errorMsg = ref<string | null>(null)

const statusTabs: Array<{ v: OrderStatus | ''; label: string }> = [
  { v: '', label: 'Semua' },
  { v: 'menunggu_ongkir', label: 'Menunggu Ongkir' },
  { v: 'menunggu_pembayaran', label: 'Menunggu Pembayaran' },
  { v: 'menunggu_verifikasi', label: 'Menunggu Verifikasi' },
  { v: 'dibayar', label: 'Dibayar' },
  { v: 'proses_cetak', label: 'Proses Cetak' },
  { v: 'dikirim', label: 'Dikirim' },
  { v: 'selesai', label: 'Selesai' },
  { v: 'dibatalkan', label: 'Dibatalkan' },
]

async function fetchList() {
  loading.value = true
  errorMsg.value = null
  try {
    const res = await orderSvc.listAdmin({
      status: status.value || undefined,
      channel: channel.value || undefined,
      page: page.value,
      pageSize,
    })
    // Backend list = paginated di server; search resi client-side (satu halaman)
    // supaya tidak butuh backend re-work.
    const q = search.value.trim().toLowerCase()
    items.value = q
      ? res.items.filter((o) => o.resi.toLowerCase().includes(q))
      : res.items
    total.value = res.total
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal memuat data'
  } finally {
    loading.value = false
  }
}

onMounted(fetchList)
watch([status, channel, page], fetchList)

let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(search, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(fetchList, 250)
})

const columns: DataTableColumn[] = [
  { key: 'created_at', label: 'Masuk', class: 'w-36 hidden md:table-cell' },
  { key: 'resi', label: 'Resi', class: 'w-40' },
  { key: 'product', label: 'Produk' },
  { key: 'total', label: 'Total', class: 'w-32 text-right' },
  { key: 'metode_ambil', label: 'Ambil', class: 'w-24 hidden lg:table-cell' },
  { key: 'channel', label: 'Channel', class: 'w-20 hidden lg:table-cell' },
  { key: 'status', label: 'Status', class: 'w-40' },
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
function fmtIDR(n: number | null | undefined): string {
  if (n == null) return '—'
  return new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(n)
}
function statusLabel(s: string): string {
  return s.replace(/_/g, ' ')
}
</script>

<template>
  <section>
    <AdminPageHeader
      title="Order"
      subtitle="Kelola pesanan customer. Set ongkir, cancel, atau lanjut ke pembayaran/desain/produksi."
    />

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />

    <!-- Filter bar -->
    <div class="mb-5 flex flex-col sm:flex-row sm:flex-wrap sm:items-center gap-3">
      <div class="flex flex-wrap rounded-md border border-hairline bg-canvas overflow-hidden text-sm">
        <button
          v-for="opt in statusTabs"
          :key="opt.v"
          type="button"
          :class="[
            'px-3 py-1.5 border-r border-hairline last:border-r-0 transition-colors',
            status === opt.v
              ? 'bg-canvas-alt font-medium text-ink-950'
              : 'text-ink-600 hover:bg-canvas-alt/60',
          ]"
          @click="status = opt.v; page = 1"
        >
          {{ opt.label }}
        </button>
      </div>

      <div class="flex items-center gap-2 flex-1 sm:flex-none">
        <select
          v-model="channel"
          class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          @change="page = 1"
        >
          <option value="">Semua channel</option>
          <option value="online">Online</option>
          <option value="pos">POS</option>
        </select>

        <input
          v-model="search"
          type="search"
          placeholder="Cari resi…"
          class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
        >
      </div>
    </div>

    <AdminDataTable
      :columns="columns"
      :items="items"
      :loading="loading"
      row-key="id"
      empty-message="Tidak ada order pada filter ini."
    >
      <template #cell-created_at="{ row }">
        <span class="text-xs text-ink-500">{{ fmtDate((row as Order).created_at) }}</span>
      </template>
      <template #cell-resi="{ row }">
        <NuxtLink
          :to="`/admin/order/${(row as Order).resi}`"
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
      <template #cell-total="{ row }">
        <span class="text-sm font-medium text-ink-900">{{ fmtIDR((row as Order).total) }}</span>
      </template>
      <template #cell-metode_ambil="{ row }">
        <span class="text-xs uppercase text-ink-600">{{ (row as Order).metode_ambil }}</span>
      </template>
      <template #cell-channel="{ row }">
        <span class="text-xs uppercase text-ink-600">{{ (row as Order).channel }}</span>
      </template>
      <template #cell-status="{ row }">
        <AdminStatusBadge :status="statusLabel((row as Order).status)" />
      </template>
    </AdminDataTable>

    <AdminPagination
      v-if="total > 0"
      :page="page"
      :limit="pageSize"
      :total="total"
      @update:page="(v) => (page = v)"
    />
  </section>
</template>
