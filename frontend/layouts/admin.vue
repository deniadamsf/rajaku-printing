<script setup lang="ts">
/**
 * Admin layout — sidebar navigasi (permission-aware) + topbar dengan info user
 * & tombol logout. Dipakai semua halaman di /admin/*.
 *
 * definePageMeta({ layout: 'admin', middleware: ['staff-only'] }) di tiap page.
 *
 * Palet & ikon patuh CLAUDE.md §26 (brand crimson + ink warm neutral + Lucide
 * monoline). Sidebar pakai ink-950 (near-black warm) bukan slate-900 (biru-tint).
 *
 * Tinggi sidebar: aside = kolom flex setinggi layar (sticky di desktop, drawer
 * fixed di mobile), header brand `shrink-0`, nav `flex-1 min-h-0 overflow-y-auto`.
 * JANGAN kembali ke tinggi hitung manual (`h-[calc(100vh-3.5rem)]`) — angkanya
 * meleset begitu tinggi header berubah, dan 100vh di browser mobile menghitung
 * bar URL yang tersembunyi sehingga menu paling bawah terpotong.
 */
import { LayoutDashboard, Menu, X } from '@lucide/vue'

const auth = useAuthStore()
const route = useRoute()
const nav = useAdminNav()

const sidebarOpen = ref(false)

const groupLabels: Record<'operasional' | 'konten' | 'kelola', string> = {
  operasional: 'Operasional',
  konten: 'Konten',
  kelola: 'Kelola',
}

function isActive(to: string): boolean {
  // Cocokkan prefix: /admin/artikel/123 aktif untuk /admin/artikel.
  if (to === '/admin') return route.path === '/admin'
  return route.path === to || route.path.startsWith(to + '/')
}

async function onLogout() {
  await auth.logout()
  await navigateTo('/login')
}
</script>

