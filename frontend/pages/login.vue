<script setup lang="ts">
/**
 * /login — Form login unified customer + staff (§10).
 *
 * Design: patuh CLAUDE.md §26 (Fraunces headline + Inter body, brand/ink/hairline,
 * Lucide icon — tanpa rose/slate/emoji).
 */
import { LogIn, Loader2 } from '@lucide/vue'
import { ApiError } from '~/composables/useApi'

definePageMeta({
  middleware: ['guest'],
  layout: 'default',
})

useSeoMeta({
  title: 'Login',
  description: 'Login ke Rajaku Printing untuk lacak order & kelola pesanan.',
})

const auth = useAuthStore()
const route = useRoute()

const form = reactive({
  email: '',
  password: '',
})
const submitting = ref(false)
const errorMsg = ref('')

async function onSubmit() {
  if (submitting.value) return
  errorMsg.value = ''
  submitting.value = true
  try {
    const userType = await auth.login(form.email.trim(), form.password)
    // Prioritas redirect: query `?redirect=` → home path berdasarkan user_type.
    // NOTE: pakai userType return langsung (bukan getter homePath) supaya
    // navigateTo tidak race dengan reactive getter yang mungkin belum ter-update.
    const fallback = userType === 'staff' ? '/admin' : '/akun'
    const target = (route.query.redirect as string) || fallback
    await navigateTo(target)
  } catch (e) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal login, coba lagi.'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <section class="mx-auto max-w-md px-4 py-16 md:py-24">
    <div class="text-center">
      <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
        Rajaku Printing
      </p>
      <h1 class="mt-2 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
        Login
      </h1>
      <p class="mt-2 text-sm text-ink-500 leading-relaxed">
        Login untuk customer & staff pakai form yang sama.
      </p>
    </div>

    <form class="mt-8 space-y-4" novalidate @submit.prevent="onSubmit">
      <AlertMessage :message="errorMsg" variant="error" />

      <BaseInput
        id="email"
        v-model="form.email"
        label="Email"
        type="email"
        autocomplete="email"
        required
        :disabled="submitting"
      />
      <BaseInput
        id="password"
        v-model="form.password"
        label="Password"
        type="password"
        autocomplete="current-password"
        required
        :disabled="submitting"
      />

      <button
        type="submit"
        :disabled="submitting"
        class="inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-60"
      >
        <Loader2 v-if="submitting" class="h-4 w-4 animate-spin" :stroke-width="1.5" />
        <LogIn v-else class="h-4 w-4" :stroke-width="1.5" />
        <span>{{ submitting ? 'Memproses…' : 'Login' }}</span>
      </button>

      <p class="text-center text-sm text-ink-500">
        Belum punya akun?
        <NuxtLink
          to="/register"
          class="font-medium text-brand-500 transition-colors hover:text-brand-600"
        >
          Daftar
        </NuxtLink>
      </p>
    </form>
  </section>
</template>
