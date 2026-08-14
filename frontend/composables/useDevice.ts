import { useBreakpoints } from '@vueuse/core'

/**
 * useDevice — adaptive rendering (spec section 18).
 *
 * Bukan sekadar responsive CSS: dipakai untuk conditional component rendering
 * di level Nuxt supaya JS bundle desktop-only (TresJS, GSAP scroll-scrub) TIDAK
 * ikut terkirim ke mobile. Contoh pakai:
 *
 *   const { isMobile, isDesktop } = useDevice()
 *   <ClientOnly>
 *     <HeroScrollScrub v-if="isDesktop" />
 *     <HeroStaticMobile v-else />
 *   </ClientOnly>
 *
 * Breakpoint konsisten dengan Tailwind default (md=768, lg=1024).
 */
export function useDevice() {
  const bp = useBreakpoints({
    mobile: 0,
    tablet: 768,
    desktop: 1024,
  })

  const isMobile = bp.smaller('tablet')
  const isTablet = bp.between('tablet', 'desktop')
  const isDesktop = bp.greaterOrEqual('desktop')

  return { isMobile, isTablet, isDesktop }
}
