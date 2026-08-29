<script setup lang="ts" generic="T extends Record<string, unknown>">
/**
 * DataTable — reusable table shell. Kolom didefinisikan lewat prop, isi cell
 * via named slot `cell-<key>` untuk kustomisasi. Fallback: render item[col.key].
 *
 * Usage:
 *   <AdminDataTable :columns="cols" :items="rows" row-key="id">
 *     <template #cell-status="{ row }"><StatusBadge :status="row.status" /></template>
 *   </AdminDataTable>
 */

export interface DataTableColumn {
  key: string
  label: string
  /** Optional class tambahan untuk th & td (mis. w-32 text-right). */
  class?: string
}

const props = defineProps<{
  columns: DataTableColumn[]
  items: T[]
  loading?: boolean
  /** Field yang unik per row untuk :key. */
  rowKey: keyof T | string
  /** Pesan saat items kosong & tidak loading. */
  emptyMessage?: string
  /**
   * Opsional: seluruh baris jadi klikable (cursor pointer + emit `row-click`),
   * dipakai halaman yang navigasi ke detail dari mana pun di baris diklik
   * (mis. `/admin/pelanggan`) — bukan cuma satu sel jadi link. Sel yang punya
   * elemen interaktifnya sendiri (tombol/link) wajib `@click.stop` supaya
   * tidak ikut memicu navigasi baris.
   */
  rowClickable?: boolean
}>()

const emit = defineEmits<{
  (e: 'row-click', row: T): void
}>()

function rowKeyOf(row: T): string | number {
  const v = row[props.rowKey as keyof T]
  if (typeof v === 'string' || typeof v === 'number') return v
  return JSON.stringify(v)
}
</script>

<template>
  <div class="overflow-x-auto rounded-lg border border-hairline bg-canvas">
    <table class="min-w-full divide-y divide-hairline text-sm">
      <thead class="bg-canvas-alt">
        <tr>
          <th
            v-for="col in columns"
            :key="col.key"
            :class="['px-4 py-3 text-left text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500', col.class]"
          >
            {{ col.label }}
          </th>
        </tr>
      </thead>
      <tbody class="divide-y divide-hairline">
        <tr v-if="loading">
          <td :colspan="columns.length" class="px-4 py-8 text-center text-sm text-ink-500">
            Memuat…
          </td>
        </tr>
        <tr v-else-if="items.length === 0">
          <td :colspan="columns.length" class="px-4 py-8 text-center text-sm text-ink-500">
            {{ emptyMessage || 'Belum ada data.' }}
          </td>
        </tr>
        <tr
          v-for="row in items"
          v-else
          :key="rowKeyOf(row)"
          :class="['hover:bg-canvas-alt/60 transition-colors', rowClickable && 'cursor-pointer']"
          @click="rowClickable && emit('row-click', row)"
        >
          <td
            v-for="col in columns"
            :key="col.key"
            :class="['px-4 py-3 align-middle text-ink-900', col.class]"
          >
            <slot :name="`cell-${col.key}`" :row="row" :value="row[col.key]">
              {{ row[col.key] }}
            </slot>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
