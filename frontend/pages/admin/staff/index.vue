<script setup lang="ts">
/**
 * /admin/staff — Kelola akun staff (§10, "Kelola Staff").
 *
 * Flow super admin:
 *   1. List staff dgn search + filter aktif/nonaktif + pagination.
 *   2. "Invite staff" modal — input nama/email/phone + assign role(s).
 *   3. Setelah create sukses, tampil panel dgn invite URL + token
 *      (**satu-satunya kesempatan lihat** — super admin forward manual ke calon staff).
 *   4. Edit staff (nama/email/phone), assign roles, activate/deactivate.
 *
 * Design: patuh CLAUDE.md §26 (Fraunces + Inter + Lucide, brand/ink/hairline).
 */
import {
  Plus,
  Pencil,
  Power,
  Users,
  ShieldCheck,
  Copy,
  Check,
  Search,
  X,
  Loader2,
} from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import type {
  AdminRole,
  AdminStaffUser,
  CreateStaffInput,
  CreateStaffResult,
  UpdateStaffInput,
} from '~/types/staff'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Kelola Staff — Rajaku Admin' })

const staffApi = useAdminStaff()
const auth = useAuthStore()

const canManage = computed(() => auth.hasPermission('staff.manage'))

// -------------------- state --------------------
const staff = ref<AdminStaffUser[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const search = ref('')
const activeFilter = ref<'' | 'active' | 'inactive'>('')
const loading = ref(false)
const errorMsg = ref<string | null>(null)
const successMsg = ref<string | null>(null)

const roles = ref<AdminRole[]>([])
const rolesLoading = ref(false)

function showSuccess(msg: string) {
  successMsg.value = msg
  setTimeout(() => (successMsg.value = null), 2500)
}

function toApiError(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

// -------------------- fetch --------------------
async function fetchStaff() {
  loading.value = true
  errorMsg.value = null
  try {
    const res = await staffApi.listStaff({
      q: search.value.trim() || undefined,
      active: activeFilter.value === 'active' ? true : activeFilter.value === 'inactive' ? false : '',
      page: page.value,
      pageSize: pageSize.value,
    })
    staff.value = res.Items ?? []
    total.value = res.Total ?? 0
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal memuat daftar staff')
  } finally {
    loading.value = false
  }
}

async function fetchRoles() {
  rolesLoading.value = true
  try {
    const res = await staffApi.listRoles()
    roles.value = res.items
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal memuat daftar role')
  } finally {
    rolesLoading.value = false
  }
}

onMounted(async () => {
  await Promise.all([fetchStaff(), fetchRoles()])
})

// Debounced search
let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(search, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    fetchStaff()
  }, 350)
})
watch([activeFilter, page], () => {
  fetchStaff()
})

// -------------------- invite modal --------------------
const inviteModalOpen = ref(false)
const inviteForm = reactive<CreateStaffInput>({
  name: '',
  email: '',
  phone: '',
  role_ids: [],
})
const inviteSaving = ref(false)
const inviteResult = ref<CreateStaffResult | null>(null)
const copiedField = ref<'url' | 'token' | null>(null)

function openInviteModal() {
  errorMsg.value = null
  inviteResult.value = null
  inviteForm.name = ''
  inviteForm.email = ''
  inviteForm.phone = ''
  inviteForm.role_ids = []
  inviteModalOpen.value = true
}

function closeInviteModal() {
  inviteModalOpen.value = false
  inviteResult.value = null
}

async function submitInvite() {
  inviteSaving.value = true
  errorMsg.value = null
  try {
    const res = await staffApi.createStaff({
      name: inviteForm.name.trim(),
      email: inviteForm.email.trim(),
      phone: inviteForm.phone.trim(),
      role_ids: [...inviteForm.role_ids],
    })
    inviteResult.value = res
    await fetchStaff()
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal membuat staff')
  } finally {
    inviteSaving.value = false
  }
}

