/**
 * useAdminNav — sumber tunggal daftar menu admin panel.
 *
 * Setiap item punya `permission` (opsional). Item ditampilkan hanya kalau
 * user punya permission tsb — sesuai section 10 (menu toggle per role).
 * Item tanpa `permission` selalu tampil untuk semua staff (mis. Dashboard).
 *
 * Icon = komponen Lucide (monoline, 1 warna — sesuai CLAUDE.md §26.2).
 * Dilarang pakai emoji.
 */
import type { Component } from 'vue'
import {
  CreditCard,
  Package,
  Printer,
  Palette,
  ReceiptText,
  FileText,
  FolderTree,
  Users,
  ShieldCheck,
  Sparkles,
  SlidersHorizontal,
  Images,
  QrCode,
  TicketPercent,
  ClipboardList,
  UserCheck,
  Contact,
} from '@lucide/vue'

export interface AdminNavItem {
  /** Label ditampilkan di sidebar & dashboard card. */
  label: string
  /** Path Nuxt (leading slash). */
  to: string
  /** Deskripsi singkat — dipakai di dashboard card. */
  description: string
  /** Icon component Lucide (monoline stroke 1.5). */
  icon: Component
  /**
   * Permission code (satu). Kosong = semua staff. Cek pakai
   * useAuthStore().hasPermission(item.permission).
   */
  permission?: string
  /** Group heading untuk sidebar; item dalam grup sama ditampilkan bertingkat. */
  group: 'operasional' | 'konten' | 'kelola'
}

export function useAdminNav() {
  const items: AdminNavItem[] = [
    // Operasional harian
    {
      label: 'Verifikasi Pembayaran',
      to: '/admin/pembayaran',
      description: 'Approve/tolak bukti transfer customer',
      icon: CreditCard,
      permission: 'payment.verify',
      group: 'operasional',
    },
    {
      label: 'Order',
      to: '/admin/order',
      description: 'Lihat & kelola pesanan',
      icon: Package,
      permission: 'order.view',
      group: 'operasional',
    },
    {
      label: 'Produksi',
      to: '/admin/produksi',
      description: 'Update status cetak/QC',
      icon: Printer,
      permission: 'production.update',
      group: 'operasional',
    },
    {
      label: 'Desain',
      to: '/admin/desain',
      description: 'Approve upload / kerjakan request',
      icon: Palette,
      permission: 'design.work',
      group: 'operasional',
    },
    {
      label: 'POS / Kasir',
      to: '/admin/pos',
      description: 'Buat order walk-in',
      icon: ReceiptText,
      permission: 'pos.create_order',
      group: 'operasional',
    },

    // Konten
    {
      label: 'Artikel',
      to: '/admin/artikel',
      description: 'CMS artikel SEO (auto-WebP)',
      icon: FileText,
      permission: 'article.view',
      group: 'konten',
    },

    // Kelola
    {
      label: 'Katalog',
      to: '/admin/katalog',
      description: 'Produk, bahan, aturan harga',
      icon: FolderTree,
      permission: 'catalog.manage',
      group: 'kelola',
    },
    {
      label: 'Diskon',
      to: '/admin/diskon',
      description: 'Kelola promo & potongan harga',
      icon: TicketPercent,
      permission: 'discount.manage',
      group: 'kelola',
    },
    {
      label: 'Rekap Order',
      to: '/admin/rekap',
      description: 'Laporan penjualan & diskon',
      icon: ClipboardList,
      permission: 'report.view',
      group: 'kelola',
    },
    {
      label: 'Membership',
      to: '/admin/membership',
      description: 'Approve/tolak/cabut pengajuan member',
      icon: UserCheck,
      permission: 'membership.view',
      group: 'kelola',
    },
    {
      label: 'Pelanggan',
      to: '/admin/pelanggan',
      description: 'Data pelanggan, riwayat order, blokir akun',
      icon: Contact,
      permission: 'customer.view',
      group: 'kelola',
    },
    {
      label: 'Staff',
      to: '/admin/staff',
      description: 'Invite & assign role staff',
      icon: Users,
      permission: 'staff.manage',
      group: 'kelola',
    },
    {
      label: 'Role & Permission',
      to: '/admin/role',
      description: 'Toggle menu per role',
      icon: ShieldCheck,
      permission: 'role.manage',
      group: 'kelola',
    },
    {
      label: 'Pengaturan',
      to: '/admin/pengaturan',
      description: 'Kebijakan global, mis. retensi file desain',
      icon: SlidersHorizontal,
      permission: 'settings.manage',
      group: 'kelola',
    },
    {
      label: 'Media Landing Page',
      to: '/admin/site-media',
      description: 'Ganti gambar hero, logo, proses tanpa deploy',
      icon: Images,
      permission: 'sitemedia.manage',
      group: 'kelola',
    },
    {
      label: 'Pairing WhatsApp',
      to: '/admin/whatsapp',
      description: 'Tautkan nomor WA toko lewat QR, tanpa SSH',
      icon: QrCode,
      permission: 'notification.manage',
      group: 'kelola',
    },
    {
      // Living reference — CLAUDE.md §26.11. Tanpa permission = semua staff bisa lihat
      // (internal design reference, bukan operational tool).
      label: 'Design System',
      to: '/admin/design-system',
      description: 'Palet, tipografi, komponen — CLAUDE.md §26',
      icon: Sparkles,
      group: 'kelola',
    },
  ]

  const auth = useAuthStore()

  /** Items yang boleh diakses user aktif (permission-checked). */
  const visibleItems = computed<AdminNavItem[]>(() =>
    items.filter((it) => !it.permission || auth.hasPermission(it.permission)),
  )

  /** Group by heading — dipakai sidebar. Group kosong tidak ditampilkan. */
  const grouped = computed(() => {
    const buckets: Record<AdminNavItem['group'], AdminNavItem[]> = {
      operasional: [],
      konten: [],
      kelola: [],
    }
    for (const it of visibleItems.value) {
      buckets[it.group].push(it)
    }
    return buckets
  })

  return { items, visibleItems, grouped }
}
