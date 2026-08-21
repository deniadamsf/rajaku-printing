# Lewati Upload Desain — Langsung Cetak (Walk-in / POS)

**Ditambahkan:** 21 Agustus 2026
**Modul:** `design` (backend) + halaman admin desain (frontend)
**Berlaku untuk:** order walk-in (POS) saja — **tidak** berlaku untuk order online

---

## Masalah yang dipecahkan

Sebelum ini, order tidak bisa maju ke proses cetak sebelum ada file desain yang ter-upload ke sistem.

Untuk order **online** aturan itu memang benar dan tetap dipertahankan — sistem perlu tahu apa yang harus dicetak, dan file itu satu-satunya sumbernya.

Tapi untuk order **walk-in**, aturan yang sama jadi langkah mubazir. Pelanggan datang membawa desain yang sudah jadi, filenya dibuka langsung di komputer desainer, dan pekerjaannya bisa langsung dicetak. Staff terpaksa meng-upload file ke sistem semata-mata supaya tombol lanjut bisa diklik — padahal filenya sudah ada di depan mata.

Secara teknis, penyebabnya: `StaffVerifyUpload` mewajibkan minimal satu file berperan `customer_upload` yang belum terhapus. Kalau tidak ada, dia menolak. Sementara di state machine, dari status `dibayar` hanya ada dua jalan keluar — `desain_diverifikasi` dan `desain_dikerjakan` — jadi tidak ada jalan pintas ke `proses_cetak`.

## Solusinya

Jalur baru yang melewati kewajiban file itu, **tapi dikunci hanya untuk order POS** dan **mewajibkan catatan tertulis** sebagai gantinya.

Alur status tidak berubah sama sekali: order tetap bergerak `dibayar` → `desain_diverifikasi` seperti biasa. Yang dilewati hanya syarat "harus ada file".

---

## Kapan tombolnya muncul

Tombol **"Lewati Upload — Langsung Cetak"** di halaman `/admin/desain/[resi]` hanya tampil kalau **semua** syarat ini terpenuhi:

| Syarat | Alasan |
|---|---|
| Staff punya permission `design.skip_upload` | Izin tersendiri — lihat bagian di bawah |
| `channel = pos` | Kunci utama — order online tidak boleh lewat sini |
| `design_source = upload` | Order "minta desain" punya alurnya sendiri |
| `status = dibayar` | Titik yang sama dengan verifikasi normal |
| Belum ada file `customer_upload` | Kalau filenya sudah ada, pakai tombol "Verifikasi & Lanjut Cetak" biasa |

Kalau filenya ternyata sudah ter-upload, tombol ini sengaja **tidak** muncul — tidak ada gunanya melewati sesuatu yang sudah beres.

## Pengamannya

### Izin tersendiri, bukan menumpang izin lain

Tombol ini dikunci di permission **`design.skip_upload`**, terpisah dari `design.approve`.

Alasannya: tombol ini dirancang untuk momen di meja kasir, tapi role `cashier` tidak punya `design.approve`. Kalau tombolnya menumpang izin itu, kasir akan ditolak dan harus memanggil desainer — fiturnya mati di tangan orang yang justru dituju. Sebaliknya, kalau `design.approve` diberikan penuh ke kasir, dia ikut mendapat kuasa memverifikasi desain order **online**, yang jauh lebih luas dari yang dibutuhkan.

Izin baru menyelesaikan keduanya: kasir bisa melewati upload untuk walk-in di depannya, tanpa kuasa tambahan atas order online. Ini sejalan dengan §10 yang memang menganut izin granular yang bisa di-toggle super admin.

Izin ini diberikan ke role `super_admin`, `designer`, dan `cashier` lewat migration `000024`. Kalau nanti perlu dicabut dari salah satu role, super admin bisa melakukannya sendiri dari halaman `/admin/role` tanpa deploy ulang.

**Terkunci untuk POS.** Ini pengaman terpenting. Kalau tombol ini terbuka untuk order online, staff yang salah klik bisa mendorong order online tanpa file desain ke proses cetak — dan tidak akan ada yang tahu harus mencetak apa. Backend menolak permintaan apa pun yang `channel`-nya bukan `pos`, jadi kunci ini tidak bisa ditembus dari sisi frontend.

