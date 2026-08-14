<script setup lang="ts">
/**
 * /admin/role — Kelola role & toggle permission (§10).
 *
 * Layout 2-kolom (desktop):
 *   - Kiri: daftar role dgn permission count. Klik untuk pilih.
 *   - Kanan: editor role terpilih —
 *       • Header (Fraunces + is_system badge)
 *       • Basic (display_name + description) — edit inline, disabled kalau system
 *       • Grid permission toggle per kategori (admin/catalog/cms/…)
 *       • Simpan / Reset changes
 *       • Delete role (non-system saja)
 *
 * Bikin role baru via tombol "+ Role baru" → modal minimal (name + display + desc).
 * Permission assignment lakukan setelah dibuat, dari editor.
 *
 * Design: patuh CLAUDE.md §26 (Fraunces + Inter + Lucide, brand/ink/hairline).
 */
import {
  ShieldCheck,
  Plus,
  Save,
  RotateCcw,
  Trash2,
  Lock,
  Loader2,
  ChevronRight,
} from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import type { AdminRole, AdminPermission } from '~/types/staff'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Role & Permission — Rajaku Admin' })

const auth = useAuthStore()
const roleApi = useAdminRole()

const canManage = computed(() => auth.hasPermission('role.manage'))

// -------------------- state --------------------
const roles = ref<AdminRole[]>([])
const permissions = ref<AdminPermission[]>([])
const rolesLoading = ref(false)
const permsLoading = ref(false)

const selectedId = ref<string | null>(null)
const selectedDetail = ref<AdminRole | null>(null)
const detailLoading = ref(false)

const errorMsg = ref<string | null>(null)
const successMsg = ref<string | null>(null)

// Basic edit fields (dirty tracked separately from permission set)
const basicForm = reactive({
  display_name: '',
  description: '',
})
const basicOriginal = ref<{ display_name: string; description: string } | null>(null)
const basicSaving = ref(false)

// Permission toggle set (Set<code>)
const activeCodes = ref<Set<string>>(new Set())
const originalCodes = ref<Set<string>>(new Set())
const permsSaving = ref(false)

function showSuccess(msg: string) {
  successMsg.value = msg
  setTimeout(() => (successMsg.value = null), 2500)
}

