<script setup lang="ts">
/**
 * /admin/pengaturan — Setting global aplikasi (§19).
 *
 * Isi saat ini: retensi file desain (hari), lebar kertas struk POS (§12),
 * saklar fitur membership (§30.1), dan rekening & QRIS pembayaran (§7). Nilai
 * retensi dibaca job retention tiap kali jalan; lebar struk dibaca
 * `/admin/pos/index.vue` untuk `@page` dinamis; saklar membership dibaca
 * service `discount`/`membership` saat validasi & oleh halaman akun customer
 * untuk sembunyikan/tampilkan "Ajukan jadi Member"; nilai rekening/QRIS
 * dibaca publik lewat `GET /payment-info` (lihat `usePaymentInfo.ts`) — jadi
 * perubahan di sini langsung tampil ke pembeli, tanpa deploy ulang.
 *
 * Sengaja tidak generic-form: tiap setting punya konteks & konsekuensi sendiri
 * yang perlu dijelaskan ke admin (menurunkan retensi = file lama langsung
 * terhapus di sweep berikutnya; salah ketik nomor rekening = uang pembeli
 * salah alamat). Setting baru nanti ditambah sebagai card sendiri, bukan
 * diseragamkan jadi tabel key/value telanjang.
 *
 * Design: patuh CLAUDE.md §26 (Fraunces + Inter + Lucide, brand/ink/hairline).
 */
import { HardDrive, Landmark, Printer, QrCode, Save, RotateCcw, Loader2, TriangleAlert, UserCheck } from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import {
  SETTING_DESIGN_RETENTION_DAYS,
  SETTING_POS_RECEIPT_WIDTH_MM,
  SETTING_MEMBERSHIP_ENABLED,
  PAYMENT_SETTING_KEYS,
  SETTING_PAYMENT_BANK_NAME,
  SETTING_PAYMENT_ACCOUNT_NAME,
  SETTING_PAYMENT_ACCOUNT_NUMBER,
  SETTING_PAYMENT_QRIS_NOTE,
  SETTING_PAYMENT_QRIS_MERCHANT_NAME,
  SETTING_PAYMENT_QRIS_NMID,
  type AppSetting,
} from '~/composables/useAdminSettings'

/**
 * Cadangan (BUKAN sumber kebenaran) kalau `allowed_values` tidak ada di
 * response — mis. backend lama yang belum kirim field ini. Sumber kebenaran
 * sesungguhnya adalah `receiptWidth.allowed_values` dari `GET
 * /admin/settings` (kontrak §9/settings/model/setting_item.go), dibaca lewat
 * computed `receiptWidthOptions` di bawah.
 */
const FALLBACK_RECEIPT_WIDTH_OPTIONS = ['58', '80']

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Pengaturan — Rajaku Admin', robots: 'noindex,nofollow' })

const auth = useAuthStore()
const settingsApi = useAdminSettings()

const canManage = computed(() => auth.hasPermission('settings.manage'))

const settings = ref<AppSetting[]>([])
const loading = ref(false)
const saving = ref(false)
const errorMsg = ref('')
const successMsg = ref('')

/**
 * Nilai yang sedang diedit, per key.
 *
 * Tipe `string | number` bukan kelalaian: Vue otomatis menerapkan modifier
 * `.number` pada `<input type="number">`, jadi v-model mengisi angka walaupun
 * nilai awalnya string dari API. Semua pengiriman ke backend lewat
 * `toValue()` supaya body JSON-nya tetap string (backend bind `value string`).
 */
const draft = reactive<Record<string, string | number>>({})

function toValue(key: string): string {
  const v = draft[key]
  return v === undefined || v === null ? '' : String(v)
}

const retention = computed(() =>
  settings.value.find((s) => s.key === SETTING_DESIGN_RETENTION_DAYS),
)
const retentionDirty = computed(
  () =>
    !!retention.value &&
    toValue(SETTING_DESIGN_RETENTION_DAYS) !== retention.value.value,
)

