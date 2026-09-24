<script setup lang="ts">
/**
 * ProductMultiSelect — pilih beberapa produk dari katalog lewat checklist +
 * pencarian. Dipakai di form diskon (`/admin/diskon`, §28.9) untuk cakupan
 * `applies_to === 'selected'`.
 *
 * Produk nonaktif tetap ditampilkan (ditandai "Nonaktif", tidak disembunyikan)
 * — kalau sebuah diskon sudah menyimpan produk yang belakangan dinonaktifkan,
 * admin yang membuka form edit tetap harus melihat pilihan itu apa adanya,
 * bukan kehilangan datanya secara diam-diam.
 */
import { Search, PackageX } from '@lucide/vue'

interface PickableProduct {
  id: string
  name: string
  category?: string
  pricing_type?: string
  is_active?: boolean
}

const props = defineProps<{
  modelValue: string[]
  products: PickableProduct[]
  loading?: boolean
  disabled?: boolean
}>()

const emit = defineEmits<{ (e: 'update:modelValue', value: string[]): void }>()

const search = ref('')

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return props.products
  return props.products.filter((p) =>
    p.name.toLowerCase().includes(q) || (p.category ?? '').toLowerCase().includes(q),
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
        placeholder="Cari produk…"
        :disabled="disabled || loading"
        class="w-full bg-transparent text-sm text-ink-900 placeholder-ink-400 focus:outline-none disabled:cursor-not-allowed"
      >
      <span class="shrink-0 whitespace-nowrap text-xs font-medium text-ink-500">
        {{ modelValue.length }} dipilih
      </span>
    </div>

    <div class="max-h-56 overflow-y-auto p-1">
      <p v-if="loading" class="px-3 py-4 text-center text-xs text-ink-500">Memuat produk…</p>
      <div v-else-if="filtered.length === 0" class="flex flex-col items-center gap-1.5 px-3 py-6 text-center">
        <PackageX class="h-5 w-5 text-ink-300" :stroke-width="1.5" />
        <p class="text-xs text-ink-500">
          {{ products.length === 0 ? 'Belum ada produk di katalog.' : 'Tidak ada produk yang cocok dengan pencarian.' }}
        </p>
      </div>
      <label
        v-for="p in filtered"
        :key="p.id"
        :class="[
          'flex cursor-pointer items-center justify-between gap-2 rounded-md px-2 py-1.5 text-sm transition-colors',
          disabled ? 'cursor-not-allowed opacity-60' : 'hover:bg-canvas-alt',
        ]"
      >
        <span class="flex items-center gap-2">
          <input
            type="checkbox"
            :checked="isChecked(p.id)"
            :disabled="disabled"
            class="h-4 w-4 rounded border-hairline accent-brand-500"
            @change="toggle(p.id)"
          >
          <span class="text-ink-900">{{ p.name }}</span>
          <span v-if="p.pricing_type === 'per_m2'" class="rounded bg-sky-50 px-1.5 py-0.5 text-[10px] font-medium text-sky-700">m²</span>
          <span v-if="p.category" class="text-xs text-ink-500">· {{ p.category }}</span>
        </span>
        <span v-if="p.is_active === false" class="shrink-0 text-[10px] font-medium uppercase tracking-wide text-ink-400">Nonaktif</span>
      </label>
    </div>
  </div>
</template>
