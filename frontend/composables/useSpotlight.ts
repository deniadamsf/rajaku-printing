/**
 * useSpotlight — sorotan lembut yang mengikuti kursor di atas kartu.
 *
 * Pemakaian: pasang `@pointermove="onPointerMove"` di WADAH grid (satu
 * listener untuk semua kartu — bukan satu listener per kartu), lalu beri
 * kelas `spotlight` pada tiap kartunya. Gaya visualnya ada di
 * `assets/css/tailwind.css`.
 *
 *   const { onPointerMove } = useSpotlight()
 *   <div class="grid" @pointermove="onPointerMove">
 *     <a class="spotlight rounded-lg border …">…</a>
 *   </div>
 *
 * Yang ditulis hanyalah dua custom property (posisi kursor relatif terhadap
 * kartu); tampil/hilangnya sorotan diurus `:hover` di CSS. Jadi tidak ada
 * kelas yang ditambah/dibuang dari JavaScript, dan tidak ada state Vue yang
 * ikut berubah tiap gerakan kursor — pointermove itu sering sekali terpanggil.
 */
export function useSpotlight() {
  function onPointerMove(e: PointerEvent) {
    // Sentuhan/pena diabaikan: efek ini memang cuma untuk kursor asli, sejalan
    // dengan penjagaan @media (hover: hover) di CSS-nya.
    if (e.pointerType !== 'mouse') return

    const card = (e.target as HTMLElement | null)?.closest<HTMLElement>('.spotlight')
    if (!card) return

    const r = card.getBoundingClientRect()
    card.style.setProperty('--spot-x', `${e.clientX - r.left}px`)
    card.style.setProperty('--spot-y', `${e.clientY - r.top}px`)
  }

  return { onPointerMove }
}