function toApiError(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

// -------------------- fetch --------------------
async function fetchAll() {
  rolesLoading.value = true
  permsLoading.value = true
  errorMsg.value = null
  try {
    const [rolesRes, permsRes] = await Promise.all([
      roleApi.listRoles(),
      roleApi.listPermissions(),
    ])
    roles.value = rolesRes.items
    permissions.value = permsRes.items
    // Auto-select first role kalau belum ada seleksi.
    if (!selectedId.value && roles.value.length > 0) {
      await selectRole(roles.value[0].id)
    }
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal memuat data')
  } finally {
    rolesLoading.value = false
    permsLoading.value = false
  }
}

async function selectRole(id: string) {
  if (isDirty.value) {
    if (!confirm('Ada perubahan yang belum disimpan. Buang perubahan?')) return
  }
  selectedId.value = id
  detailLoading.value = true
  errorMsg.value = null
  try {
    const r = await roleApi.getRole(id)
    selectedDetail.value = r
    basicForm.display_name = r.display_name
    basicForm.description = r.description ?? ''
    basicOriginal.value = { display_name: r.display_name, description: r.description ?? '' }
    const codes = new Set((r.permissions ?? []).map((p) => p.code))
    activeCodes.value = new Set(codes)
    originalCodes.value = new Set(codes)
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal memuat detail role')
    selectedDetail.value = null
  } finally {
    detailLoading.value = false
  }
}

onMounted(fetchAll)

// -------------------- computed --------------------
const permsByCategory = computed(() => {
  const groups: Record<string, AdminPermission[]> = {}
  for (const p of permissions.value) {
    if (!groups[p.category]) groups[p.category] = []
    groups[p.category].push(p)
  }
  // Deterministic category order
  const orderedKeys = Object.keys(groups).sort()
  const out: Array<{ category: string; items: AdminPermission[] }> = []
  for (const k of orderedKeys) {
    out.push({ category: k, items: groups[k] })
  }
  return out
})

const isPermsDirty = computed(() => {
  if (activeCodes.value.size !== originalCodes.value.size) return true
  for (const c of activeCodes.value) {
    if (!originalCodes.value.has(c)) return true
  }
  return false
})

const isBasicDirty = computed(() => {
  if (!basicOriginal.value) return false
  return (
    basicForm.display_name.trim() !== basicOriginal.value.display_name ||
    (basicForm.description ?? '').trim() !== basicOriginal.value.description
  )
})

const isDirty = computed(() => isPermsDirty.value || isBasicDirty.value)

const isSystemRole = computed(() => !!selectedDetail.value?.is_system)

// -------------------- actions --------------------
function togglePerm(code: string) {
  if (!canManage.value) return
  const next = new Set(activeCodes.value)
  if (next.has(code)) next.delete(code)
  else next.add(code)
  activeCodes.value = next
}

function selectAllInCategory(category: string) {
  if (!canManage.value) return
  const codes = permsByCategory.value.find((g) => g.category === category)?.items.map((p) => p.code) ?? []
  const next = new Set(activeCodes.value)
  const allActive = codes.every((c) => next.has(c))
  if (allActive) {
    for (const c of codes) next.delete(c)
  } else {
    for (const c of codes) next.add(c)
  }
  activeCodes.value = next
}

async function savePerms() {
  if (!selectedDetail.value) return
  permsSaving.value = true
  errorMsg.value = null
  try {
    await roleApi.setPermissions(selectedDetail.value.id, [...activeCodes.value])
    originalCodes.value = new Set(activeCodes.value)
    showSuccess(`Permission role "${selectedDetail.value.display_name}" diperbarui.`)
    // Refresh roles list for perm count.
    const res = await roleApi.listRoles()
    roles.value = res.items
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal menyimpan permission')
  } finally {
    permsSaving.value = false
  }
}

async function saveBasic() {
  if (!selectedDetail.value || isSystemRole.value) return
  basicSaving.value = true
  errorMsg.value = null
  try {
    await roleApi.updateRoleBasic(selectedDetail.value.id, {
      display_name: basicForm.display_name.trim(),
      description: basicForm.description.trim() || undefined,
    })
    basicOriginal.value = {
      display_name: basicForm.display_name.trim(),
      description: basicForm.description.trim(),
    }
    showSuccess('Detail role diperbarui.')
    // Refresh list untuk sync display_name.
    const res = await roleApi.listRoles()
    roles.value = res.items
    if (selectedDetail.value) {
      selectedDetail.value.display_name = basicOriginal.value.display_name
      selectedDetail.value.description = basicOriginal.value.description || undefined
    }
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal update role')
  } finally {
    basicSaving.value = false
  }
}

function resetChanges() {
  if (!selectedDetail.value) return
  activeCodes.value = new Set(originalCodes.value)
  if (basicOriginal.value) {
    basicForm.display_name = basicOriginal.value.display_name
    basicForm.description = basicOriginal.value.description
  }
}

async function deleteRole() {
  const r = selectedDetail.value
  if (!r || r.is_system) return
  if (!confirm(`Hapus role "${r.display_name}"? Aksi ini permanen — staff yang punya role ini akan kehilangan permission-nya.`)) return
  errorMsg.value = null
  try {
    await roleApi.deleteRole(r.id)
    showSuccess(`Role "${r.display_name}" dihapus.`)
    selectedDetail.value = null
    selectedId.value = null
    await fetchAll()
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal hapus role')
  }
}

// -------------------- create role modal --------------------
const createOpen = ref(false)
const createForm = reactive({
  name: '',
  display_name: '',
  description: '',
})
const createSaving = ref(false)

function openCreate() {
  createForm.name = ''
  createForm.display_name = ''
  createForm.description = ''
  createOpen.value = true
}

async function submitCreate() {
  createSaving.value = true
  errorMsg.value = null
  try {
    const r = await roleApi.createRole({
      name: createForm.name.trim(),
      display_name: createForm.display_name.trim(),
      description: createForm.description.trim() || undefined,
    })
    showSuccess(`Role "${r.display_name}" dibuat. Pilih permission-nya di panel kanan.`)
    createOpen.value = false
    await fetchAll()
    await selectRole(r.id)
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal buat role')
  } finally {
    createSaving.value = false
  }
}

// -------------------- helpers --------------------
const categoryLabel: Record<string, string> = {
  admin: 'Admin',
  catalog: 'Katalog',
  cms: 'Konten (CMS)',
  design: 'Desain',
  order: 'Order',
  payment: 'Pembayaran',
  pos: 'POS / Kasir',
  production: 'Produksi',
  shipping: 'Pengiriman',
}
</script>

<template>
  <section>
    <AdminPageHeader
      title="Role & Permission"
      subtitle="Toggle permission per role. Perubahan langsung mempengaruhi menu & akses staff dengan role tsb."
    >
      <template #actions>
        <button
          v-if="canManage"
          type="button"
          class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt transition-colors"
          @click="openCreate"
        >
          <Plus class="h-4 w-4" :stroke-width="1.75" />
          Role baru
        </button>
      </template>
    </AdminPageHeader>

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />
    <AlertMessage v-if="successMsg" variant="success" :message="successMsg" class="mb-4" />

    <div class="grid gap-6 lg:grid-cols-[280px_minmax(0,1fr)]">
      <!-- ============================ Role list (left) ============================ -->
      <aside class="rounded-lg border border-hairline bg-canvas p-2 h-max">
        <div class="px-2 py-1.5 flex items-center gap-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
          <ShieldCheck class="h-3.5 w-3.5" :stroke-width="1.75" />
          Daftar role
        </div>
        <div v-if="rolesLoading" class="p-4 text-center text-xs text-ink-500">
          <Loader2 class="mx-auto h-4 w-4 animate-spin" :stroke-width="1.75" />
        </div>
        <ul v-else class="space-y-0.5">
          <li v-for="r in roles" :key="r.id">
            <button
              type="button"
              :class="[
                'w-full text-left px-3 py-2.5 rounded-md flex items-center gap-2 transition-colors',
                selectedId === r.id
                  ? 'bg-brand-500 text-canvas'
                  : 'text-ink-700 hover:bg-canvas-alt',
              ]"
              @click="selectRole(r.id)"
            >
              <div class="flex-1 min-w-0">
                <p :class="['text-sm font-medium truncate', selectedId === r.id ? 'text-canvas' : 'text-ink-900']">
                  {{ r.display_name }}
                </p>
                <p :class="['mt-0.5 font-mono text-[10px] truncate', selectedId === r.id ? 'text-canvas/70' : 'text-ink-500']">
                  {{ r.name }}
                </p>
              </div>
              <Lock
                v-if="r.is_system"
                :class="['h-3 w-3 flex-none', selectedId === r.id ? 'text-canvas/70' : 'text-ink-400']"
                :stroke-width="1.75"
                aria-label="Role system"
              />
              <ChevronRight
                v-if="selectedId !== r.id"
                class="h-3.5 w-3.5 flex-none text-ink-400"
                :stroke-width="1.75"
              />
            </button>
          </li>
        </ul>
      </aside>

      <!-- ============================ Editor (right) ============================ -->
      <div v-if="detailLoading" class="rounded-lg border border-hairline bg-canvas p-12 text-center text-sm text-ink-500">
        <Loader2 class="mx-auto h-4 w-4 animate-spin" :stroke-width="1.75" />
        <p class="mt-2">Memuat detail role…</p>
      </div>
      <div v-else-if="!selectedDetail" class="rounded-lg border border-hairline bg-canvas p-12 text-center text-sm text-ink-500">
        Pilih role di kiri untuk mulai edit.
      </div>
      <div v-else class="space-y-6">
        <!-- Header + basic edit -->
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <div class="flex items-center gap-2">
                <h2 class="font-serif text-xl font-semibold text-ink-950">{{ selectedDetail.display_name }}</h2>
                <span
                  v-if="selectedDetail.is_system"
                  class="inline-flex items-center gap-1 rounded-full bg-gold-50 px-2 py-0.5 text-[10px] font-medium text-gold-900 ring-1 ring-inset ring-gold-200"
                >
                  <Lock class="h-3 w-3" :stroke-width="1.75" />
                  System
                </span>
              </div>
              <p class="mt-1 font-mono text-xs text-ink-500">{{ selectedDetail.name }}</p>
            </div>
            <button
              v-if="canManage && !selectedDetail.is_system"
              type="button"
              class="inline-flex items-center gap-1 rounded-md border border-brand-200 bg-canvas px-3 py-1.5 text-xs font-semibold text-brand-700 hover:bg-brand-50 hover:border-brand-300 transition-colors"
              @click="deleteRole"
            >
              <Trash2 class="h-3.5 w-3.5" :stroke-width="1.75" />
              Hapus role
            </button>
          </div>

          <div class="mt-5 grid gap-4 sm:grid-cols-2">
            <div>
              <label for="role-display" class="block text-sm font-medium text-ink-900">Display name</label>
              <input
                id="role-display"
                v-model="basicForm.display_name"
                type="text"
                :disabled="isSystemRole || !canManage"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
              >
            </div>
            <div>
              <label for="role-desc" class="block text-sm font-medium text-ink-900">Deskripsi</label>
              <input
                id="role-desc"
                v-model="basicForm.description"
                type="text"
                :disabled="isSystemRole || !canManage"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
              >
            </div>
          </div>
          <p v-if="isSystemRole" class="mt-3 text-xs text-ink-500">
            Role system tidak bisa diubah nama/deskripsi/dihapus — tapi permission-nya <strong class="text-ink-900">toggle-able</strong>.
          </p>

          <div v-if="isBasicDirty && !isSystemRole" class="mt-4 flex justify-end">
            <button
              type="button"
              :disabled="basicSaving"
              class="inline-flex items-center gap-1 rounded-md bg-brand-500 px-3 py-1.5 text-xs font-semibold text-canvas hover:bg-brand-600 transition-colors disabled:opacity-60"
              @click="saveBasic"
            >
              <Loader2 v-if="basicSaving" class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
              <Save v-else class="h-3.5 w-3.5" :stroke-width="1.75" />
              Simpan detail
            </button>
          </div>
        </div>

        <!-- Permission editor -->
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex flex-wrap items-center justify-between gap-3 mb-5">
            <div>
              <h3 class="text-sm font-semibold text-ink-900">Permission</h3>
              <p class="mt-1 text-xs text-ink-500">
                {{ activeCodes.size }} dari {{ permissions.length }} aktif
                <span v-if="isPermsDirty" class="ml-1 inline-flex items-center rounded-full bg-brand-50 px-2 py-0.5 text-[10px] font-medium text-brand-700 ring-1 ring-inset ring-brand-200">
                  Belum disimpan
                </span>
              </p>
            </div>
            <div v-if="canManage" class="flex gap-2">
              <button
                type="button"
                :disabled="!isPermsDirty || permsSaving"
                class="inline-flex items-center gap-1 rounded-md border border-hairline bg-canvas px-3 py-1.5 text-xs font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                @click="resetChanges"
              >
                <RotateCcw class="h-3.5 w-3.5" :stroke-width="1.75" />
                Batal perubahan
              </button>
              <button
                type="button"
                :disabled="!isPermsDirty || permsSaving"
                class="inline-flex items-center gap-1 rounded-md bg-brand-500 px-3 py-1.5 text-xs font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                @click="savePerms"
              >
                <Loader2 v-if="permsSaving" class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
                <Save v-else class="h-3.5 w-3.5" :stroke-width="1.75" />
                Simpan permission
              </button>
            </div>
          </div>

          <div v-if="permsLoading" class="text-center text-sm text-ink-500 py-6">
            <Loader2 class="mx-auto h-4 w-4 animate-spin" :stroke-width="1.75" />
          </div>
          <div v-else class="space-y-6">
            <div v-for="group in permsByCategory" :key="group.category">
              <div class="mb-2 flex items-center justify-between">
                <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
                  {{ categoryLabel[group.category] ?? group.category }}
                </p>
                <button
                  v-if="canManage"
                  type="button"
                  class="text-[11px] font-medium text-ink-500 hover:text-brand-500 transition-colors"
                  @click="selectAllInCategory(group.category)"
                >
                  {{ group.items.every((p) => activeCodes.has(p.code)) ? 'Kosongkan grup' : 'Pilih semua grup' }}
                </button>
              </div>
              <div class="grid gap-2 sm:grid-cols-2">
                <label
                  v-for="p in group.items"
                  :key="p.id"
                  :class="[
                    'flex cursor-pointer items-start gap-2 rounded-md border p-3 text-xs transition-colors',
                    activeCodes.has(p.code)
                      ? 'border-brand-500 bg-brand-50/50'
                      : 'border-hairline bg-canvas hover:border-ink-300',
                    !canManage && 'cursor-not-allowed opacity-70',
                  ]"
                >
                  <input
                    type="checkbox"
                    class="mt-0.5 accent-brand-500"
                    :checked="activeCodes.has(p.code)"
                    :disabled="!canManage"
                    @change="togglePerm(p.code)"
                  >
                  <span class="flex-1 min-w-0">
                    <span class="block font-semibold text-ink-900">{{ p.display_name }}</span>
                    <span class="mt-0.5 block font-mono text-[10px] text-ink-500 truncate">{{ p.code }}</span>
                    <span v-if="p.description" class="mt-0.5 block text-ink-500">{{ p.description }}</span>
                  </span>
                </label>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ============================ Create role modal ============================ -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="createOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="!createSaving && (createOpen = false)" />
          <div role="dialog" aria-modal="true" class="relative w-full max-w-md rounded-lg bg-canvas shadow-xl ring-1 ring-black/5">
            <form class="p-6" @submit.prevent="submitCreate">
              <h3 class="font-serif text-lg font-semibold text-ink-950">Role baru</h3>
              <p class="mt-1 text-xs text-ink-500 leading-relaxed">
                Buat role kosong dulu. Toggle permission-nya di panel editor setelah dibuat.
              </p>

              <div class="mt-4 space-y-4">
                <div>
                  <label for="role-name" class="block text-sm font-medium text-ink-900">Name (internal) <span class="text-brand-500">*</span></label>
                  <input
                    id="role-name"
                    v-model="createForm.name"
                    type="text"
                    required
                    placeholder="marketing_lead"
                    pattern="^[a-z][a-z0-9_]*$"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-mono placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                  <p class="mt-1 text-xs text-ink-500">snake_case (a-z, 0-9, _). Tidak bisa diubah nanti.</p>
                </div>
                <div>
                  <label for="role-display-new" class="block text-sm font-medium text-ink-900">Display name <span class="text-brand-500">*</span></label>
                  <input
                    id="role-display-new"
                    v-model="createForm.display_name"
                    type="text"
                    required
                    placeholder="Marketing Lead"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                </div>
                <div>
                  <label for="role-desc-new" class="block text-sm font-medium text-ink-900">Deskripsi</label>
                  <textarea
                    id="role-desc-new"
                    v-model="createForm.description"
                    rows="2"
                    placeholder="Kelola konten & artikel marketing"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  />
                </div>
              </div>

              <div class="mt-6 flex justify-end gap-2">
                <button
                  type="button"
                  class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50"
                  :disabled="createSaving"
                  @click="createOpen = false"
                >
                  Batal
                </button>
                <button
                  type="submit"
                  :disabled="createSaving"
                  class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 transition-colors disabled:opacity-60"
                >
                  <Loader2 v-if="createSaving" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
                  Buat role
                </button>
              </div>
            </form>
          </div>
        </div>
      </Transition>
    </Teleport>
  </section>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