async function copyToClipboard(text: string, field: 'url' | 'token') {
  try {
    await navigator.clipboard.writeText(text)
    copiedField.value = field
    setTimeout(() => {
      if (copiedField.value === field) copiedField.value = null
    }, 1500)
  } catch {
    // fallback: user can manually select
  }
}

// -------------------- edit modal --------------------
const editModalOpen = ref(false)
const editingStaff = ref<AdminStaffUser | null>(null)
const editForm = reactive<UpdateStaffInput & { role_ids: string[] }>({
  name: '',
  email: '',
  phone: '',
  role_ids: [],
})
const editSaving = ref(false)

function openEditModal(s: AdminStaffUser) {
  errorMsg.value = null
  editingStaff.value = s
  editForm.name = s.name
  editForm.email = s.email ?? ''
  editForm.phone = s.phone
  editForm.role_ids = (s.roles ?? []).map((r) => r.id)
  editModalOpen.value = true
}

async function submitEdit() {
  const s = editingStaff.value
  if (!s) return
  editSaving.value = true
  errorMsg.value = null
  try {
    // Basic profile update.
    await staffApi.updateStaff(s.id, {
      name: editForm.name.trim(),
      email: editForm.email.trim(),
      phone: editForm.phone.trim(),
    })
    // Role diff — only PATCH if changed.
    const currentIds = new Set((s.roles ?? []).map((r) => r.id))
    const newIds = new Set(editForm.role_ids)
    const same = currentIds.size === newIds.size && [...currentIds].every((id) => newIds.has(id))
    if (!same) {
      await staffApi.assignRoles(s.id, [...editForm.role_ids])
    }
    showSuccess(`Staff "${editForm.name}" diperbarui.`)
    editModalOpen.value = false
    editingStaff.value = null
    await fetchStaff()
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal update staff')
  } finally {
    editSaving.value = false
  }
}

// -------------------- activate/deactivate --------------------
async function toggleActive(s: AdminStaffUser) {
  const verb = s.is_active ? 'Nonaktifkan' : 'Aktifkan kembali'
  if (!confirm(`${verb} akun "${s.name}"?`)) return
  errorMsg.value = null
  try {
    if (s.is_active) await staffApi.deactivateStaff(s.id)
    else await staffApi.activateStaff(s.id)
    showSuccess(`Akun "${s.name}" ${s.is_active ? 'dinonaktifkan' : 'diaktifkan kembali'}.`)
    await fetchStaff()
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal mengubah status akun')
  }
}

// -------------------- helpers --------------------
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

function fmtDate(iso?: string): string {
  if (!iso) return '—'
  try {
    return new Date(iso).toLocaleDateString('id-ID', {
      day: '2-digit',
      month: 'short',
      year: 'numeric',
    })
  } catch {
    return iso
  }
}

