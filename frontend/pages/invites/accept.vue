<script setup lang="ts">
/**
 * /invites/accept — Publik. Calon staff yang di-invite super admin klik link
 * di sini → verify token → set password → akun aktif → auto-login redirect
 * ke admin dashboard.
 *
 * Alur error state:
 *   - No token → CTA balik ke /
 *   - Verify failed (invalid/expired/used) → panel error dgn instruksi
 *   - Accept failed → inline error di form
 *
 * Design: patuh CLAUDE.md §26 (Fraunces + Inter + Lucide, brand/ink/hairline).
 */
import { ShieldCheck, Loader2, Check, AlertTriangle, Eye, EyeOff } from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import type { InviteVerification } from '~/composables/useInvite'

definePageMeta({
  // Publik — tidak wajib login. User yg sudah login (customer/staff) tetap
  // boleh akses; setelah accept berhasil akan re-hydrate ke akun yg baru.
  layout: 'default',
})

useSeoMeta({
  title: 'Aktifkan akun — Rajaku Printing',
  robots: 'noindex,nofollow',
})

const route = useRoute()
const invite = useInvite()
const auth = useAuthStore()

const rawToken = computed(() => {
  const t = route.query.token
  return typeof t === 'string' ? t : ''
})

// -------------------- verify token on mount --------------------
const verifyState = ref<'idle' | 'loading' | 'ok' | 'error'>('idle')
const verifyError = ref<string | null>(null)
const verification = ref<InviteVerification | null>(null)

async function runVerify() {
  if (!rawToken.value) {
    verifyState.value = 'error'
    verifyError.value = 'Link tidak lengkap — token invite tidak ada di URL.'
    return
  }
  verifyState.value = 'loading'
  verifyError.value = null
  try {
    verification.value = await invite.verify(rawToken.value)
    verifyState.value = 'ok'
  } catch (e) {
    verifyState.value = 'error'
    verifyError.value = e instanceof ApiError ? e.message : 'Verifikasi link gagal.'
  }
}

onMounted(runVerify)

// -------------------- accept form --------------------
const password = ref('')
const confirmPassword = ref('')
const showPassword = ref(false)
const submitting = ref(false)
const submitError = ref<string | null>(null)
const successful = ref(false)

const passwordLength = computed(() => password.value.length)
const passwordTooShort = computed(() => passwordLength.value > 0 && passwordLength.value < 8)
const passwordTooLong = computed(() => passwordLength.value > 72)
const passwordMismatch = computed(
  () => confirmPassword.value.length > 0 && confirmPassword.value !== password.value,
)
const canSubmit = computed(() => {
  if (submitting.value) return false
  if (verifyState.value !== 'ok') return false
  if (passwordLength.value < 8 || passwordLength.value > 72) return false
  if (confirmPassword.value !== password.value) return false
  return true
})

async function onSubmit() {
  if (!canSubmit.value) return
  submitting.value = true
  submitError.value = null
  try {
    await invite.accept(rawToken.value, password.value)
    successful.value = true
    // Auto-login pakai email + password yang baru di-set → hydrate store → arahkan ke /admin.
    if (verification.value?.email) {
      try {
        await auth.login(verification.value.email, password.value)
        await navigateTo(auth.homePath)
        return
      } catch {
        // Kalau auto-login gagal karena race dgn user setup, jangan blocking
        // — tampilkan sukses & suruh manual login.
      }
    }
  } catch (e) {
    submitError.value = e instanceof ApiError ? e.message : 'Gagal aktivasi akun.'
  } finally {
    submitting.value = false
  }
}

