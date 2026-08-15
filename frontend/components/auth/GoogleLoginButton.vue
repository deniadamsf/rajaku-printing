<script setup lang="ts">
/**
 * Tombol "Lanjut dengan Google" — dipakai di /login & /register (§10).
 * Klik = full browser navigation ke backend (bukan fetch), lihat
 * `useGoogleAuth().startGoogleLogin()`.
 *
 * Style: secondary button §26.7 (border-hairline, bukan brand solid — Google
 * login bukan CTA utama, tetap ada tapi tidak bersaing dgn tombol submit form).
 */
import { Loader2 } from '@lucide/vue'

interface Props {
  /** Path tujuan pasca login sukses, diteruskan ke backend via `?redirect=`. */
  redirect?: string
  label?: string
}

const props = withDefaults(defineProps<Props>(), {
  redirect: '/akun',
  label: 'Lanjut dengan Google',
})

const google = useGoogleAuth()
const redirecting = ref(false)

function onClick() {
  if (redirecting.value) return
  redirecting.value = true
  google.startGoogleLogin(props.redirect)
}
</script>

<template>
  <button
    type="button"
    :disabled="redirecting"
    class="inline-flex w-full items-center justify-center gap-2 rounded-md border border-hairline bg-canvas px-4 py-2.5 text-sm font-semibold text-ink-900 transition-colors hover:bg-canvas-alt focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-60"
    @click="onClick"
  >
    <Loader2 v-if="redirecting" class="h-4 w-4 animate-spin text-ink-400" :stroke-width="1.5" />
    <!-- Pengecualian §26: mark pihak ketiga, warna wajib mengikuti brand guideline Google -->
    <svg v-else class="h-4 w-4 flex-none" viewBox="0 0 48 48" aria-hidden="true">
      <path fill="#4285F4" d="M45.12 24.5c0-1.56-.14-3.06-.4-4.5H24v8.51h11.84c-.51 2.75-2.06 5.08-4.39 6.64v5.52h7.11c4.16-3.83 6.56-9.47 6.56-16.17z" />
      <path fill="#34A853" d="M24 46c5.94 0 10.92-1.97 14.56-5.33l-7.11-5.52c-1.97 1.32-4.49 2.1-7.45 2.1-5.73 0-10.58-3.87-12.31-9.07H4.34v5.7C7.96 41.07 15.4 46 24 46z" />
      <path fill="#FBBC05" d="M11.69 28.18C11.25 26.86 11 25.45 11 24s.25-2.86.69-4.18v-5.7H4.34C2.85 17.09 2 20.45 2 24s.85 6.91 2.34 9.88l7.35-5.7z" />
      <path fill="#EA4335" d="M24 10.75c3.23 0 6.13 1.11 8.41 3.29l6.31-6.31C34.91 4.18 29.93 2 24 2 15.4 2 7.96 6.93 4.34 14.12l7.35 5.7c1.73-5.2 6.58-9.07 12.31-9.07z" />
    </svg>
    <span>{{ redirecting ? 'Mengalihkan…' : label }}</span>
  </button>
</template>
