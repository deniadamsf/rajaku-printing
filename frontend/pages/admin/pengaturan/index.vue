<script setup lang="ts">
/**
 * /admin/pengaturan — Setting global aplikasi (§19).
 *
 * Isi saat ini satu setting: retensi file desain (hari). Nilainya dibaca job
 * retention tiap kali jalan, jadi perubahan di sini langsung berlaku di run
 * berikutnya — tanpa deploy ulang, tanpa restart backend.
 *
 * Sengaja tidak generic-form: tiap setting punya konteks & konsekuensi sendiri
 * yang perlu dijelaskan ke admin (menurunkan retensi = file lama langsung
 * terhapus di sweep berikutnya). Setting baru nanti ditambah sebagai card
 * sendiri, bukan diseragamkan jadi tabel key/value telanjang.
 *
 * Design: patuh CLAUDE.md §26 (Fraunces + Inter + Lucide, brand/ink/hairline).
 */
import { HardDrive, Save, RotateCcw, Loader2, TriangleAlert } from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import { SETTING_DESIGN_RETENTION_DAYS, type AppSetting } from '~/composables/useAdminSettings'

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

    <div v-else-if="retention" class="max-w-2xl rounded-lg border border-hairline bg-canvas p-6 md:p-8">
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
            class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-medium text-ink-900 transition-colors hover:bg-canvas-alt disabled:cursor-not-allowed disabled:opacity-50"
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

    <div v-else class="rounded-lg border border-hairline bg-canvas p-8 text-center">
      <p class="text-sm text-ink-500">Belum ada pengaturan yang bisa diubah.</p>
    </div>
  </section>
</template>