**Catatan wajib.** Staff harus menulis lokasi file fisiknya, misalnya `PC desain — folder Agustus/spanduk-warung-bu-sri`. Catatan ini tersimpan di riwayat order. Jadi kalau enam bulan lagi ada yang bertanya "order ini dulu dicetak pakai desain yang mana", jejaknya masih ada walaupun filenya tidak pernah masuk sistem. Permintaan dengan catatan kosong ditolak backend.

**Dialog konfirmasi.** Tombolnya tidak langsung mengeksekusi — dia membuka dialog `AdminConfirmDialog` varian `danger`, komponen yang sama dipakai halaman admin lain untuk aksi yang tidak bisa dibatalkan. Dialog itu menyebutkan konsekuensinya terus terang dan **menampilkan ulang catatan yang sudah diketik**, supaya staff bisa membacanya sekali lagi sebelum menekan "Ya, lanjut cetak". Kalau gagal, dialog tetap terbuka agar catatan tidak perlu diketik ulang.

**Tidak mengirim WhatsApp.** Pelanggan sedang berdiri di depan kasir — notifikasi ke ponselnya tidak ada gunanya. Ini konsisten dengan tombol "Disetujui Langsung" yang sudah ada untuk walk-in.

---

## Yang tidak berubah

- `StaffVerifyUpload` (jalur upload normal) — tetap mewajibkan file, tidak disentuh sama sekali.
- State machine order — tidak ada status atau transisi baru.
- Alur order online — sama persis seperti sebelumnya.

## Perubahan file

**Backend** (`backend/internal/design/`)

| File | Perubahan |
|---|---|
| `designapi/designapi.go` | Sentinel error `ErrSkipUploadOnlyForPOS`, `ErrSkipNoteRequired` |
| `service/dto.go` | Struct `SkipUploadInput` |
| `service/design_service.go` | Method `StaffSkipUpload` |
| `handler/design_handler.go` | Handler + pemetaan error ke HTTP 400 |
| `handler/routes.go` | Route baru (permission `design.skip_upload`) |
| `service/design_service_test.go` | 6 test (lihat daftar di bawah) |
| `migrations/000024_design_skip_upload_permission.{up,down}.sql` | Permission `design.skip_upload` + grant ke 3 role |

Endpoint: `POST /admin/orders/:resi/design-skip-upload`, body `{"note": "..."}`, permission `design.skip_upload`.

Test yang menjaga fitur ini:

| Test | Menjaga apa |
|---|---|
| `HappyPath` | Order maju ke `desain_diverifikasi`, catatan tersimpan ter-trim, WhatsApp **tidak** terkirim |
| `OnlineChannel_Rejected` | Order online tidak bisa lolos |
| `RequestSource_Rejected` | Order "minta desain" tidak melompati tahap pengerjaan |
| `WrongStatus_Rejected` | Klik dua kali tidak berefek ganda |
| `EmptyNote_Rejected` | Catatan tidak bisa dikosongkan |
| `ConcurrentStateChange_MapsToOrderStateChanged` | Dua staff klik bersamaan → yang kedua ditolak, bukan menimpa |

`backend/internal/server/router.go` sengaja tidak disentuh — modul design mendaftarkan rutenya sendiri lewat `RegisterRoutes`.

**Frontend**

| File | Perubahan |
|---|---|
| `composables/useDesign.ts` | Fungsi `skipUpload(resi, note)` |
| `pages/admin/desain/[resi]/index.vue` | Computed `canSkipUpload`, handler `doSkipUpload()`, card aksi + dialog konfirmasi |

Catatan lokasi file yang diketik staff muncul di timeline **"Riwayat status"** pada halaman detail order (`/admin/order/[resi]`). Staff produksi punya izin `order.view`, jadi mereka bisa membacanya saat hendak mencetak — catatan ini benar-benar sampai ke orang yang membutuhkannya, bukan cuma mengendap di database.

Di file `.vue` yang sama juga diperbaiki satu hal lama yang tidak berkaitan dengan fitur ini: tombol "Approve Walk-in" sebelumnya tidak punya focus ring keyboard (§26.6). Ditemukan oleh audit design drift, diperbaiki sekalian karena hanya satu baris.

---

## Catatan untuk ke depan

Kalau nanti muncul kebutuhan agar order online juga bisa dilewati, **jangan** sekadar melonggarkan guard `channel = pos`. Pikirkan dulu bagaimana staff produksi tahu apa yang harus dicetak, karena untuk order online tidak ada file dan tidak ada pelanggan di tempat yang bisa ditanya.
