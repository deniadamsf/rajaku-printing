<script setup lang="ts">
/**
 * OrderItemsTable — tabel baris produk sebuah order (§32 Order Multi-Item).
 * Dipakai bersama oleh detail order admin (`/admin/order/[resi]`) & detail
 * pesanan customer (`/akun/pesanan/[resi]`), supaya keduanya tampil konsisten.
 *
 * HANYA menampilkan kolom item (produk, bahan, ukuran, qty, harga satuan,
 * subtotal) — angka uang level order (Subtotal → Diskon → Ongkir → TOTAL,
 * §28.7) TETAP dirender terpisah oleh halaman pemanggil dari field `Order`
 * langsung, BUKAN dijumlahkan dari tabel ini (§32.2).
 *
 * Overflow horizontal sengaja `overflow-x-auto` (bukan kartu bertumpuk) —
 * beda dari form order publik (§18: kartu wajib di form INTERAKTIF di HP).
 * Ini tabel BACA-SAJA di admin/akun, konsisten dengan pola kolom
 * `hidden md:table-cell` yang sudah dipakai `AdminDataTable` di seluruh app.
 */
import type { OrderItem } from '~/types/order'

defineProps<{
  items: OrderItem[]
}>()

function fmtIDR(n: number): string {
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    maximumFractionDigits: 0,
  }).format(n)
}
</script>

<template>
  <div class="overflow-x-auto rounded-md border border-hairline">
    <table class="w-full min-w-[560px] text-sm">
      <thead>
        <tr class="border-b border-hairline bg-canvas-alt/60 text-left text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
          <th class="px-3 py-2 font-medium">Produk</th>
          <th class="px-3 py-2 font-medium">Bahan</th>
          <th class="px-3 py-2 font-medium">Ukuran</th>
          <th class="px-3 py-2 font-medium text-right">Qty</th>
          <th class="px-3 py-2 font-medium text-right">Harga satuan</th>
          <th class="px-3 py-2 font-medium text-right">Subtotal</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="it in items" :key="it.line_no" class="border-b border-hairline last:border-b-0">
          <td class="px-3 py-2.5 align-top text-ink-900">{{ it.product_name }}</td>
          <td class="px-3 py-2.5 align-top text-ink-700">{{ it.material_name }}</td>
          <td class="px-3 py-2.5 align-top font-mono text-xs text-ink-700">{{ it.width_cm }} × {{ it.height_cm }} cm</td>
          <td class="px-3 py-2.5 align-top text-right text-ink-700">{{ it.quantity }}</td>
          <td class="px-3 py-2.5 align-top text-right text-ink-700">{{ fmtIDR(it.unit_price) }}</td>
          <td class="px-3 py-2.5 align-top text-right font-medium text-ink-950">{{ fmtIDR(it.subtotal) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