// -------------------- Kasir & Struk (§12) --------------------
const receiptWidth = computed(() =>
  settings.value.find((s) => s.key === SETTING_POS_RECEIPT_WIDTH_MM),
)
const receiptWidthDirty = computed(
  () =>
    !!receiptWidth.value &&
    toValue(SETTING_POS_RECEIPT_WIDTH_MM) !== receiptWidth.value.value,
)
/** Sumber kebenaran: `allowed_values` dari backend. Fallback hanya dipakai kalau field itu kosong. */
const receiptWidthOptions = computed(
  () => receiptWidth.value?.allowed_values ?? FALLBACK_RECEIPT_WIDTH_OPTIONS,
)

// -------------------- Membership (§30.1) --------------------
const membership = computed(() =>
  settings.value.find((s) => s.key === SETTING_MEMBERSHIP_ENABLED),
)
const membershipDirty = computed(
  () =>
    !!membership.value &&
    toValue(SETTING_MEMBERSHIP_ENABLED) !== membership.value.value,
)
/** Toggle switch butuh boolean; setting-nya tersimpan sebagai string "true"/"false" (§22 — bind body JSON tetap string). */
const membershipDraftEnabled = computed({
  get: () => toValue(SETTING_MEMBERSHIP_ENABLED) === 'true',
  set: (v: boolean) => {
    draft[SETTING_MEMBERSHIP_ENABLED] = v ? 'true' : 'false'
  },
})

// -------------------- Rekening & QRIS (§7) --------------------
function settingByKey(key: string): AppSetting | undefined {
  return settings.value.find((s) => s.key === key)
}
function isDirty(key: string): boolean {
  const s = settingByKey(key)
  return !!s && toValue(key) !== s.value
}

// Semua 6 key harus ada supaya card ditampilkan — kalau backend belum
// migrasi/seed setting ini, card disembunyikan daripada tampil separuh.
const paymentLoaded = computed(() =>
  PAYMENT_SETTING_KEYS.every((k) => !!settingByKey(k)),
)
const paymentDirtyKeys = computed(() => PAYMENT_SETTING_KEYS.filter((k) => isDirty(k)))
const paymentDirty = computed(() => paymentDirtyKeys.value.length > 0)

// Nomor rekening butuh konfirmasi eksplisit sebelum disimpan (salah ketik
// satu angka = uang pembeli salah alamat) — field lain dalam grup ini tidak.
const accountNumberDirty = computed(() => isDirty(SETTING_PAYMENT_ACCOUNT_NUMBER))
const accountNumberConfirmOpen = ref(false)
const paymentSaving = ref(false)

const accountNumberOldValue = computed(() => settingByKey(SETTING_PAYMENT_ACCOUNT_NUMBER)?.value ?? '')
const accountNumberNewValue = computed(() => toValue(SETTING_PAYMENT_ACCOUNT_NUMBER))
const bankNameNewValue = computed(() => toValue(SETTING_PAYMENT_BANK_NAME))

/** Diklik dari tombol "Simpan" grup rekening — gate lewat dialog kalau nomor rekening berubah. */
function requestSavePayment() {
  if (accountNumberDirty.value) {
    accountNumberConfirmOpen.value = true
    return
  }
  savePaymentFields()
}

/** Simpan semua key rekening/QRIS yang berubah, sekuensial supaya urutannya deterministik. */
async function savePaymentFields() {
  if (paymentSaving.value) return
  paymentSaving.value = true
  errorMsg.value = ''
  successMsg.value = ''
  try {
    for (const key of paymentDirtyKeys.value) {
      const updated = await settingsApi.update(key, toValue(key))
      settings.value = settings.value.map((s) => (s.key === key ? updated : s))
      draft[key] = updated.value
    }
    successMsg.value = 'Rekening & QRIS disimpan. Perubahan langsung tampil ke pembeli.'
  } catch (e) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal menyimpan pengaturan rekening'
  } finally {
    paymentSaving.value = false
    accountNumberConfirmOpen.value = false
  }
}

function resetPayment() {
  for (const key of PAYMENT_SETTING_KEYS) {
    const current = settingByKey(key)
    if (current) draft[key] = current.value
  }
}