// -------------------- helpers --------------------
function fmtExpires(iso?: string): string {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleString('id-ID', {
      day: '2-digit',
      month: 'short',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return iso
  }
}
</script>

<template>
  <main class="mx-auto max-w-md px-4 py-16 md:py-24">
    <!-- Header eyebrow -->
    <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2 flex items-center gap-2">
      <ShieldCheck class="h-3.5 w-3.5" :stroke-width="1.75" />
      Aktivasi Akun Staff
    </p>
    <h1 class="font-serif text-2xl md:text-3xl font-semibold tracking-tight text-ink-950">
      Set password Anda
    </h1>

    <!-- Verify loading -->
    <div v-if="verifyState === 'loading'" class="mt-8 rounded-lg border border-hairline bg-canvas p-8 text-center">
      <Loader2 class="mx-auto h-5 w-5 animate-spin text-ink-500" :stroke-width="1.75" />
      <p class="mt-3 text-sm text-ink-500">Memverifikasi link invite…</p>
    </div>

    <!-- Verify error -->
    <div v-else-if="verifyState === 'error'" class="mt-8 rounded-lg border border-brand-200 bg-brand-50/50 p-6">
      <div class="flex items-start gap-3">
        <AlertTriangle class="h-5 w-5 text-brand-700 flex-none mt-0.5" :stroke-width="1.75" />
        <div class="flex-1">
          <h2 class="font-serif text-lg font-semibold text-ink-950">Link tidak valid</h2>
          <p class="mt-1 text-sm text-ink-700 leading-relaxed">{{ verifyError }}</p>
          <p class="mt-2 text-xs text-ink-500 leading-relaxed">
            Kemungkinan: token salah salin, sudah kadaluarsa (>24 jam), atau sudah dipakai.
            Minta super admin issue invite baru.
          </p>
          <div class="mt-4 flex gap-2">
            <NuxtLink
              to="/"
              class="inline-flex items-center rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
            >
              Kembali ke Beranda
            </NuxtLink>
            <NuxtLink
              to="/login"
              class="inline-flex items-center rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 transition-colors"
            >
              Login (kalau sudah aktif)
            </NuxtLink>
          </div>
        </div>
      </div>
    </div>

    <!-- Success (after accept, before auto-login redirect) -->
    <div v-else-if="successful" class="mt-8 rounded-lg border border-emerald-200 bg-emerald-50 p-6">
      <div class="flex items-start gap-3">
        <Check class="h-5 w-5 text-emerald-600 flex-none mt-0.5" :stroke-width="1.75" />
        <div class="flex-1">
          <h2 class="font-serif text-lg font-semibold text-emerald-900">Akun aktif</h2>
          <p class="mt-1 text-sm text-emerald-800 leading-relaxed">
            Password Anda tersimpan. Mengalihkan ke dashboard admin…
          </p>
          <NuxtLink
            to="/login"
            class="mt-3 inline-flex items-center gap-2 text-sm font-semibold text-emerald-900 hover:underline"
          >
            Kalau tidak dialihkan otomatis, klik di sini →
          </NuxtLink>
        </div>
      </div>
    </div>

    <!-- Verified + form -->
    <form v-else-if="verifyState === 'ok' && verification" class="mt-8 space-y-6" @submit.prevent="onSubmit">
      <!-- User info recap -->
      <div class="rounded-lg border border-hairline bg-canvas p-5">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Anda akan mengaktifkan</p>
        <p class="mt-1 font-serif text-lg font-semibold text-ink-950">{{ verification.name }}</p>
        <p class="mt-0.5 text-sm text-ink-700">{{ verification.email }}</p>
        <p class="mt-2 text-xs text-ink-500">
          Link ini valid sampai <strong class="text-ink-900">{{ fmtExpires(verification.expires_at) }}</strong>.
        </p>
      </div>

      <AlertMessage v-if="submitError" variant="error" :message="submitError" />

      <!-- Password inputs -->
      <div class="space-y-4">
        <div>
          <label for="inv-pass" class="block text-sm font-medium text-ink-900">
            Password baru <span class="text-brand-500">*</span>
          </label>
          <div class="mt-1 relative">
            <input
              id="inv-pass"
              v-model="password"
              :type="showPassword ? 'text' : 'password'"
              required
              minlength="8"
              maxlength="72"
              autocomplete="new-password"
              class="block w-full rounded-md border border-hairline bg-canvas pr-10 pl-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              :class="{ 'border-brand-500': passwordTooShort || passwordTooLong }"
            >
            <button
              type="button"
              class="absolute inset-y-0 right-0 flex items-center px-2 text-ink-500 hover:text-ink-900 transition-colors"
              :aria-label="showPassword ? 'Sembunyikan password' : 'Tampilkan password'"
              @click="showPassword = !showPassword"
            >
              <EyeOff v-if="showPassword" class="h-4 w-4" :stroke-width="1.75" />
              <Eye v-else class="h-4 w-4" :stroke-width="1.75" />
            </button>
          </div>
          <p class="mt-1 text-xs" :class="passwordTooShort || passwordTooLong ? 'text-brand-700' : 'text-ink-500'">
            <template v-if="passwordTooShort">
              Minimal 8 karakter (baru {{ passwordLength }}).
            </template>
            <template v-else-if="passwordTooLong">
              Maksimum 72 karakter.
            </template>
            <template v-else>
              8–72 karakter. Kombinasi huruf + angka + simbol lebih aman.
            </template>
          </p>
        </div>

        <div>
          <label for="inv-pass2" class="block text-sm font-medium text-ink-900">
            Konfirmasi password <span class="text-brand-500">*</span>
          </label>
          <input
            id="inv-pass2"
            v-model="confirmPassword"
            :type="showPassword ? 'text' : 'password'"
            required
            autocomplete="new-password"
            class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
            :class="{ 'border-brand-500': passwordMismatch }"
          >
          <p v-if="passwordMismatch" class="mt-1 text-xs text-brand-700">Password tidak cocok.</p>
        </div>
      </div>

      <button
        type="submit"
        :disabled="!canSubmit"
        class="inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      >
        <Loader2 v-if="submitting" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
        <ShieldCheck v-else class="h-4 w-4" :stroke-width="1.75" />
        {{ submitting ? 'Mengaktifkan…' : 'Aktifkan akun' }}
      </button>

      <p class="text-center text-xs text-ink-500 leading-relaxed">
        Setelah aktif, Anda akan diarahkan ke dashboard admin sesuai role yang di-assign super admin.
      </p>
    </form>
  </main>
</template>
