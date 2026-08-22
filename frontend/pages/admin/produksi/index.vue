<script setup lang="ts">
import type { Order, OrderStatus } from '~/types/order'
import type { DataTableColumn } from '~/components/admin/DataTable.vue'
import { ApiError } from '~/composables/useApi'
import { Printer, CheckCircle2, PackageCheck, Truck, Flag } from '@lucide/vue'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Produksi — Admin' })

const auth = useAuthStore()
const orderSvc = useOrder()
const productionSvc = useProduction()

const canAdvance = computed(() => auth.hasPermission('production.update'))

/**
 * Production workflow tabs — 5 tahap dari desain siap → selesai.
 * Tiap tahap = filter status di endpoint /admin/orders, plus 1 action button
 * yang advance ke stage berikutnya via /production/*.
 *
 * Catatan: `siap_kirim` & `siap_ambil` dipisah kolom — meski keduanya hasil
 * dari mark-siap, action lanjutannya beda (siap_kirim → mark-shipped butuh
 * courier+tracking, siap_ambil langsung ke mark-selesai saat customer datang).
 */
interface Stage {
  value: OrderStatus
  label: string
  actionLabel: string
  icon: typeof Printer
  hint: string
}

const stages: Stage[] = [
  {
    value: 'desain_diverifikasi',
    label: 'Siap Cetak',
    actionLabel: 'Mulai Cetak',
    icon: Printer,
    hint: 'Desain sudah diverifikasi. Klik untuk memulai proses cetak.',
  },
  {
    value: 'proses_cetak',
    label: 'Sedang Cetak',
    actionLabel: 'Kirim ke QC',
    icon: CheckCircle2,
    hint: 'Cetak selesai. Kirim ke tahap QC.',
  },
  {
    value: 'qc',
    label: 'QC',
    actionLabel: 'Mark Siap',
    icon: PackageCheck,
    hint: 'QC selesai — tandai order siap (kirim/ambil sesuai metode).',
  },
  {
    value: 'siap_kirim',
    label: 'Siap Kirim',
    actionLabel: 'Serahkan Kurir',
    icon: Truck,
    hint: 'Order siap dikirim. Serahkan ke kurir & input tracking.',
  },
  {
    value: 'siap_ambil',
    label: 'Siap Ambil',
    actionLabel: 'Mark Selesai',
    icon: Flag,
    hint: 'Order menunggu pengambilan customer. Tandai selesai saat customer datang.',
  },
  {
    value: 'dikirim',
    label: 'Dalam Perjalanan',
    actionLabel: 'Mark Selesai',
    icon: Flag,
    hint: 'Order sedang dikirim. Tandai selesai saat customer konfirmasi diterima.',
  },
]

const activeStage = ref<Stage>(stages[1]) // default: Sedang Cetak
const items = ref<Order[]>([])
const total = ref(0)
const loading = ref(false)
const errorMsg = ref<string | null>(null)
const successMsg = ref<string | null>(null)
const busyResi = ref<string | null>(null)