async function fetchSettings() {
  loading.value = true
  errorMsg.value = ''
  try {
    const res = await settingsApi.list()
    settings.value = res.items
    for (const s of res.items) {
      draft[s.key] = s.value
    }
  } catch (e) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal memuat pengaturan'
  } finally {
    loading.value = false
  }
}

async function save(key: string) {
  if (saving.value) return
  saving.value = true
  errorMsg.value = ''
  successMsg.value = ''
  try {
    const updated = await settingsApi.update(key, toValue(key))
    settings.value = settings.value.map((s) => (s.key === key ? updated : s))
    draft[key] = updated.value
    successMsg.value = `${updated.display_name} disimpan.`
  } catch (e) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal menyimpan pengaturan'
  } finally {
    saving.value = false
  }
}

function reset(key: string) {
  const current = settings.value.find((s) => s.key === key)
  if (current) draft[key] = current.value
}

function formatDate(iso?: string) {
  if (!iso) return '—'
  return new Date(iso).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' })
}

onMounted(fetchSettings)
</script>

<template>
  <section>
    <AdminPageHeader
      title="Pengaturan"
      subtitle="Kebijakan global yang berlaku langsung tanpa deploy ulang. Hanya super admin yang bisa mengubah."
    />

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />
    <AlertMessage v-if="successMsg" variant="success" :message="successMsg" class="mb-4" />

    <div v-if="loading" class="rounded-lg border border-hairline bg-canvas p-8 text-center">
      <Loader2 class="mx-auto h-5 w-5 animate-spin text-ink-400" :stroke-width="1.5" />
      <p class="mt-2 text-xs text-ink-500">Memuat pengaturan…</p>
    </div>

    <div v-else class="space-y-6">
    <div v-if="retention" class="max-w-2xl rounded-lg border border-hairline bg-canvas p-6 md:p-8">
      <div class="flex items-start gap-3">
        <HardDrive class="mt-0.5 h-5 w-5 shrink-0 text-ink-600" :stroke-width="1.5" />
        <div>
          <h2 class="text-lg font-semibold text-ink-950">{{ retention.display_name }}</h2>
          <p class="mt-1 text-sm leading-relaxed text-ink-500">
            {{ retention.description }}
          </p>
        </div>
      </div>

      <div class="mt-6 flex flex-wrap items-end gap-3">
        <div>
          <label for="retention-days" class="block text-sm font-medium text-ink-900">
            Masa retensi
          </label>
          <div class="mt-1 flex items-center gap-2">
            <input
              id="retention-days"
              v-model="draft[SETTING_DESIGN_RETENTION_DAYS]"
              type="number"
              min="1"
              max="365"
              inputmode="numeric"
              :disabled="!canManage || saving"
              class="w-28 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 transition-colors focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20 disabled:cursor-not-allowed disabled:bg-canvas-alt disabled:text-ink-500"
            >
            <span class="text-sm text-ink-500">hari</span>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <button
            type="button"
            :disabled="!canManage || saving || !retentionDirty"
            class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt disabled:cursor-not-allowed disabled:opacity-50"
            @click="save(SETTING_DESIGN_RETENTION_DAYS)"
          >
            <Loader2 v-if="saving" class="h-4 w-4 animate-spin" :stroke-width="1.5" />
            <Save v-else class="h-4 w-4" :stroke-width="1.5" />
            Simpan
          </button>
          <button
            type="button"
            :disabled="saving || !retentionDirty"
            class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-medium text-ink-900 transition-colors hover:bg-canvas-alt disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
            @click="reset(SETTING_DESIGN_RETENTION_DAYS)"
          >
            <RotateCcw class="h-4 w-4" :stroke-width="1.5" />
            Batal
          </button>
        </div>
      </div>

      <!-- Konsekuensi yang gampang tidak disadari: menurunkan angka = file lama
           langsung masuk antrean hapus di sweep berikutnya. -->
      <div
        v-if="retentionDirty"
        class="mt-5 flex items-start gap-2 rounded-md bg-brand-50 p-3 text-xs leading-relaxed text-ink-700"
      >
        <TriangleAlert class="mt-0.5 h-4 w-4 shrink-0 text-brand-500" :stroke-width="1.5" />
        <p>
          Menurunkan masa retensi berlaku surut: file yang umurnya sudah melewati angka baru
          akan dihapus pada sweep berikutnya. Pastikan file yang masih dibutuhkan sudah diunduh.
        </p>
      </div>

      <dl class="mt-6 space-y-1 border-t border-hairline pt-4 text-xs text-ink-500">
        <div class="flex gap-2">
          <dt>Key</dt>
          <dd class="font-mono text-ink-700">{{ retention.key }}</dd>
        </div>
        <div class="flex gap-2">
          <dt>Terakhir diubah</dt>
          <dd>{{ formatDate(retention.updated_at) }}</dd>
        </div>
      </dl>

      <p v-if="!canManage" class="mt-4 text-xs text-ink-500">
        Anda tidak punya permission <span class="font-mono">settings.manage</span> — nilai hanya bisa dilihat.
      </p>
    </div>

    <!-- ============ Kasir & Struk (§12) ============ -->
    <div v-if="receiptWidth" class="max-w-2xl rounded-lg border border-hairline bg-canvas p-6 md:p-8">
      <div class="flex items-start gap-3">
        <Printer class="mt-0.5 h-5 w-5 shrink-0 text-ink-600" :stroke-width="1.5" />
        <div>
          <h2 class="text-lg font-semibold text-ink-950">Kasir & Struk</h2>
          <p class="mt-1 text-sm leading-relaxed text-ink-500">
            Lebar kertas thermal yang dipakai kasir. Menentukan ukuran halaman cetak (<span class="font-mono text-xs">@page</span>) struk POS.
          </p>
        </div>
      </div>

      <div class="mt-6">
        <p class="text-sm font-medium text-ink-900">Lebar kertas struk</p>
        <div class="mt-2 grid max-w-xs grid-cols-2 gap-2">
          <label
            v-for="w in receiptWidthOptions"
            :key="w"
            :class="[
              'flex cursor-pointer items-center justify-center gap-2 rounded-md border p-3 text-sm font-semibold transition-colors',
              toValue(SETTING_POS_RECEIPT_WIDTH_MM) === w
                ? 'border-brand-500 bg-brand-50/50 text-ink-950'
                : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
              !canManage || saving ? 'cursor-not-allowed opacity-60' : '',
            ]"
          >
            <input
              type="radio"
              name="pos-receipt-width"
              :value="w"
              :checked="toValue(SETTING_POS_RECEIPT_WIDTH_MM) === w"
              :disabled="!canManage || saving"
              class="accent-brand-500"
              @change="draft[SETTING_POS_RECEIPT_WIDTH_MM] = w"
            >
            {{ w }} mm
          </label>
        </div>
        <p class="mt-2 text-xs text-ink-500">
          Harus cocok dengan roll printer yang terpasang di kasir — salah pilih membuat struk
          kepotong (kertas lebih sempit dari yang dipilih) atau ter-scale kecil (kertas lebih lebar).
        </p>
      </div>

      <div class="mt-6 flex items-center gap-2">
        <button
          type="button"
          :disabled="!canManage || saving || !receiptWidthDirty"
          class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt disabled:cursor-not-allowed disabled:opacity-50"
          @click="save(SETTING_POS_RECEIPT_WIDTH_MM)"
        >
          <Loader2 v-if="saving" class="h-4 w-4 animate-spin" :stroke-width="1.5" />
          <Save v-else class="h-4 w-4" :stroke-width="1.5" />
          Simpan
        </button>
        <button
          type="button"
          :disabled="saving || !receiptWidthDirty"
          class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-medium text-ink-900 transition-colors hover:bg-canvas-alt disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          @click="reset(SETTING_POS_RECEIPT_WIDTH_MM)"
        >
          <RotateCcw class="h-4 w-4" :stroke-width="1.5" />
          Batal
        </button>
      </div>

      <dl class="mt-6 space-y-1 border-t border-hairline pt-4 text-xs text-ink-500">
        <div class="flex gap-2">
          <dt>Key</dt>
          <dd class="font-mono text-ink-700">{{ receiptWidth.key }}</dd>
        </div>
        <div class="flex gap-2">
          <dt>Terakhir diubah</dt>
          <dd>{{ formatDate(receiptWidth.updated_at) }}</dd>
        </div>
      </dl>

      <p v-if="!canManage" class="mt-4 text-xs text-ink-500">
        Anda tidak punya permission <span class="font-mono">settings.manage</span> — nilai hanya bisa dilihat.
      </p>
    </div>

    <!-- ============ Membership (§30.1) ============ -->
    <div v-if="membership" class="max-w-2xl rounded-lg border border-hairline bg-canvas p-6 md:p-8">
      <div class="flex items-start gap-3">
        <UserCheck class="mt-0.5 h-5 w-5 shrink-0 text-ink-600" :stroke-width="1.5" />
        <div>
          <h2 class="text-lg font-semibold text-ink-950">{{ membership.display_name }}</h2>
          <p class="mt-1 text-sm leading-relaxed text-ink-500">
            {{ membership.description }}
          </p>
        </div>
      </div>

      <div class="mt-6 flex items-center gap-3">
        <AdminToggleSwitch
          v-model="membershipDraftEnabled"
          :disabled="!canManage || saving"
          aria-label="Aktifkan fitur membership"
        />
        <span class="text-sm font-medium text-ink-900">
          {{ membershipDraftEnabled ? 'Aktif' : 'Nonaktif' }}
        </span>
      </div>

      <div class="mt-6 flex items-center gap-2">
        <button
          type="button"
          :disabled="!canManage || saving || !membershipDirty"
          class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt disabled:cursor-not-allowed disabled:opacity-50"
          @click="save(SETTING_MEMBERSHIP_ENABLED)"
        >
          <Loader2 v-if="saving" class="h-4 w-4 animate-spin" :stroke-width="1.5" />
          <Save v-else class="h-4 w-4" :stroke-width="1.5" />
          Simpan
        </button>
        <button
          type="button"
          :disabled="saving || !membershipDirty"
          class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-medium text-ink-900 transition-colors hover:bg-canvas-alt disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          @click="reset(SETTING_MEMBERSHIP_ENABLED)"
        >
          <RotateCcw class="h-4 w-4" :stroke-width="1.5" />
          Batal
        </button>
      </div>

      <!-- Konsekuensi yang gampang tidak disadari: menonaktifkan TIDAK me-reset
           data member yang sudah ada (§30.1) — hanya menyembunyikan halaman
           pengajuan & menolak diskon khusus member dipakai. -->
      <div
        v-if="membershipDirty && !membershipDraftEnabled"
        class="mt-5 flex items-start gap-2 rounded-md bg-brand-50 p-3 text-xs leading-relaxed text-ink-700"
      >
        <TriangleAlert class="mt-0.5 h-4 w-4 shrink-0 text-brand-500" :stroke-width="1.5" />
        <p>
          Menonaktifkan menyembunyikan halaman "Ajukan jadi Member" dan menolak diskon
          khusus member saat dipakai. Data member yang sudah ada tetap tersimpan dan
          otomatis berlaku lagi kalau diaktifkan ulang.
        </p>
      </div>

      <dl class="mt-6 space-y-1 border-t border-hairline pt-4 text-xs text-ink-500">
        <div class="flex gap-2">
          <dt>Key</dt>
          <dd class="font-mono text-ink-700">{{ membership.key }}</dd>
        </div>
        <div class="flex gap-2">
          <dt>Terakhir diubah</dt>
          <dd>{{ formatDate(membership.updated_at) }}</dd>
        </div>
      </dl>

      <p v-if="!canManage" class="mt-4 text-xs text-ink-500">
        Anda tidak punya permission <span class="font-mono">settings.manage</span> — nilai hanya bisa dilihat.
      </p>
    </div>

    <!-- ============ Rekening & QRIS (§7) ============ -->
    <div v-if="paymentLoaded" class="max-w-2xl rounded-lg border border-hairline bg-canvas p-6 md:p-8">
      <div class="flex items-start gap-3">
        <Landmark class="mt-0.5 h-5 w-5 shrink-0 text-ink-600" :stroke-width="1.5" />
        <div>
          <h2 class="text-lg font-semibold text-ink-950">Rekening & QRIS</h2>
          <p class="mt-1 text-sm leading-relaxed text-ink-500">
            Tujuan transfer yang tampil langsung ke pembeli di halaman pembayaran (lacak resi & akun
            pesanan). Perubahan berlaku seketika — tanpa deploy ulang.
          </p>
        </div>
      </div>

      <!-- Bank -->
      <div class="mt-6 space-y-4">
        <div>
          <label for="payment-bank-name" class="block text-sm font-medium text-ink-900">Nama bank</label>
          <input
            id="payment-bank-name"
            v-model="draft[SETTING_PAYMENT_BANK_NAME]"
            type="text"
            placeholder="Contoh: BCA"
            :disabled="!canManage || paymentSaving"
            class="mt-1 block w-full max-w-xs rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 transition-colors focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20 disabled:cursor-not-allowed disabled:bg-canvas-alt disabled:text-ink-500"
          >
        </div>

        <div>
          <label for="payment-account-name" class="block text-sm font-medium text-ink-900">Nama pemilik rekening</label>
          <input
            id="payment-account-name"
            v-model="draft[SETTING_PAYMENT_ACCOUNT_NAME]"
            type="text"
            placeholder="Contoh: CV WANSHOU NIAGA UTAMA"
            :disabled="!canManage || paymentSaving"
            class="mt-1 block w-full max-w-sm rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 transition-colors focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20 disabled:cursor-not-allowed disabled:bg-canvas-alt disabled:text-ink-500"
          >
        </div>

        <div>
          <label for="payment-account-number" class="block text-sm font-medium text-ink-900">Nomor rekening</label>
          <input
            id="payment-account-number"
            v-model="draft[SETTING_PAYMENT_ACCOUNT_NUMBER]"
            type="text"
            inputmode="numeric"
            placeholder="Contoh: 3245070777"
            :disabled="!canManage || paymentSaving"
            class="mt-1 block w-full max-w-xs rounded-md border border-hairline bg-canvas px-3 py-2 font-mono text-sm tracking-wider text-ink-900 transition-colors focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20 disabled:cursor-not-allowed disabled:bg-canvas-alt disabled:text-ink-500"
          >
          <p class="mt-1 text-xs text-ink-500">
            Salah satu angka saja bisa membuat transfer pembeli salah alamat — perubahan nomor ini
            wajib dikonfirmasi ulang sebelum disimpan.
          </p>
        </div>
      </div>

      <!-- QRIS -->
      <div class="mt-6 border-t border-hairline pt-6">
        <div class="flex items-center gap-2">
          <QrCode class="h-4 w-4 text-ink-500" :stroke-width="1.5" />
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
            QRIS (opsional — kosongkan kalau belum tersedia)
          </p>
        </div>

        <div class="mt-3 space-y-4">
          <div>
            <label for="payment-qris-note" class="block text-sm font-medium text-ink-900">Catatan QRIS</label>
            <textarea
              id="payment-qris-note"
              v-model="draft[SETTING_PAYMENT_QRIS_NOTE]"
              rows="2"
              maxlength="300"
              placeholder="Contoh: Pindai QRIS dengan aplikasi apa pun berlogo QRIS, lalu unggah bukti bayarnya."
              :disabled="!canManage || paymentSaving"
              class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 transition-colors focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20 disabled:cursor-not-allowed disabled:bg-canvas-alt disabled:text-ink-500"
            />
          </div>

          <div class="flex flex-wrap gap-4">
            <div class="min-w-0 flex-1">
              <label for="payment-qris-merchant" class="block text-sm font-medium text-ink-900">Nama merchant di QRIS</label>
              <input
                id="payment-qris-merchant"
                v-model="draft[SETTING_PAYMENT_QRIS_MERCHANT_NAME]"
                type="text"
                placeholder="Nama yang muncul saat dipindai (boleh beda dari nama rekening)"
                :disabled="!canManage || paymentSaving"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 transition-colors focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20 disabled:cursor-not-allowed disabled:bg-canvas-alt disabled:text-ink-500"
              >
            </div>
            <div class="min-w-0 flex-1">
              <label for="payment-qris-nmid" class="block text-sm font-medium text-ink-900">NMID</label>
              <input
                id="payment-qris-nmid"
                v-model="draft[SETTING_PAYMENT_QRIS_NMID]"
                type="text"
                placeholder="Contoh: ID2026569993978"
                :disabled="!canManage || paymentSaving"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 font-mono text-sm text-ink-900 transition-colors focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20 disabled:cursor-not-allowed disabled:bg-canvas-alt disabled:text-ink-500"
              >
            </div>
          </div>
        </div>

        <p class="mt-3 text-xs text-ink-500">
          Gambar kode QRIS diunggah dari <NuxtLink to="/admin/site-media" class="text-brand-500 underline hover:text-brand-600">Kelola Media Situs</NuxtLink> (slot "QRIS"), bukan di sini.
        </p>
      </div>

      <div class="mt-6 flex items-center gap-2">
        <button
          type="button"
          :disabled="!canManage || paymentSaving || !paymentDirty"
          class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt disabled:cursor-not-allowed disabled:opacity-50"
          @click="requestSavePayment"
        >
          <Loader2 v-if="paymentSaving" class="h-4 w-4 animate-spin" :stroke-width="1.5" />
          <Save v-else class="h-4 w-4" :stroke-width="1.5" />
          Simpan
        </button>
        <button
          type="button"
          :disabled="paymentSaving || !paymentDirty"
          class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-medium text-ink-900 transition-colors hover:bg-canvas-alt disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          @click="resetPayment"
        >
          <RotateCcw class="h-4 w-4" :stroke-width="1.5" />
          Batal
        </button>
      </div>

      <!-- Konsekuensi yang gampang tidak disadari: perubahan tampil langsung
           ke pembeli, tidak ada masa transisi/preview terpisah. -->
      <div
        v-if="accountNumberDirty"
        class="mt-5 flex items-start gap-2 rounded-md bg-brand-50 p-3 text-xs leading-relaxed text-ink-700"
      >
        <TriangleAlert class="mt-0.5 h-4 w-4 shrink-0 text-brand-500" :stroke-width="1.5" />
        <p>
          Nomor rekening berubah. Anda akan diminta konfirmasi sekali lagi sebelum perubahan ini
          disimpan dan tampil ke pembeli.
        </p>
      </div>

      <p v-if="!canManage" class="mt-4 text-xs text-ink-500">
        Anda tidak punya permission <span class="font-mono">settings.manage</span> — nilai hanya bisa dilihat.
      </p>
    </div>

    <div v-if="!retention && !receiptWidth && !membership && !paymentLoaded" class="rounded-lg border border-hairline bg-canvas p-8 text-center">
      <p class="text-sm text-ink-500">Belum ada pengaturan yang bisa diubah.</p>
    </div>
    </div>

    <!-- ============ Konfirmasi perubahan nomor rekening ============ -->
    <AdminConfirmDialog
      v-model:open="accountNumberConfirmOpen"
      title="Ubah nomor rekening tujuan?"
      message="Periksa sekali lagi sebelum menyimpan — nomor ini langsung tampil ke pembeli sebagai tujuan transfer."
      confirm-label="Ya, simpan perubahan"
      variant="danger"
      :loading="paymentSaving"
      @confirm="savePaymentFields"
    >
      <div class="mt-4 grid grid-cols-2 gap-3 text-sm">
        <div class="rounded-md border border-hairline bg-canvas-alt/60 p-3">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Nomor lama</p>
          <p class="mt-1 font-mono text-ink-700 tracking-wider break-all">{{ accountNumberOldValue || '—' }}</p>
        </div>
        <div class="rounded-md border border-brand-200 bg-brand-50/60 p-3">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-brand-700">Nomor baru</p>
          <p class="mt-1 font-mono text-ink-950 font-semibold tracking-wider break-all">{{ accountNumberNewValue || '—' }}</p>
        </div>
      </div>
      <p class="mt-3 text-xs text-ink-500">
        Atas nama bank: <span class="font-medium text-ink-700">{{ bankNameNewValue || '—' }}</span>
      </p>
    </AdminConfirmDialog>
  </section>
</template>
