<script setup lang="ts">
/**
 * /register — Pendaftaran akun customer (§10: publik hanya bisa daftar sebagai
 * customer; akun staff dibuat super admin dari admin panel).
 *
 * Design: patuh CLAUDE.md §26 (Fraunces headline + Inter body, brand/ink/hairline,
 * Lucide icon — tanpa rose/slate/emoji).
 */
import { UserPlus, Loader2 } from '@lucide/vue'
import { ApiError } from '~/composables/useApi'

definePageMeta({
  middleware: ['guest'],
  layout: 'default',
})

useSeoMeta({
  title: 'Daftar Akun Customer',
  description: 'Daftar akun customer untuk order banner & lacak history pesanan.',
})

const auth = useAuthStore()

const form = reactive({
  name: '',
  email: '',
  phone: '',
  password: '',
})
const submitting = ref(false)
const errorMsg = ref('')

async function onSubmit() {
  if (submitting.value) return
  errorMsg.value = ''
  submitting.value = true
  try {
    const userType = await auth.register({
      email: form.email.trim(),
      phone: form.phone.trim(),
      name: form.name.trim(),
      password: form.password,
    })
    // Public register hanya untuk customer, tapi tetap route defensif berdasarkan
    // userType return value (bukan getter homePath yang bisa race dgn reactivity).
    await navigateTo(userType === 'staff' ? '/admin' : '/akun')
  } catch (e) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal mendaftar, coba lagi.'
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
        Daftar Akun
      </h1>
      <p class="mt-2 text-sm text-ink-500 leading-relaxed">
        Daftar untuk lacak order & simpan history pesanan.
      </p>
    </div>

    <form class="mt-8 space-y-4" novalidate @submit.prevent="onSubmit">
      <AlertMessage :message="errorMsg" variant="error" />

      <BaseInput
        id="name"
        v-model="form.name"
        label="Nama lengkap"
        autocomplete="name"
        required
        :disabled="submitting"
      />
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
        id="phone"
        v-model="form.phone"
        label="Nomor WhatsApp"
        type="tel"
        autocomplete="tel"
        placeholder="08xxxxxxxxxx"
        helper="Otomatis dikonversi ke format 62xxx."
        required
        :disabled="submitting"
      />
      <BaseInput
        id="password"
        v-model="form.password"
        label="Password"
        type="password"
        autocomplete="new-password"
        helper="Minimal 8 karakter."
        required
        :disabled="submitting"
      />

      <button
        type="submit"
        :disabled="submitting"
        class="inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-60"
      >
        <Loader2 v-if="submitting" class="h-4 w-4 animate-spin" :stroke-width="1.5" />
        <UserPlus v-else class="h-4 w-4" :stroke-width="1.5" />
        <span>{{ submitting ? 'Memproses…' : 'Daftar' }}</span>
      </button>

      <p class="text-center text-sm text-ink-500">
        Sudah punya akun?
        <NuxtLink
          to="/login"
          class="font-medium text-brand-500 transition-colors hover:text-brand-600"
        >
          Login
        </NuxtLink>
      </p>
    </form>
  </section>
</template>
