/**
 * usePrefersReducedMotion — reactive wrapper untuk media query
 * `prefers-reduced-motion: reduce` (CLAUDE.md §26.6 — quality floor, wajib
 * dihormati, bukan opsional).
 *
 * Dipakai landing motion components (scroll reveal, lightbox, sticky CTA) untuk
 * men-nol-kan durasi/transform saat user minta reduced motion — bukan cuma
 * "dilewati", tapi tetap tampilkan final state langsung.
 */
export function usePrefersReducedMotion() {
  const prefersReduced = ref(false)

  if (import.meta.client) {
    const mql = window.matchMedia('(prefers-reduced-motion: reduce)')
    prefersReduced.value = mql.matches

    const handler = (e: MediaQueryListEvent) => {
      prefersReduced.value = e.matches
    }
    mql.addEventListener('change', handler)
    onScopeDispose(() => mql.removeEventListener('change', handler))
  }

  return prefersReduced
}