function fmtDateTime(iso?: string): string {
  if (!iso) return '—'
  try {
    return new Date(iso).toLocaleString('id-ID', {
      day: '2-digit',
      month: 'short',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return iso
  }
}
</script>

<template>
  <section>
    <AdminPageHeader
      title="Kelola Staff"
      subtitle="Invite akun karyawan baru & atur role-nya. Akun customer tidak muncul di sini."
    >
      <template #actions>
        <button
          v-if="canManage"
          type="button"
          class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt transition-colors"
          @click="openInviteModal"
        >
          <Plus class="h-4 w-4" :stroke-width="1.75" />
          Invite staff
        </button>
      </template>
    </AdminPageHeader>

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />
    <AlertMessage v-if="successMsg" variant="success" :message="successMsg" class="mb-4" />

    <!-- Toolbar: search + filter -->
    <div class="mb-4 flex flex-wrap items-center gap-2">
      <div class="relative flex-1 min-w-[240px]">
        <Search class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-ink-400" :stroke-width="1.75" />
        <input
          v-model="search"
          type="search"
          placeholder="Cari nama, email, atau no. HP…"
          class="w-full rounded-md border border-hairline bg-canvas pl-9 pr-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
        >
      </div>
      <div class="inline-flex rounded-md border border-hairline overflow-hidden text-sm">
        <button
          v-for="opt in [
            { v: '', label: 'Semua' },
            { v: 'active', label: 'Aktif' },
            { v: 'inactive', label: 'Nonaktif' },
          ]"
          :key="opt.v"
          type="button"
          :class="[
            'px-3 py-1.5 border-r border-hairline last:border-r-0 transition-colors',
            activeFilter === opt.v ? 'bg-ink-950 text-canvas' : 'bg-canvas text-ink-700 hover:bg-canvas-alt',
          ]"
          @click="activeFilter = opt.v as '' | 'active' | 'inactive'; page = 1"
        >
          {{ opt.label }}
        </button>
      </div>
    </div>

    <!-- Staff table -->
    <div class="rounded-lg border border-hairline bg-canvas overflow-hidden">
      <table class="w-full text-sm">
        <thead class="bg-canvas-alt border-b border-hairline text-left">
          <tr>
            <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Nama / Email</th>
            <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 hidden md:table-cell w-40">Phone</th>
            <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Role</th>
            <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 w-28">Status</th>
            <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 hidden lg:table-cell w-40">Login terakhir</th>
            <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 w-32 text-right">Aksi</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-hairline">
          <tr v-if="loading">
            <td colspan="6" class="px-4 py-12 text-center text-sm text-ink-500">
              <Loader2 class="mx-auto h-4 w-4 animate-spin" :stroke-width="1.75" />
              <p class="mt-2">Memuat…</p>
            </td>
          </tr>
          <tr v-else-if="staff.length === 0">
            <td colspan="6" class="px-4 py-12 text-center text-sm text-ink-500">
              <Users class="mx-auto h-6 w-6 text-ink-400" :stroke-width="1.5" />
              <p class="mt-2">
                {{ search || activeFilter ? 'Tidak ada staff cocok filter.' : 'Belum ada staff. Klik ' }}
                <strong v-if="!search && !activeFilter" class="text-ink-900">Invite staff</strong>
                {{ !search && !activeFilter ? ' untuk mulai.' : '' }}
              </p>
            </td>
          </tr>
          <tr v-for="s in staff" :key="s.id" :class="['hover:bg-canvas-alt/60', !s.is_active && 'opacity-60']">
            <td class="px-4 py-3">
              <p class="text-ink-900 font-medium">{{ s.name }}</p>
              <p v-if="s.email" class="mt-0.5 text-xs text-ink-500">{{ s.email }}</p>
            </td>
            <td class="px-4 py-3 hidden md:table-cell font-mono text-xs text-ink-700">{{ s.phone }}</td>
            <td class="px-4 py-3">
              <div v-if="s.roles && s.roles.length" class="flex flex-wrap gap-1">
                <span
                  v-for="r in s.roles"
                  :key="r.id"
                  class="inline-flex items-center rounded-full bg-gold-50 px-2 py-0.5 text-[11px] font-medium text-gold-900 ring-1 ring-inset ring-gold-200"
                >
                  {{ r.display_name }}
                </span>
              </div>
              <span v-else class="text-xs text-ink-400">— tanpa role</span>
            </td>
            <td class="px-4 py-3">
              <span