async function fetchList() {
  loading.value = true
  errorMsg.value = null
  try {
    const res = await orderSvc.listAdmin({
      status: activeStage.value.value,
      page: 1,
      pageSize: 50,
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
watch(activeStage, fetchList)

// --- Advance actions ---
async function advance(order: Order) {
  if (!canAdvance.value) return
  const stage = activeStage.value
  busyResi.value = order.resi
  errorMsg.value = null
  try {
    switch (stage.value) {
      case 'desain_diverifikasi':
        await productionSvc.startCetak(order.resi)
        break
      case 'proses_cetak':
        await productionSvc.startQC(order.resi)
        break
      case 'qc':
        await productionSvc.markSiap(order.resi)
        break
      case 'siap_kirim':
        // Butuh input courier+tracking → buka modal instead of instant advance.
        openShippedModal(order)
        return
      case 'siap_ambil':
      case 'dikirim':
        await productionSvc.markSelesai(order.resi)
        break
    }
    successMsg.value = `${order.resi} → ${stage.actionLabel} berhasil.`
    setTimeout(() => (successMsg.value = null), 2500)
    await fetchList()
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal advance status'
  } finally {
    busyResi.value = null
  }
}

// --- Shipped modal (courier + tracking) ---
const shippedOrder = ref<Order | null>(null)
const shippedCourier = ref('')
const shippedTracking = ref('')
const shippedNote = ref('')
const shippedBusy = ref(false)

/**
 * Error milik modal "Serahkan ke kurir". Dipisah dari `errorMsg` (banner
 * halaman) karena banner ada di belakang overlay modal — kegagalan submit
 * jadi tidak terlihat sampai modal ditutup manual.
 */
const modalError = ref<string | null>(null)

function openShippedModal(order: Order) {
  shippedOrder.value = order
  shippedCourier.value = ''
  shippedTracking.value = ''
  shippedNote.value = ''
  modalError.value = null
}

async function submitShipped() {
  const o = shippedOrder.value
  if (!o) return
  if (!shippedCourier.value.trim() || !shippedTracking.value.trim()) {
    modalError.value = 'Courier & tracking wajib diisi.'
    return
  }
  shippedBusy.value = true
  modalError.value = null
  try {
    await productionSvc.markShipped(o.resi, {
      courier: shippedCourier.value.trim(),
      tracking_number: shippedTracking.value.trim(),
      note: shippedNote.value.trim() || undefined,
    })
    successMsg.value = `${o.resi} diserahkan ke ${shippedCourier.value.trim()} · ${shippedTracking.value.trim()}.`
    setTimeout(() => (successMsg.value = null), 3000)
    shippedOrder.value = null
    await fetchList()
  } catch (e: unknown) {
    modalError.value = e instanceof ApiError ? e.message : 'Gagal mark shipped'
  } finally {
    shippedBusy.value = false
  }
}

// --- Columns ---
const columns: DataTableColumn[] = [
  { key: 'created_at', label: 'Masuk', class: 'w-32 hidden md:table-cell' },
  { key: 'resi', label: 'Resi', class: 'w-36' },
  { key: 'product', label: 'Produk' },
  { key: 'metode_ambil', label: 'Ambil', class: 'w-24 hidden lg:table-cell' },
  { key: 'action', label: '', class: 'w-40 text-right' },
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
      title="Produksi"
      subtitle="Pipeline cetak → QC → kirim/ambil → selesai. Pilih tahap untuk lihat order yang siap advance."
    />

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />
    <AlertMessage v-if="successMsg" variant="success" :message="successMsg" class="mb-4" />

    <!-- Stage tabs -->
    <div class="mb-5 grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-2">
      <button
        v-for="s in stages"
        :key="s.value"
        type="button"
        :class="[
          'flex items-center gap-2 rounded-md border px-3 py-2 text-xs font-medium text-left transition-colors',
          activeStage.value === s.value
            ? 'border-brand-500 bg-brand-50 text-brand-800'
            : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300 hover:text-ink-950',
        ]"
        @click="activeStage = s"
      >
        <component :is="s.icon" class="h-4 w-4 shrink-0" :stroke-width="1.75" />
        <span class="min-w-0 truncate">{{ s.label }}</span>
      </button>
    </div>

    <!-- Hint -->
    <div class="mb-5 rounded-lg border border-hairline bg-canvas p-4 flex items-start gap-3">
      <component :is="activeStage.icon" class="h-4 w-4 mt-0.5 text-ink-500 shrink-0" :stroke-width="1.75" />
      <div>
        <p class="text-sm font-medium text-ink-900">{{ activeStage.label }}</p>
        <p class="text-xs text-ink-500 mt-0.5 leading-relaxed">{{ activeStage.hint }}</p>
      </div>
      <span class="ml-auto text-xs text-ink-500 shrink-0">{{ total }} order</span>
    </div>

    <div v-if="!canAdvance" class="mb-4 rounded-md border border-gold-200 bg-gold-50 p-3 text-xs text-gold-900">
      Kamu tidak punya permission <code class="font-mono">production.update</code> — tombol advance disabled.
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
          :to="`/admin/order/${(row as Order).resi}`"
          class="font-mono text-xs text-ink-900 hover:text-brand-500 transition-colors"
        >
          {{ (row as Order).resi }}
        </NuxtLink>
      </template>
      <template #cell-product="{ row }">
        <p class="text-sm text-ink-900">{{ (row as Order).product_name }}</p>
        <p class="text-xs text-ink-500">
          {{ (row as Order).width_cm }}×{{ (row as Order).height_cm }}cm · {{ (row as Order).material_name }} · {{ (row as Order).quantity }} pcs
        </p>
      </template>
      <template #cell-metode_ambil="{ row }">
        <span class="text-xs uppercase text-ink-600">{{ (row as Order).metode_ambil }}</span>
      </template>
      <template #cell-action="{ row }">
        <button
          type="button"
          :disabled="!canAdvance || busyResi === (row as Order).resi"
          class="inline-flex items-center gap-1.5 rounded-md bg-ink-950 px-3 py-1.5 text-xs font-semibold text-canvas hover:bg-ink-900 disabled:opacity-50 disabled:cursor-not-allowed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors"
          @click="advance(row as Order)"
        >
          <span
            v-if="busyResi === (row as Order).resi"
            class="inline-block h-3 w-3 rounded-full border-2 border-canvas/70 border-t-transparent animate-spin"
            aria-hidden="true"
          />
          {{ activeStage.actionLabel }} ›
        </button>
      </template>
    </AdminDataTable>

    <!-- Shipped modal (courier + tracking) -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="shippedOrder" class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/50" aria-hidden="true" @click="!shippedBusy && (shippedOrder = null)" />
          <div role="dialog" aria-modal="true" class="relative w-full max-w-md rounded-lg bg-canvas shadow-xl ring-1 ring-black/5 p-5">
            <h3 class="font-serif text-lg font-semibold text-ink-950">
              Serahkan {{ shippedOrder.resi }} ke kurir
            </h3>
            <p class="mt-1 text-xs text-ink-500 leading-relaxed">
              Input nama ekspedisi & nomor resi kurir. Customer dinotifikasi via WA.
            </p>
            <AlertMessage v-if="modalError" variant="error" :message="modalError" class="mt-3" />
            <div class="mt-4 space-y-3">
              <div>
                <label for="courier" class="block text-xs font-medium text-ink-700">Ekspedisi</label>
                <input
                  id="courier"
                  v-model="shippedCourier"
                  type="text"
                  placeholder="JNE / SiCepat / J&T / Grab / …"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                >
              </div>
              <div>
                <label for="tracking" class="block text-xs font-medium text-ink-700">Nomor resi kurir</label>
                <input
                  id="tracking"
                  v-model="shippedTracking"
                  type="text"
                  placeholder="Contoh: JNE1234567890"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors font-mono"
                >
              </div>
              <div>
                <label for="note" class="block text-xs font-medium text-ink-700">Catatan (opsional)</label>
                <input
                  id="note"
                  v-model="shippedNote"
                  type="text"
                  maxlength="500"
                  placeholder="Estimasi 2-3 hari…"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                >
              </div>
            </div>
            <div class="mt-5 flex justify-end gap-2">
              <button
                type="button"
                class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm text-ink-700 hover:bg-canvas-alt disabled:opacity-50 transition-colors"
                :disabled="shippedBusy"
                @click="shippedOrder = null"
              >
                Batal
              </button>
              <button
                type="button"
                class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 disabled:opacity-60 transition-colors"
                :disabled="shippedBusy || !shippedCourier.trim() || !shippedTracking.trim()"
                @click="submitShipped"
              >
                <span
                  v-if="shippedBusy"
                  class="inline-block h-3 w-3 rounded-full border-2 border-canvas/70 border-t-transparent animate-spin"
                  aria-hidden="true"
                />
                Serahkan &amp; Notifikasi
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </section>
</template>

<style scoped>
.fade-enter-active, .fade-leave-active { transition: opacity 0.15s ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
</style>
