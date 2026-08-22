<script setup lang="ts">
/**
 * SiteHeader — navbar publik (§17: CTA "Order Banner" & akses login WAJIB
 * selalu terlihat di semua halaman publik, desktop maupun mobile).
 *
 * Kenapa dipisah dari `layouts/default.vue`: navbar ini punya state sendiri
 * (drawer mobile, status scroll) dan dipakai di semua halaman publik —
 * menaruhnya di layout membuat file layout menampung dua urusan sekaligus.
 *
 * Masalah yang diperbaiki dari versi sebelumnya:
 *  1. Di layar HP semuanya dijejalkan ke satu baris — wordmark, tombol Order,
 *     nama user, dan tombol Keluar — sehingga teksnya membungkus jadi dua
 *     baris dan navbar terlihat berantakan. Paling parah saat sudah login,
 *     karena dua elemen teks tambahan ikut masuk.
 *  2. Tautan Katalog/Showcase/Artikel di-`hidden sm:inline` — di HP menu itu
 *     TIDAK BISA dijangkau sama sekali, bukan cuma disembunyikan.
 *
 * Sekarang: HP hanya memuat wordmark + tombol Order ringkas + satu tombol
 * menu; sisanya masuk drawer. Desktop memuat tautan utama, dan identitas user
 * dipadatkan jadi avatar inisial + tombol keluar berikon (bukan dua potong
 * teks) supaya barisnya tetap lapang.
 *
 * Design: CLAUDE.md §26 — brand/gold/ink/canvas, Fraunces wordmark, ikon
 * Lucide monoline, transisi 150-250ms ease-out, focus ring wajib.
 */
import { ArrowRight, LogOut, Menu, PackageSearch, X } from '@lucide/vue'
import { AnimatePresence, motion } from 'motion-v'

const auth = useAuthStore()
const route = useRoute()
const prefersReduced = usePrefersReducedMotion()

const focusRing =
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas rounded-sm'

/** Menu utama — satu sumber untuk desktop & drawer, supaya tidak drift. */
const navLinks = [
  { label: 'Katalog', to: '/katalog' },
  { label: 'Showcase', to: '/showcase' },
  { label: 'Artikel', to: '/artikel' },
  { label: 'Tentang Kami', to: '/tentang-kami' },
] as const

function isActive(prefix: string) {
  return route.path === prefix || route.path.startsWith(prefix + '/')
}

const firstName = computed(() => auth.user?.name?.split(' ')[0] || 'Akun')
const initial = computed(() => (auth.user?.name?.trim()?.[0] || 'A').toUpperCase())

// --- Status scroll: navbar mengencang setelah halaman digulir -------------
// Di atas hero yang gelap, navbar tipis nyaris transparan terasa menyatu;
// begitu konten terang lewat di bawahnya, navbar butuh dasar yang lebih padat
// supaya teksnya tetap terbaca. Perubahan hanya warna/bayangan — tinggi
// navbar TIDAK ikut berubah supaya konten di bawahnya tidak bergeser (§26.6).
const scrolled = ref(false)
let ticking = false

function onScroll() {
  if (ticking) return
  ticking = true
  requestAnimationFrame(() => {
    scrolled.value = window.scrollY > 8
    ticking = false
  })
}

// --- Drawer mobile --------------------------------------------------------
const menuOpen = ref(false)

function closeMenu() {
  menuOpen.value = false
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') closeMenu()
}

// Kunci scroll badan halaman selama drawer terbuka — tanpa ini, menggulir di
// atas drawer malah menggeser halaman di belakangnya.
watch(menuOpen, (open) => {
  if (import.meta.server) return
  document.body.style.overflow = open ? 'hidden' : ''
})

// Pindah halaman harus menutup drawer; kalau tidak, drawer tetap menutupi
// halaman tujuan setelah tautan diklik.
watch(() => route.fullPath, closeMenu)

onMounted(() => {
  onScroll()
  window.addEventListener('scroll', onScroll, { passive: true })
  window.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', onScroll)
  window.removeEventListener('keydown', onKeydown)
  if (import.meta.client) document.body.style.overflow = ''
})

async function onLogout() {
  closeMenu()
  await auth.logout()
  await navigateTo('/')
}

const drawerTransition = computed(() =>
  prefersReduced.value
    ? { duration: 0 }
    : { duration: 0.28, ease: [0.22, 1, 0.36, 1] as const },
)
</script>

