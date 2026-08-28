<script setup lang="ts">
/**
 * /admin/membership — Kelola pengajuan member (CLAUDE.md §30.4).
 *
 * Layout: tab status + tabel + modal aksi (approve langsung, reject/revoke/
 * reinstate lewat modal beralasan wajib — pola sama dengan modal hapus di
 * `/admin/diskon`). Permission `membership.view` untuk lihat halaman,
 * `membership.manage` untuk tombol aksi (§30.5).
 */
import { ShieldAlert } from '@lucide/vue'
import type { DataTableColumn } from '~/components/admin/DataTable.vue'
import { ApiError } from '~/composables/useApi'
import type { Membership, MembershipStatus } from '~/types/membership'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Membership — Rajaku Admin' })

const auth = useAuthStore()
const membershipSvc = useMembership()

const canView = computed(() => auth.hasPermission('membership.view'))
const canManage = computed(() => auth.hasPermission('membership.manage'))

// -------------------- list state --------------------
const items = ref<Membership[]>([])
const total = ref(0)
const loading = ref(false)
const errorMsg = ref<string | null>(null)
const successMsg = ref<string | null>(null)

const statusFilter = ref<MembershipStatus | ''>('pending')
const page = ref(1)
const pageSize = 10

const statusTabs: Array<{ v: MembershipStatus | ''; label: string }> = [
  { v: 'pending', label: 'Pending' },
  { v: 'active', label: 'Aktif' },
  { v: 'rejected', label: 'Ditolak' },
  { v: 'revoked', label: 'Dicabut' },
  { v: '', label: 'Semua' },
]

