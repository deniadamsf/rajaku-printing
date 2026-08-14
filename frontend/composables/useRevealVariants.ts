/**
 * useRevealVariants — variants standar untuk scroll-reveal stagger di landing
 * page (satu-satunya "momen orkestrasi" selain hero — lihat brief motion §1).
 *
 * Pola pakai (container + item, `once: true` wajib — brief §2):
 *
 *   const { container, item } = useRevealVariants()
 *
 *   <motion.ul
 *     :variants="container"
 *     initial="hidden"
 *     while-in-view="show"
 *     :in-view-options="{ once: true, margin: '-100px' }"
 *   >
 *     <motion.li v-for="x in list" :key="x.id" :variants="item">...</motion.li>
 *   </motion.ul>
 *
 * Hanya mengubah `opacity`/`transform` (translateY) — tidak pernah width/height/
 * top/left (brief §3). Menghormati prefers-reduced-motion (§26.6): kalau reduced,
 * durasi ~0 dan elemen langsung di posisi akhir, bukan di-skip diam-diam.
 */
export function useRevealVariants(options?: { stagger?: number; y?: number; duration?: number }) {
  const prefersReduced = usePrefersReducedMotion()
  const stagger = options?.stagger ?? 0.08
  const yOffset = options?.y ?? 16
  const duration = options?.duration ?? 0.5

  const container = computed(() => ({
    hidden: {},
    show: {
      transition: prefersReduced.value
        ? { staggerChildren: 0, delayChildren: 0 }
        : { staggerChildren: stagger, delayChildren: 0.04 },
    },
  }))

  const item = computed(() => ({
    hidden: prefersReduced.value ? { opacity: 1, y: 0 } : { opacity: 0, y: yOffset },
    show: {
      opacity: 1,
      y: 0,
      transition: {
        duration: prefersReduced.value ? 0 : duration,
        ease: [0.22, 1, 0.36, 1],
      },
    },
  }))

  return { container, item, prefersReduced }
}