:class="[
                'inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset',
                s.is_active
                  ? 'bg-emerald-50 text-emerald-800 ring-emerald-200'
                  : 'bg-canvas-alt text-ink-500 ring-hairline',
              ]">
                {{ s.is_active ? 'Aktif' : 'Nonaktif' }}
              </span>
            </td>
            <td class="px-4 py-3 hidden lg:table-cell text-xs text-ink-500">{{ fmtDateTime(s.last_login_at) }}</td>
            <td class="px-4 py-3">
              <div class="flex justify-end gap-1">
                <button
                  v-if="canManage"
                  type="button"
                  class="inline-flex items-center rounded-md border border-hairline bg-canvas p-1.5 text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
                  aria-label="Edit"
                  @click="openEditModal(s)"
                >
                  <Pencil class="h-3.5 w-3.5" :stroke-width="1.75" />
                </button>
                <button
                  v-if="canManage"
                  type="button"
                  class="inline-flex items-center rounded-md border border-hairline bg-canvas p-1.5 text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
                  :aria-label="s.is_active ? 'Nonaktifkan' : 'Aktifkan'"
                  @click="toggleActive(s)"
                >
                  <Power class="h-3.5 w-3.5" :stroke-width="1.75" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Pagination -->
    <div v-if="total > 0" class="mt-4 flex items-center justify-between text-xs text-ink-500">
      <p>
        Halaman <strong class="text-ink-900">{{ page }}</strong> dari
        <strong class="text-ink-900">{{ totalPages }}</strong>
        · total <strong class="text-ink-900">{{ total }}</strong> staff
      </p>
      <div class="flex gap-1">
        <button
          type="button"
          :disabled="page <= 1"
          class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          @click="page--"
        >
          ← Sebelumnya
        </button>
        <button
          type="button"
          :disabled="page >= totalPages"
          class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          @click="page++"
        >
          Selanjutnya →
        </button>
      </div>
    </div>

    <!-- ============================ Invite modal ============================ -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="inviteModalOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="!inviteSaving && closeInviteModal()" />
          <div role="dialog" aria-modal="true" class="relative w-full max-w-lg rounded-lg bg-canvas shadow-xl ring-1 ring-black/5 max-h-[90vh] overflow-y-auto">
            <!-- After-create: invite URL panel -->
            <div v-if="inviteResult" class="p-6">
              <div class="flex items-start gap-3">
                <Check class="h-6 w-6 text-emerald-600 flex-none mt-0.5" :stroke-width="1.75" />
                <div class="flex-1">
                  <h3 class="font-serif text-lg font-semibold text-ink-950">Invite terkirim ke sistem</h3>
                  <p class="mt-1 text-sm text-ink-500 leading-relaxed">
                    Akun <strong class="text-ink-900">{{ inviteResult.staff.name }}</strong> sudah dibuat (belum aktif).
                    Forward link di bawah ini via WA / email — <strong class="text-brand-700">token ini cuma tampil sekali</strong>.
                  </p>
                </div>
              </div>

              <div class="mt-5 space-y-4">
                <div>
                  <label class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Invite URL</label>
                  <div class="mt-1 flex gap-2">
                    <code class="flex-1 rounded-md border border-hairline bg-canvas-alt px-3 py-2 text-xs font-mono text-ink-700 break-all">{{ inviteResult.invite_url }}</code>
                    <button
                      type="button"
                      class="inline-flex items-center gap-1 rounded-md border border-hairline bg-canvas px-3 py-2 text-xs font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
                      @click="copyToClipboard(inviteResult.invite_url, 'url')"
                    >
                      <Check v-if="copiedField === 'url'" class="h-3.5 w-3.5 text-emerald-600" :stroke-width="1.75" />
                      <Copy v-else class="h-3.5 w-3.5" :stroke-width="1.75" />
                      {{ copiedField === 'url' ? 'Disalin' : 'Salin' }}
                    </button>
                  </div>
                </div>
                <div>
                  <label class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Token (raw)</label>
                  <div class="mt-1 flex gap-2">
                    <code class="flex-1 rounded-md border border-hairline bg-canvas-alt px-3 py-2 text-xs font-mono text-ink-700 break-all">{{ inviteResult.invite_token }}</code>
                    <button
                      type="button"
                      class="inline-flex items-center gap-1 rounded-md border border-hairline bg-canvas px-3 py-2 text-xs font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
                      @click="copyToClipboard(inviteResult.invite_token, 'token')"
                    >
                      <Check v-if="copiedField === 'token'" class="h-3.5 w-3.5 text-emerald-600" :stroke-width="1.75" />
                      <Copy v-else class="h-3.5 w-3.5" :stroke-width="1.75" />
                      {{ copiedField === 'token' ? 'Disalin' : 'Salin' }}
                    </button>
                  </div>
                </div>
                <div class="rounded-md border border-gold-200 bg-gold-50 p-3 text-xs text-gold-900 leading-relaxed">
                  <strong>Setelah staff terima link:</strong> mereka buka URL → set password sendiri → akun otomatis aktif dan bisa login.
                </div>
              </div>

              <div class="mt-6 flex justify-end">
                <button
                  type="button"
                  class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 transition-colors"
                  @click="closeInviteModal"
                >
                  Selesai
                </button>
              </div>
            </div>

            <!-- Form -->
            <form v-else class="p-6" @submit.prevent="submitInvite">
              <h3 class="font-serif text-lg font-semibold text-ink-950">Invite staff baru</h3>
              <p class="mt-1 text-xs text-ink-500 leading-relaxed">
                Sistem generate link invite. Anda forward manual ke calon staff via WA / email.
              </p>

              <div class="mt-4 space-y-4">
                <div>
                  <label for="inv-name" class="block text-sm font-medium text-ink-900">Nama <span class="text-brand-500">*</span></label>
                  <input
                    id="inv-name"
                    v-model="inviteForm.name"
                    type="text"
                    required
                    placeholder="Dini Pratama"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                </div>
                <div>
                  <label for="inv-email" class="block text-sm font-medium text-ink-900">Email <span class="text-brand-500">*</span></label>
                  <input
                    id="inv-email"
                    v-model="inviteForm.email"
                    type="email"
                    required
                    placeholder="dini@rajakuprinting.id"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                </div>
                <div>
                  <label for="inv-phone" class="block text-sm font-medium text-ink-900">No. HP / WA <span class="text-brand-500">*</span></label>
                  <input
                    id="inv-phone"
                    v-model="inviteForm.phone"
                    type="tel"
                    required
                    inputmode="numeric"
                    placeholder="0812 3456 7890"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                  <p class="mt-1 text-xs text-ink-500">Auto-format ke <span class="font-mono">62xxx</span>.</p>
                </div>
                <div>
                  <div class="flex items-center gap-2">
                    <ShieldCheck class="h-3.5 w-3.5 text-ink-500" :stroke-width="1.75" />
                    <label class="text-sm font-medium text-ink-900">Role</label>
                  </div>
                  <p class="mt-0.5 text-xs text-ink-500 leading-relaxed">
                    Bisa lebih dari satu. Toggle nanti via menu Edit kalau perlu.
                  </p>
                  <div v-if="rolesLoading" class="mt-2 text-xs text-ink-500">
                    <Loader2 class="inline h-3 w-3 animate-spin mr-1" :stroke-width="1.75" />
                    Memuat role…
                  </div>
                  <div v-else class="mt-2 grid gap-2 sm:grid-cols-2">
                    <label
                      v-for="r in roles"
                      :key="r.id"
                      :class="[
                        'flex cursor-pointer items-start gap-2 rounded-md border p-2.5 text-xs transition-colors',
                        inviteForm.role_ids.includes(r.id)
                          ? 'border-brand-500 bg-brand-50/50 text-ink-950'
                          : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                      ]"
                    >
                      <input
                        v-model="inviteForm.role_ids"
                        type="checkbox"
                        :value="r.id"
                        class="mt-0.5 accent-brand-500"
                      >
                      <span class="flex-1">
                        <span class="block font-semibold text-ink-900">{{ r.display_name }}</span>
                        <span class="mt-0.5 block font-mono text-[10px] text-ink-500">{{ r.name }}</span>
                        <span v-if="r.description" class="mt-0.5 block text-ink-500">{{ r.description }}</span>
                      </span>
                    </label>
                  </div>
                </div>
              </div>

              <div class="mt-6 flex justify-end gap-2">
                <button
                  type="button"
                  class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50"
                  :disabled="inviteSaving"
                  @click="closeInviteModal"
                >
                  Batal
                </button>
                <button
                  type="submit"
                  :disabled="inviteSaving"
                  class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 transition-colors disabled:opacity-60"
                >
                  <Loader2 v-if="inviteSaving" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
                  Buat invite
                </button>
              </div>
            </form>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- ============================ Edit modal ============================ -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="editModalOpen && editingStaff" class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="!editSaving && (editModalOpen = false)" />
          <div role="dialog" aria-modal="true" class="relative w-full max-w-lg rounded-lg bg-canvas shadow-xl ring-1 ring-black/5 max-h-[90vh] overflow-y-auto">
            <form class="p-6" @submit.prevent="submitEdit">
              <div class="flex items-start justify-between">
                <div>
                  <h3 class="font-serif text-lg font-semibold text-ink-950">Edit staff</h3>
                  <p class="mt-1 text-xs text-ink-500">
                    ID <span class="font-mono">{{ editingStaff.id.slice(0, 8) }}…</span>
                    · dibuat {{ fmtDate(editingStaff.created_at) }}
                  </p>
                </div>
                <button
                  type="button"
                  class="rounded-md p-1 text-ink-500 hover:bg-canvas-alt hover:text-ink-900"
                  aria-label="Tutup"
                  @click="editModalOpen = false"
                >
                  <X class="h-4 w-4" :stroke-width="1.75" />
                </button>
              </div>

              <div class="mt-4 space-y-4">
                <div>
                  <label for="edit-name" class="block text-sm font-medium text-ink-900">Nama</label>
                  <input
                    id="edit-name"
                    v-model="editForm.name"
                    type="text"
                    required
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                </div>
                <div>
                  <label for="edit-email" class="block text-sm font-medium text-ink-900">Email</label>
                  <input
                    id="edit-email"
                    v-model="editForm.email"
                    type="email"
                    required
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                </div>
                <div>
                  <label for="edit-phone" class="block text-sm font-medium text-ink-900">No. HP / WA</label>
                  <input
                    id="edit-phone"
                    v-model="editForm.phone"
                    type="tel"
                    required
                    inputmode="numeric"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                </div>
                <div>
                  <div class="flex items-center gap-2">
                    <ShieldCheck class="h-3.5 w-3.5 text-ink-500" :stroke-width="1.75" />
                    <label class="text-sm font-medium text-ink-900">Role</label>
                  </div>
                  <div class="mt-2 grid gap-2 sm:grid-cols-2">
                    <label
                      v-for="r in roles"
                      :key="r.id"
                      :class="[
                        'flex cursor-pointer items-start gap-2 rounded-md border p-2.5 text-xs transition-colors',
                        editForm.role_ids.includes(r.id)
                          ? 'border-brand-500 bg-brand-50/50 text-ink-950'
                          : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                      ]"
                    >
                      <input
                        v-model="editForm.role_ids"
                        type="checkbox"
                        :value="r.id"
                        class="mt-0.5 accent-brand-500"
                      >
                      <span class="flex-1">
                        <span class="block font-semibold text-ink-900">{{ r.display_name }}</span>
                        <span class="mt-0.5 block font-mono text-[10px] text-ink-500">{{ r.name }}</span>
                      </span>
                    </label>
                  </div>
                </div>
              </div>

              <div class="mt-6 flex justify-end gap-2">
                <button
                  type="button"
                  class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50"
                  :disabled="editSaving"
                  @click="editModalOpen = false"
                >
                  Batal
                </button>
                <button
                  type="submit"
                  :disabled="editSaving"
                  class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 transition-colors disabled:opacity-60"
                >
                  <Loader2 v-if="editSaving" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
                  Simpan perubahan
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
