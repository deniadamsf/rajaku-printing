<script setup lang="ts">
/**
 * /login — Form login unified customer + staff (§10).
 *
 * Design: patuh CLAUDE.md §26 (Fraunces headline + Inter body, brand/ink/hairline,
 * Lucide icon — tanpa rose/slate/emoji).
 */
import { History, LayoutDashboard, LogIn, Loader2, PackageSearch } from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import { googleErrorMessage } from '~/composables/useGoogleAuth'

// Poin panel brand — semuanya perilaku sistem yang memang ada (§10 satu form
// untuk semua tipe user, §11 riwayat gabung lintas channel, §5 lacak resi).
const panelPoints = [
  { icon: PackageSearch, text: 'Lacak status semua pesanan Anda dari satu halaman akun.' },
  {
    icon: History,
    text: 'Riwayat order online dan pesanan di tempat tergabung dalam satu identitas pelanggan.',
  },
  {
    icon: LayoutDashboard,
    text: 'Staf memakai form login yang sama, lalu diarahkan ke panel sesuai perannya.',
  },
]

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
const router = useRouter()

const form = reactive({
  email: '',
  password: '',
})
const submitting = ref(false)
const errorMsg = ref('')

// Redirect balik dari /auth/google?... → /login?oauth_error=<slug> (gagal).
// Tampilkan pesan lalu bersihkan query supaya tidak nempel saat refresh/share.
onMounted(() => {
  const slug = route.query.oauth_error
  if (typeof slug === 'string' && slug) {
    errorMsg.value = googleErrorMessage(slug)
    const rest = { ...route.query }
    delete rest.oauth_error
    router.replace({ query: rest })
  }
})

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
  <AuthShell
    panel-title="Satu akun untuk pelanggan dan tim."
    :panel-points="panelPoints"
  >
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

    <div class="mt-8 space-y-4">
      <AlertMessage :message="errorMsg" variant="error" />

      <AuthGoogleLoginButton redirect="/akun" />

      <div class="flex items-center gap-3">
        <span class="h-px flex-1 bg-hairline" />
        <span class="text-xs text-ink-500">atau</span>
        <span class="h-px flex-1 bg-hairline" />
      </div>
    </div>

    <form class="mt-4 space-y-4" novalidate @submit.prevent="onSubmit">
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
          class="rounded-sm font-medium text-brand-500 transition-colors hover:text-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
        >
          Daftar
        </NuxtLink>
      </p>
    </form>
  </AuthShell>
</template>
