# Rajaku Printing — Spec Proyek

Seluruh isi spec ada di **[AGENTS.md](AGENTS.md)**.

Berkas ini sengaja dibuat pendek. `AGENTS.md` adalah nama yang dibaca agent
coding di luar Claude Code (Gemini Antigravity, Gemini CLI, Cursor, dsb),
sedangkan `CLAUDE.md` dibaca Claude Code. Isinya disatukan di satu berkas
supaya tidak ada dua salinan spec yang bisa melenceng diam-diam — kalau ada
dua, yang satu pasti basi dan tidak ada yang memberi tahu.

@AGENTS.md

> **Kalau baris `@AGENTS.md` di atas tidak otomatis memuat isinya, buka dan
> baca `AGENTS.md` sekarang juga sebelum menulis kode apa pun di repo ini.**
> Spec itu memuat aturan yang tidak boleh ditebak: nama status order (§4),
> aturan coding Go (§22), design language (§26), snapshot diskon (§28.2), dan
> order multi-item (§32).

**Semua perubahan spec ditulis di `AGENTS.md`, bukan di berkas ini.**

Berkas pendamping:

- `docs/rencana-app-flutter.md` — rencana bertahap app Flutter customer/member.
- `DEPLOY.md` — prosedur deploy ke VPS.