function showSuccess(msg: string) {
  successMsg.value = msg
  setTimeout(() => (successMsg.value = null), 2500)
}
function toApiError(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

async function fetchList() {
  if (!canView.value) return
  loading.value = true
  errorMsg.value = null
  try {
    const res = await membershipSvc.list({
      status: statusFilter.value || undefined,
      page: page.value,
      per_page: pageSize,
    })
    items.value = res.items
    total.value = res.total
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal memuat daftar pengajuan membership')
  } finally {
    loading.value = false
  }
}

onMounted(fetchList)
watch([statusFilter, page], fetchList)

const columns: DataTableColumn[] = [
  { key: 'customer', label: 'Pelanggan' },
  { key: 'requested_at', label: 'Diajukan', class: 'w-40 hidden md:table-cell' },
  { key: 'decided_at', label: 'Diputuskan', class: 'w-40 hidden lg:table-cell' },
  { key: 'status', label: 'Status', class: 'w-32' },
  { key: 'actions', label: '', class: 'w-40 text-right' },
]

// -------------------- approve (langsung, tanpa alasan) --------------------
const approveTarget = ref<Membership | null>(null)
const approveBusy = ref(false)
const approveOpen = computed({
  get: () => approveTarget.value !== null,
  set: (v: boolean) => {
    if (!v) approveTarget.value = null
  },
})
const modalError = ref<string | null>(null)

function openApprove(m: Membership) {
  approveTarget.value = m
  modalError.value = null
}
async function confirmApprove() {
  if (!approveTarget.value) return
  approveBusy.value = true
  modalError.value = null
  try {
    await membershipSvc.approve(approveTarget.value.customer_id)
    showSuccess(`${approveTarget.value.name} disetujui jadi member.`)
    approveTarget.value = null
    await fetchList()
  } catch (e) {
    modalError.value = toApiError(e, 'Gagal menyetujui pengajuan')
  } finally {
    approveBusy.value = false
  }
}

// -------------------- reject/revoke/reinstate (alasan wajib) --------------------
type ReasonAction = 'reject' | 'revoke' | 'reinstate'
const reasonTarget = ref<Membership | null>(null)
const reasonAction = ref<ReasonAction>('reject')
const reasonText = ref('')
const reasonBusy = ref(false)
const reasonOpen = computed({
  get: () => reasonTarget.value !== null,
  set: (v: boolean) => {
    if (!v) reasonTarget.value = null
  },
})
const reasonCanSubmit = computed(() => reasonText.value.trim().length >= 3)

const reasonMeta: Record<ReasonAction, { title: string; confirmLabel: string; variant: 'default' | 'danger'; successVerb: string }> = {
  reject: { title: 'Tolak pengajuan ini?', confirmLabel: 'Tolak pengajuan', variant: 'danger', successVerb: 'ditolak' },
  revoke: { title: 'Cabut membership ini?', confirmLabel: 'Cabut membership', variant: 'danger', successVerb: 'dicabut' },
  reinstate: { title: 'Pulihkan membership ini?', confirmLabel: 'Pulihkan membership', variant: 'default', successVerb: 'dipulihkan' },
}

function openReason(m: Membership, action: ReasonAction) {
  reasonTarget.value = m
  reasonAction.value = action
  reasonText.value = ''
  modalError.value = null
}

async function confirmReason() {
  if (!reasonTarget.value) return
  if (!reasonCanSubmit.value) {
    modalError.value = 'Alasan minimal 3 karakter.'
    return
  }
  reasonBusy.value = true
  modalError.value = null
  try {
    const id = reasonTarget.value.customer_id
    const reason = reasonText.value.trim()
    if (reasonAction.value === 'reject') await membershipSvc.reject(id, reason)
    else if (reasonAction.value === 'revoke') await membershipSvc.revoke(id, reason)
    else await membershipSvc.reinstate(id, reason)
    showSuccess(`Membership ${reasonTarget.value.name} ${reasonMeta[reasonAction.value].successVerb}.`)
    reasonTarget.value = null
    await fetchList()
  } catch (e) {
    modalError.value = toApiError(e, 'Gagal memproses aksi')
  } finally {
    reasonBusy.value = false
  }
}

// -------------------- helpers --------------------
function fmtDate(s?: string | null): string {
  if (!s) return '—'
  try {
    return new Date(s).toLocaleString('id-ID', { day: '2-digit', month: 'short', year: '2-digit', hour: '2-digit', minute: '2-digit' })
  } catch {
    return s
  }
}
function statusLabel(s: MembershipStatus | string): string {
  const map: Record<string, string> = {
    none: 'Belum ajukan',
    pending: 'Pending',
    active: 'Aktif',
    rejected: 'Ditolak',
    revoked: 'Dicabut',
  }
  return map[s] ?? s
}
function statusTone(s: MembershipStatus | string): 'green' | 'amber' | 'ink' | 'rose' {
  if (s === 'active') return 'green'
  if (s === 'pending') return 'amber'
  if (s === 'rejected') return 'rose'
  return 'ink'
}
</script>

<template>
  <section>
    <AdminPageHeader
      title="Membership"
      subtitle="Tinjau pengajuan jadi member, approve/tolak, atau cabut membership yang disalahgunakan."
    />

    <div v-if="!canView" class="rounded-lg border border-hairline bg-canvas-alt p-6 flex items-start gap-3">
      <ShieldAlert class="h-5 w-5 text-ink-400 flex-none mt-0.5" :stroke-width="1.5" />
      <p class="text-sm text-ink-600 leading-relaxed">
        Anda tidak punya izin <span class="font-mono text-xs">membership.view</span> untuk melihat halaman ini.
        Hubungi super admin kalau ini keliru.
      </p>
    </div>

    <template v-else>
      <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />
      <AlertMessage v-if="successMsg" variant="success" :message="successMsg" class="mb-4" />

      <p v-if="!canManage" class="mb-4 text-xs text-ink-500">
        Anda tidak punya izin <span class="font-mono">membership.manage</span> — daftar hanya bisa dilihat, tombol aksi disembunyikan.
      </p>

      <!-- Filter tabs -->
      <div class="mb-5 flex flex-wrap rounded-md border border-hairline bg-canvas overflow-hidden text-sm w-fit">
        <button
          v-for="opt in statusTabs"
          :key="opt.v"
          type="button"
          :class="[
            'px-3 py-1.5 border-r border-hairline last:border-r-0 transition-colors',
            statusFilter === opt.v ? 'bg-canvas-alt font-medium text-ink-950' : 'text-ink-600 hover:bg-canvas-alt/60',
          ]"
          @click="statusFilter = opt.v; page = 1"
        >
          {{ opt.label }}
        </button>
      </div>

      <AdminDataTable
        :columns="columns"
        :items="items"
        :loading="loading"
        row-key="customer_id"
        empty-message="Tidak ada pengajuan pada filter ini."
      >
        <template #cell-customer="{ row }">
          <p class="text-sm font-medium text-ink-900">{{ (row as Membership).name }}</p>
          <p class="mt-0.5 font-mono text-xs text-ink-500">{{ (row as Membership).phone }}</p>
        </template>
        <template #cell-requested_at="{ row }">
          <span class="text-xs text-ink-500">{{ fmtDate((row as Membership).requested_at) }}</span>
        </template>
        <template #cell-decided_at="{ row }">
          <div>
            <span class="text-xs text-ink-500">{{ fmtDate((row as Membership).decided_at) }}</span>
            <p v-if="(row as Membership).decision_note" class="mt-0.5 max-w-[16rem] truncate text-xs text-ink-400" :title="(row as Membership).decision_note">
              {{ (row as Membership).decision_note }}
            </p>
          </div>
        </template>
        <template #cell-status="{ row }">
          <AdminStatusBadge :status="statusLabel((row as Membership).status)" :tone="statusTone((row as Membership).status)" />
        </template>
        <template #cell-actions="{ row }">
          <div v-if="canManage" class="flex justify-end gap-1.5">
            <button
              v-if="(row as Membership).status === 'pending'"
              type="button"
              class="rounded-md bg-brand-500 px-2.5 py-1 text-xs font-semibold text-canvas hover:bg-brand-600 transition-colors"
              @click="openApprove(row as Membership)"
            >
              Approve
            </button>
            <button
              v-if="(row as Membership).status === 'pending'"
              type="button"
              class="rounded-md border border-hairline bg-canvas px-2.5 py-1 text-xs font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
              @click="openReason(row as Membership, 'reject')"
            >
              Reject
            </button>
            <button
              v-if="(row as Membership).status === 'active'"
              type="button"
              class="rounded-md border border-brand-200 bg-canvas px-2.5 py-1 text-xs font-medium text-brand-700 hover:bg-brand-50 hover:border-brand-300 transition-colors"
              @click="openReason(row as Membership, 'revoke')"
            >
              Revoke
            </button>
            <button
              v-if="(row as Membership).status === 'revoked'"
              type="button"
              class="rounded-md border border-hairline bg-canvas px-2.5 py-1 text-xs font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
              @click="openReason(row as Membership, 'reinstate')"
            >
              Reinstate
            </button>
          </div>
        </template>
      </AdminDataTable>

      <AdminPagination
        v-if="total > 0"
        :page="page"
        :limit="pageSize"
        :total="total"
        @update:page="(v) => (page = v)"
      />
    </template>

    <!-- ============================ Approve modal ============================ -->
    <AdminConfirmDialog
      v-model:open="approveOpen"
      title="Setujui pengajuan ini?"
      confirm-label="Approve"
      :loading="approveBusy"
      :error="modalError"
      @confirm="confirmApprove"
    >
      <template v-if="approveTarget">
        <p class="mt-2 text-sm text-ink-500 leading-relaxed">
          <strong class="text-ink-900">{{ approveTarget.name }}</strong> ({{ approveTarget.phone }}) akan langsung
          berstatus member aktif dan bisa memakai diskon khusus member.
        </p>
      </template>
    </AdminConfirmDialog>

    <!-- ============================ Reject/Revoke/Reinstate modal ============================ -->
    <AdminConfirmDialog
      v-model:open="reasonOpen"
      :title="reasonMeta[reasonAction].title"
      :variant="reasonMeta[reasonAction].variant"
      :confirm-label="reasonMeta[reasonAction].confirmLabel"
      :loading="reasonBusy"
      :error="modalError"
      @confirm="confirmReason"
    >
      <template v-if="reasonTarget">
        <p class="mt-2 text-sm text-ink-500 leading-relaxed">
          <strong class="text-ink-900">{{ reasonTarget.name }}</strong> ({{ reasonTarget.phone }})
        </p>
        <div class="mt-3">
          <label for="membership-reason" class="block text-sm font-medium text-ink-900">Alasan <span class="text-brand-500">*</span></label>
          <textarea
            id="membership-reason"
            v-model="reasonText"
            rows="2"
            required
            placeholder="Contoh: nomor WA sudah tidak aktif, terverifikasi penyalahgunaan diskon"
            class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          />
        </div>
      </template>
    </AdminConfirmDialog>
  </section>
</template>