<template>
  <div class="min-h-screen bg-canvas-alt text-ink-900 font-sans">
    <!-- Topbar mobile -->
    <header class="lg:hidden sticky top-0 z-30 flex items-center justify-between border-b border-hairline bg-canvas px-4 h-12 print:hidden">
      <button
        type="button"
        class="rounded-md p-2 text-ink-600 hover:bg-canvas-alt"
        aria-label="Toggle menu"
        @click="sidebarOpen = !sidebarOpen"
      >
        <Menu class="h-5 w-5" :stroke-width="1.75" />
      </button>
      <NuxtLink to="/admin" class="font-semibold text-sm tracking-tight">Admin · Rajaku</NuxtLink>
      <span class="w-9" />
    </header>

    <div class="flex">
      <!-- Sidebar -->
      <aside
        :class="[
          'fixed inset-y-0 left-0 z-40 flex w-64 shrink-0 flex-col bg-ink-950 text-ink-100 transform transition-transform lg:sticky lg:top-0 lg:h-dvh lg:translate-x-0 print:hidden',
          sidebarOpen ? 'translate-x-0' : '-translate-x-full',
        ]"
      >
        <div class="flex h-14 shrink-0 items-center justify-between px-5 border-b border-white/5">
          <NuxtLink
            to="/admin"
            class="font-serif text-base tracking-tight text-canvas"
            @click="sidebarOpen = false"
          >
            Rajaku
            <span class="text-gold-500 font-normal">· Admin</span>
          </NuxtLink>
          <button
            type="button"
            class="lg:hidden rounded-md p-1 text-ink-400 hover:text-ink-100"
            aria-label="Tutup menu"
            @click="sidebarOpen = false"
          >
            <X class="h-5 w-5" :stroke-width="1.75" />
          </button>
        </div>

        <nav class="nav-scroll min-h-0 flex-1 overflow-y-auto px-3 py-4 space-y-5">
          <div>
            <NuxtLink
              to="/admin"
              :class="[
                'flex items-center gap-3 rounded-md px-3 py-1.5 text-sm transition-colors',
                isActive('/admin')
                  ? 'bg-white/5 text-canvas'
                  : 'text-ink-300 hover:bg-white/5 hover:text-canvas',
              ]"
              @click="sidebarOpen = false"
            >
              <LayoutDashboard class="h-4 w-4 shrink-0" :stroke-width="1.75" />
              <span>Dashboard</span>
            </NuxtLink>
          </div>

          <div
            v-for="(items, group) in nav.grouped.value"
            :key="group"
            :class="items.length === 0 ? 'hidden' : ''"
          >
            <p class="px-3 mb-1 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
              {{ groupLabels[group as 'operasional' | 'konten' | 'kelola'] }}
            </p>
            <ul class="space-y-0.5">
              <li v-for="item in items" :key="item.to">
                <NuxtLink
                  :to="item.to"
                  :class="[
                    'flex items-center gap-3 rounded-md px-3 py-1.5 text-sm transition-colors',
                    isActive(item.to)
                      ? 'bg-white/5 text-canvas'
                      : 'text-ink-300 hover:bg-white/5 hover:text-canvas',
                  ]"
                  @click="sidebarOpen = false"
                >
                  <component :is="item.icon" class="h-4 w-4 shrink-0" :stroke-width="1.75" />
                  <span>{{ item.label }}</span>
                </NuxtLink>
              </li>
            </ul>
          </div>
        </nav>
      </aside>

      <!-- Overlay mobile -->
      <div
        v-if="sidebarOpen"
        class="fixed inset-0 z-30 bg-ink-950/50 lg:hidden"
        aria-hidden="true"
        @click="sidebarOpen = false"
      />

      <!-- Main -->
      <div class="flex-1 min-w-0 flex flex-col">
        <header class="hidden lg:flex sticky top-0 z-20 h-14 items-center justify-between bg-canvas border-b border-hairline px-8 print:hidden">
          <div class="text-sm text-ink-500">
            Halo, <span class="font-medium text-ink-900">{{ auth.user?.name }}</span>
            <span v-if="auth.roles.length" class="ml-2 text-xs text-ink-400">
              · {{ auth.roles.join(', ') }}
            </span>
          </div>
          <button
            type="button"
            class="text-sm font-medium text-ink-500 hover:text-brand-500 transition-colors"
            @click="onLogout"
          >
            Keluar
          </button>
        </header>

        <main class="flex-1 min-w-0 px-4 sm:px-6 lg:px-10 py-8 print:p-0">
          <slot />
        </main>
      </div>
    </div>
  </div>
</template>

<style scoped>
/*
 * Scrollbar nav sidebar dibuat tipis & lembut — scrollbar tebal bawaan Windows
 * (dengan tombol panah) merusak kesan minimalis sidebar (CLAUDE.md §26).
 *
 * Thumb-nya sengaja TIDAK disembunyikan total seperti sebelumnya: kalau daftar
 * menu kebetulan lebih tinggi dari layar (jendela pendek / zoom browser besar),
 * item paling bawah terpotong tanpa tanda apa pun — terbaca sebagai tampilan
 * rusak, bukan sebagai area yang bisa di-scroll. Samar saat diam, jelas saat
 * kursor di atas sidebar.
 */
.nav-scroll {
  scrollbar-width: thin;
  scrollbar-color: rgba(255, 255, 255, 0.1) transparent;
  transition: scrollbar-color 200ms ease-out;
}

.nav-scroll:hover,
.nav-scroll:focus-within {
  scrollbar-color: rgba(255, 255, 255, 0.24) transparent;
}

.nav-scroll::-webkit-scrollbar {
  width: 6px;
}

.nav-scroll::-webkit-scrollbar-track {
  background: transparent;
}

.nav-scroll::-webkit-scrollbar-thumb {
  background-color: rgba(255, 255, 255, 0.1);
  border-radius: 9999px;
  transition: background-color 200ms ease-out;
}

.nav-scroll:hover::-webkit-scrollbar-thumb,
.nav-scroll:focus-within::-webkit-scrollbar-thumb {
  background-color: rgba(255, 255, 255, 0.24);
}
</style>