<template>
  <header
    :class="[
      'sticky top-0 z-40 border-b transition-colors duration-200 ease-out print:hidden',
      scrolled
        ? 'border-hairline bg-canvas/95 backdrop-blur supports-[backdrop-filter]:bg-canvas/80'
        : 'border-transparent bg-canvas/80 backdrop-blur',
    ]"
  >
    <div class="mx-auto flex h-14 max-w-6xl items-center gap-3 px-4">
      <!-- Wordmark: shrink-0 + nowrap, biar tidak pernah pecah dua baris -->
      <NuxtLink
        to="/"
        :class="['shrink-0 whitespace-nowrap font-serif text-lg tracking-tight text-ink-950', focusRing]"
      >
        Rajaku
        <span class="font-normal text-gold-500">Printing</span>
      </NuxtLink>

      <!-- Menu desktop -->
      <nav class="ml-4 hidden items-center gap-1 md:flex">
        <NuxtLink
          v-for="link in navLinks"
          :key="link.to"
          :to="link.to"
          :class="[
            'relative rounded-md px-3 py-2 text-sm font-medium transition-colors duration-200 ease-out',
            isActive(link.to) ? 'text-brand-500' : 'text-ink-700 hover:text-ink-950',
            focusRing,
          ]"
        >
          {{ link.label }}
          <!--
            Penanda halaman aktif: garis tipis di bawah label. Dipakai supaya
            status aktif tidak cuma dibedakan warna (pembeda warna saja gagal
            untuk mata yang sulit membedakan merah/abu).
          -->
          <span
            v-if="isActive(link.to)"
            class="absolute inset-x-3 -bottom-px h-px bg-brand-500"
            aria-hidden="true"
          />
        </NuxtLink>
      </nav>

      <div class="ml-auto flex shrink-0 items-center gap-2">
        <NuxtLink
          to="/lacak"
          :class="[
            'hidden items-center gap-1.5 rounded-md px-3 py-2 text-sm font-medium text-ink-700 transition-colors duration-200 ease-out hover:text-ink-950 md:inline-flex',
            focusRing,
          ]"
        >
          <PackageSearch class="h-4 w-4" :stroke-width="1.5" />
          Lacak Resi
        </NuxtLink>

        <!--
          CTA utama. Di HP labelnya dipendekkan jadi "Order" — "Order Banner"
          memakan lebar yang membuat baris navbar pecah di layar 360px.
        -->
        <NuxtLink
          to="/order"
          :class="[
            'inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-md bg-brand-500 px-3 py-2 text-sm font-semibold text-canvas transition-colors duration-200 ease-out hover:bg-brand-600 sm:px-4',
            focusRing,
          ]"
        >
          <span class="sm:hidden">Order</span>
          <span class="hidden sm:inline">Order Banner</span>
          <ArrowRight class="hidden h-4 w-4 sm:inline" :stroke-width="1.5" />
        </NuxtLink>

        <!-- Identitas user (desktop) — dipadatkan jadi avatar + ikon keluar -->
        <template v-if="auth.isAuthenticated">
          <NuxtLink
            :to="auth.homePath"
            :class="[
              'hidden items-center gap-2 rounded-md py-1 pl-1 pr-2 text-sm font-medium text-ink-700 transition-colors duration-200 ease-out hover:bg-canvas-alt hover:text-ink-950 md:inline-flex',
              focusRing,
            ]"
            :title="auth.user?.name || 'Akun saya'"
          >
            <span
              class="flex h-7 w-7 items-center justify-center rounded-full bg-ink-950 text-xs font-semibold text-canvas"
              aria-hidden="true"
            >{{ initial }}</span>
            <span class="max-w-[7rem] truncate">{{ firstName }}</span>
          </NuxtLink>
          <button
            type="button"
            :class="[
              'hidden h-9 w-9 items-center justify-center rounded-md text-ink-500 transition-colors duration-200 ease-out hover:bg-canvas-alt hover:text-brand-500 md:inline-flex',
              focusRing,
            ]"
            aria-label="Keluar"
            title="Keluar"
            @click="onLogout"
          >
            <LogOut class="h-4 w-4" :stroke-width="1.75" />
          </button>
        </template>
        <NuxtLink
          v-else
          to="/login"
          :class="[
            'hidden rounded-md px-3 py-2 text-sm font-medium text-ink-700 transition-colors duration-200 ease-out hover:text-ink-950 md:inline-block',
            focusRing,
          ]"
        >
          Login
        </NuxtLink>

        <!-- Tombol menu (HP & tablet) -->
        <button
          type="button"
          :class="[
            'inline-flex h-9 w-9 items-center justify-center rounded-md text-ink-700 transition-colors duration-200 ease-out hover:bg-canvas-alt hover:text-ink-950 md:hidden',
            focusRing,
          ]"
          :aria-expanded="menuOpen"
          aria-controls="site-menu"
          aria-label="Buka menu"
          @click="menuOpen = true"
        >
          <Menu class="h-5 w-5" :stroke-width="1.75" />
        </button>
      </div>
    </div>

    <!--
      Drawer mobile — WAJIB di-teleport ke <body>, jangan dibiarkan sebagai
      anak <header>. Header ini memakai `backdrop-blur`, dan backdrop-filter
      membuat elemen jadi containing block untuk keturunan `position: fixed`:
      drawer `inset-0` akan menempel ke kotak navbar setinggi 56px, bukan ke
      layar. Gejalanya halus — drawer "terbuka" tapi isinya terpotong jadi
      sepotong tipis di bawah navbar.
    -->
    <ClientOnly>
      <Teleport to="body">
      <AnimatePresence>
        <motion.div
          v-if="menuOpen"
          key="site-menu"
          class="fixed inset-0 z-50 md:hidden"
          :initial="{ opacity: 0 }"
          :animate="{ opacity: 1 }"
          :exit="{ opacity: 0 }"
          :transition="{ duration: prefersReduced ? 0 : 0.2 }"
        >
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="closeMenu" />

          <motion.div
            id="site-menu"
            class="absolute inset-y-0 right-0 flex w-[min(20rem,86vw)] flex-col bg-canvas shadow-xl ring-1 ring-black/5"
            role="dialog"
            aria-modal="true"
            aria-label="Menu navigasi"
            :initial="{ x: prefersReduced ? 0 : '100%' }"
            :animate="{ x: 0 }"
            :exit="{ x: prefersReduced ? 0 : '100%' }"
            :transition="drawerTransition"
          >
            <div class="flex h-14 shrink-0 items-center justify-between border-b border-hairline px-4">
              <span class="font-serif text-base tracking-tight text-ink-950">
                Rajaku <span class="font-normal text-gold-500">Printing</span>
              </span>
              <button
                type="button"
                :class="[
                  'inline-flex h-9 w-9 items-center justify-center rounded-md text-ink-500 transition-colors duration-200 ease-out hover:bg-canvas-alt hover:text-ink-950',
                  focusRing,
                ]"
                aria-label="Tutup menu"
                @click="closeMenu"
              >
                <X class="h-5 w-5" :stroke-width="1.75" />
              </button>
            </div>

            <nav class="min-h-0 flex-1 overflow-y-auto px-3 py-4">
              <NuxtLink
                v-for="link in navLinks"
                :key="link.to"
                :to="link.to"
                :class="[
                  'flex items-center rounded-md px-3 py-2.5 text-sm font-medium transition-colors duration-200 ease-out',
                  isActive(link.to)
                    ? 'bg-brand-50 text-brand-600'
                    : 'text-ink-800 hover:bg-canvas-alt hover:text-ink-950',
                  focusRing,
                ]"
              >
                {{ link.label }}
              </NuxtLink>

              <NuxtLink
                to="/lacak"
                :class="[
                  'mt-1 flex items-center gap-2 rounded-md px-3 py-2.5 text-sm font-medium transition-colors duration-200 ease-out',
                  isActive('/lacak')
                    ? 'bg-brand-50 text-brand-600'
                    : 'text-ink-800 hover:bg-canvas-alt hover:text-ink-950',
                  focusRing,
                ]"
              >
                <PackageSearch class="h-4 w-4" :stroke-width="1.5" />
                Lacak Resi
              </NuxtLink>

              <div class="mt-4 border-t border-hairline pt-4">
                <NuxtLink
                  to="/order"
                  :class="[
                    'flex items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-3 text-sm font-semibold text-canvas transition-colors duration-200 ease-out hover:bg-brand-600',
                    focusRing,
                  ]"
                >
                  Order Banner
                  <ArrowRight class="h-4 w-4" :stroke-width="1.5" />
                </NuxtLink>
              </div>
            </nav>

            <div class="shrink-0 border-t border-hairline p-3">
              <template v-if="auth.isAuthenticated">
                <NuxtLink
                  :to="auth.homePath"
                  :class="[
                    'flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium text-ink-800 transition-colors duration-200 ease-out hover:bg-canvas-alt',
                    focusRing,
                  ]"
                >
                  <span
                    class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-ink-950 text-xs font-semibold text-canvas"
                    aria-hidden="true"
                  >{{ initial }}</span>
                  <span class="min-w-0">
                    <span class="block truncate">{{ auth.user?.name || 'Akun saya' }}</span>
                    <span class="block text-xs font-normal text-ink-500">Buka akun saya</span>
                  </span>
                </NuxtLink>
                <button
                  type="button"
                  :class="[
                    'mt-1 flex w-full items-center gap-2 rounded-md px-3 py-2.5 text-sm font-medium text-ink-500 transition-colors duration-200 ease-out hover:bg-canvas-alt hover:text-brand-500',
                    focusRing,
                  ]"
                  @click="onLogout"
                >
                  <LogOut class="h-4 w-4" :stroke-width="1.75" />
                  Keluar
                </button>
              </template>
              <NuxtLink
                v-else
                to="/login"
                :class="[
                  'flex items-center justify-center rounded-md border border-hairline px-4 py-2.5 text-sm font-semibold text-ink-900 transition-colors duration-200 ease-out hover:bg-canvas-alt',
                  focusRing,
                ]"
              >
                Login
              </NuxtLink>
            </div>
          </motion.div>
        </motion.div>
      </AnimatePresence>
      </Teleport>
    </ClientOnly>
  </header>
</template>
