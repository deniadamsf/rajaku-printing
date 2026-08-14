<script setup lang="ts">
/**
 * Layout default — sticky navbar (§17): CTA "Order Banner" & "Login" wajib selalu
 * visible di semua halaman publik. Compliant dengan CLAUDE.md §26 (brand tokens
 * + Fraunces wordmark, tanpa rose/slate).
 */
const auth = useAuthStore()
const route = useRoute()

async function onLogout() {
  await auth.logout()
  await navigateTo('/')
}

function isActive(prefix: string) {
  return route.path === prefix || route.path.startsWith(prefix + '/')
}
</script>

<template>
  <div class="min-h-screen flex flex-col bg-canvas text-ink-900 font-sans">
    <header class="sticky top-0 z-40 bg-canvas/85 backdrop-blur border-b border-hairline">
      <div class="mx-auto max-w-6xl px-4 h-14 flex items-center justify-between">
        <NuxtLink to="/" class="font-serif text-lg tracking-tight text-ink-950">
          Rajaku
          <span class="text-gold-500 font-normal">Printing</span>
        </NuxtLink>

        <nav class="flex items-center gap-4 sm:gap-6">
          <NuxtLink
            to="/artikel"
            :class="[
              'hidden sm:inline text-sm font-medium transition-colors',
              isActive('/artikel') ? 'text-brand-500' : 'text-ink-700 hover:text-ink-950',
            ]"
          >
            Artikel
          </NuxtLink>

          <NuxtLink
            to="/order"
            class="inline-flex items-center rounded-md bg-brand-500 text-canvas px-4 py-2 text-sm font-semibold hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors"
          >
            Order Banner
          </NuxtLink>

          <template v-if="auth.isAuthenticated">
            <NuxtLink
              :to="auth.homePath"
              class="text-sm font-medium text-ink-700 hover:text-ink-950 transition-colors"
            >
              {{ auth.user?.name?.split(' ')[0] || 'Akun' }}
            </NuxtLink>
            <button
              type="button"
              class="text-sm font-medium text-ink-500 hover:text-brand-500 transition-colors"
              @click="onLogout"
            >
              Keluar
            </button>
          </template>
          <template v-else>
            <NuxtLink
              to="/login"
              class="text-sm font-medium text-ink-700 hover:text-ink-950 transition-colors"
            >
              Login
            </NuxtLink>
          </template>
        </nav>
      </div>
    </header>

    <main class="flex-1">
      <slot />
    </main>

    <footer class="border-t border-hairline py-8 mt-16">
      <div class="mx-auto max-w-6xl px-4 flex flex-col sm:flex-row items-center justify-between gap-4 text-xs text-ink-500">
        <p class="font-serif text-sm text-ink-700">
          Rajaku <span class="text-gold-500">Printing</span>
        </p>
        <nav class="flex items-center gap-5">
          <NuxtLink to="/artikel" class="hover:text-ink-900 transition-colors">Artikel</NuxtLink>
          <NuxtLink to="/order" class="hover:text-ink-900 transition-colors">Order Banner</NuxtLink>
          <NuxtLink to="/lacak" class="hover:text-ink-900 transition-colors">Lacak Resi</NuxtLink>
        </nav>
        <p>&copy; {{ new Date().getFullYear() }} Rajaku Printing</p>
      </div>
    </footer>
  </div>
</template>
