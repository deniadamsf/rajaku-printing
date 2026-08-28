<script setup lang="ts">
/**
 * MemberMultiSelect — pilih beberapa member aktif lewat checklist + pencarian.
 * Dipakai form diskon (`/admin/diskon`, §30.3) untuk cakupan
 * `member_scope === 'selected_members'`. Pola sama dengan
 * `AdminProductMultiSelect` (§28.9) — checklist + search client-side di atas
 * daftar yang sudah di-fetch.
 *
 * Sumber datanya sengaja daftar member `active` (bukan pencarian semua
 * customer) — diskon `audience_scope='member'` hanya pernah berlaku untuk
 * customer berstatus `active` (§30.3, `ErrDiscountMembershipRequired`), jadi
 * memilih dari non-member tidak ada gunanya.
 */
import { Search, UserX } from '@lucide/vue'

interface PickableMember {
  id: string
  name: string
  phone: string
}

const props = defineProps<{
  modelValue: string[]
  members: PickableMember[]
  loading?: boolean
  disabled?: boolean
}>()

const emit = defineEmits<{ (e: 'update:modelValue', value: string[]): void }>()

const search = ref('')

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return props.members
  return props.members.filter((m) =>
    m.name.toLowerCase().includes(q) || m.phone.toLowerCase().includes(q),
  )
})

function isChecked(id: string): boolean {
  return props.modelValue.includes(id)
}

function toggle(id: string) {
  if (props.disabled) return
  const next = isChecked(id)
    ? props.modelValue.filter((x) => x !== id)
    : [...props.modelValue, id]
  emit('update:modelValue', next)
}
</script>

<template>
  <div class="rounded-md border border-hairline bg-canvas">
    <div class="flex items-center gap-2 border-b border-hairline px-3 py-2">
      <Search class="h-3.5 w-3.5 shrink-0 text-ink-400" :stroke-width="1.75" />
      <input
        v-model="search"
        type="search"
        placeholder="Cari member (nama/WA)…"
        :disabled="disabled || loading"
        class="w-full bg-transparent text-sm text-ink-900 placeholder-ink-400 focus:outline-none disabled:cursor-not-allowed"
      >
      <span class="shrink-0 whitespace-nowrap text-xs font-medium text-ink-500">
        {{ modelValue.length }} dipilih
      </span>
    </div>

    <div class="max-h-56 overflow-y-auto p-1">
      <p v-if="loading" class="px-3 py-4 text-center text-xs text-ink-500">Memuat member…</p>
      <div v-else-if="filtered.length === 0" class="flex flex-col items-center gap-1.5 px-3 py-6 text-center">
        <UserX class="h-5 w-5 text-ink-300" :stroke-width="1.5" />
        <p class="text-xs text-ink-500">
          {{ members.length === 0 ? 'Belum ada member aktif.' : 'Tidak ada member yang cocok dengan pencarian.' }}
        </p>
      </div>
      <label
        v-for="m in filtered"
        :key="m.id"
        :class="[
          'flex cursor-pointer items-center justify-between gap-2 rounded-md px-2 py-1.5 text-sm transition-colors',
          disabled ? 'cursor-not-allowed opacity-60' : 'hover:bg-canvas-alt',
        ]"
      >
        <span class="flex items-center gap-2">
          <input
            type="checkbox"
            :checked="isChecked(m.id)"
            :disabled="disabled"
            class="h-4 w-4 rounded border-hairline accent-brand-500"
            @change="toggle(m.id)"
          >
          <span class="text-ink-900">{{ m.name }}</span>
          <span class="font-mono text-xs text-ink-500">{{ m.phone }}</span>
        </span>
      </label>
    </div>
  </div>
</template>
