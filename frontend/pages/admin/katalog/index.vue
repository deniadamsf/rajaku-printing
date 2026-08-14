<script setup lang="ts">
/**
 * /admin/katalog — CRUD bahan, produk, dan pricing rule (§9/§10).
 *
 * Layout:
 *   - Tab switcher (Bahan | Produk).
 *   - Bahan: list + inline modal untuk create/edit + toggle aktif.
 *   - Produk: list + modal create/edit + drilldown ke tabel pricing per produk.
 *   - Pricing rows dieditkan langsung dari panel drilldown produk
 *     (bentuk row berbeda per_m2 vs paket, sesuai product.pricing_type).
 *
 * Design: patuh CLAUDE.md §26 (brand crimson + ink warm neutral + Lucide).
 */
import {
  Layers,
  Package,
  Plus,
  Pencil,
  Power,
  ChevronLeft,
  Loader2,
  Trash2,
} from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import type {
  AdminMaterial,
  AdminMaterialInput,
  AdminPricingInput,
  AdminPricingRow,
  AdminProduct,
  AdminProductDetail,
  AdminProductInput,
} from '~/types/catalog-admin'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Katalog — Rajaku Admin' })

const admin = useAdminCatalog()

// -------------------- top-level state --------------------
type Tab = 'materials' | 'products'
const tab = ref<Tab>('materials')
const errorMsg = ref<string | null>(null)
const successMsg = ref<string | null>(null)

function showSuccess(msg: string) {
  successMsg.value = msg
  setTimeout(() => (successMsg.value = null), 2500)
}

function toApiError(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

// -------------------- materials --------------------
const materials = ref<AdminMaterial[]>([])
const materialsLoading = ref(false)

async function fetchMaterials() {
  materialsLoading.value = true
  errorMsg.value = null
  try {
    const res = await admin.listMaterials()
    materials.value = res.materials
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal memuat bahan')
  } finally {
    materialsLoading.value = false
  }
}

// Material modal (create + edit)
const materialModalOpen = ref(false)
const materialEditingId = ref<string | null>(null)
const materialForm = reactive<AdminMaterialInput>({
  code: '',
  name: '',
  description: '',
})
const materialSaving = ref(false)

function openMaterialModal(existing?: AdminMaterial) {
  errorMsg.value = null
  if (existing) {
    materialEditingId.value = existing.id
    materialForm.code = existing.code
    materialForm.name = existing.name
    materialForm.description = existing.description ?? ''
  } else {
    materialEditingId.value = null
    materialForm.code = ''
    materialForm.name = ''
    materialForm.description = ''
  }
  materialModalOpen.value = true
}

async function saveMaterial() {
  materialSaving.value = true
  errorMsg.value = null
  try {
    const body: AdminMaterialInput = {
      code: materialForm.code.trim(),
      name: materialForm.name.trim(),
      description: materialForm.description?.trim() || undefined,
    }
    if (materialEditingId.value) {
      await admin.updateMaterial(materialEditingId.value, body)
      showSuccess('Bahan diperbarui.')
    } else {
      await admin.createMaterial(body)
      showSuccess('Bahan baru dibuat.')
    }
    materialModalOpen.value = false
    await fetchMaterials()
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal menyimpan bahan')
  } finally {
    materialSaving.value = false
  }
}

async function toggleMaterialActive(m: AdminMaterial) {
  errorMsg.value = null
  try {
    if (m.is_active) await admin.deactivateMaterial(m.id)
    else await admin.activateMaterial(m.id)
    showSuccess(m.is_active ? 'Bahan dinonaktifkan.' : 'Bahan diaktifkan.')
    await fetchMaterials()
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal mengubah status bahan')
  }
}

// -------------------- products --------------------
const products = ref<AdminProduct[]>([])
const productsLoading = ref(false)

async function fetchProducts() {
  productsLoading.value = true
  errorMsg.value = null
  try {
    const res = await admin.listProducts()
    products.value = res.products
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal memuat produk')
  } finally {
    productsLoading.value = false
  }
}

// Product modal (create + edit — TANPA pricings; itu di panel drilldown)
const productModalOpen = ref(false)
const productEditingId = ref<string | null>(null)
const productEditingHasPricings = ref(false)
const productForm = reactive<AdminProductInput & { min_width_cm: number | null; min_height_cm: number | null; max_width_cm: number | null; max_height_cm: number | null }>({
  slug: '',
  name: '',
  description: '',
  category: '',
  pricing_type: 'per_m2',
  min_width_cm: null,
  min_height_cm: null,
  max_width_cm: null,
  max_height_cm: null,
  display_order: 0,
})
const productSaving = ref(false)

function openProductModal(existing?: AdminProduct) {
  errorMsg.value = null
  if (existing) {
    productEditingId.value = existing.id
    productForm.slug = existing.slug
    productForm.name = existing.name
    productForm.description = existing.description ?? ''
    productForm.category = existing.category
    productForm.pricing_type = existing.pricing_type
    productForm.min_width_cm = existing.min_width_cm ?? null
    productForm.min_height_cm = existing.min_height_cm ?? null
    productForm.max_width_cm = existing.max_width_cm ?? null
    productForm.max_height_cm = existing.max_height_cm ?? null
    productForm.display_order = existing.display_order
    // Kunci pricing_type editor kalau produk sudah punya pricing rows.
    // Kita cek via detail fetch (async — untuk MVP asumsi safe: lock kalau editing).
    productEditingHasPricings.value = false
  } else {
    productEditingId.value = null
    productEditingHasPricings.value = false
    productForm.slug = ''
    productForm.name = ''
    productForm.description = ''
    productForm.category = ''
    productForm.pricing_type = 'per_m2'
    productForm.min_width_cm = null
    productForm.min_height_cm = null
    productForm.max_width_cm = null
    productForm.max_height_cm = null
    productForm.display_order = 0
  }
  productModalOpen.value = true
}

async function saveProduct() {
  productSaving.value = true
  errorMsg.value = null
  try {
    const body: AdminProductInput = {
      slug: productForm.slug.trim(),
      name: productForm.name.trim(),
      description: productForm.description?.trim() || undefined,
      category: productForm.category.trim(),
      pricing_type: productForm.pricing_type,
      min_width_cm: productForm.min_width_cm ?? undefined,
      min_height_cm: productForm.min_height_cm ?? undefined,
      max_width_cm: productForm.max_width_cm ?? undefined,
      max_height_cm: productForm.max_height_cm ?? undefined,
      display_order: productForm.display_order ?? 0,
    }
    if (productEditingId.value) {
      await admin.updateProduct(productEditingId.value, body)
      showSuccess('Produk diperbarui.')
      // reload drilldown kalau sedang buka detail-nya
      if (detailProduct.value?.id === productEditingId.value) {
        await openProductDetail(productEditingId.value)
      }
    } else {
      await admin.createProduct(body)
      showSuccess('Produk baru dibuat.')
    }
    productModalOpen.value = false
    await fetchProducts()
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal menyimpan produk')
  } finally {
    productSaving.value = false
  }
}

async function toggleProductActive(p: AdminProduct) {
  errorMsg.value = null
  try {
    if (p.is_active) await admin.deactivateProduct(p.id)
    else await admin.activateProduct(p.id)
    showSuccess(p.is_active ? 'Produk dinonaktifkan.' : 'Produk diaktifkan.')
    await fetchProducts()
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal mengubah status produk')
  }
}

// -------------------- product drilldown (pricings) --------------------
const detailProduct = ref<AdminProductDetail | null>(null)
const detailLoading = ref(false)

async function openProductDetail(id: string) {
  detailLoading.value = true
  errorMsg.value = null
  try {
    detailProduct.value = await admin.getProduct(id)
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal memuat detail produk')
    detailProduct.value = null
  } finally {
    detailLoading.value = false
  }
}

function closeProductDetail() {
  detailProduct.value = null
  pricingModalOpen.value = false
}

// Pricing modal
const pricingModalOpen = ref(false)
const pricingEditingId = ref<string | null>(null)
const pricingForm = reactive<{
  material_id: string
  price_per_m2: number | null
  min_charge_m2: number | null
  width_cm: number | null
  height_cm: number | null
  package_label: string
  price_total: number | null
}>({
  material_id: '',
  price_per_m2: null,
  min_charge_m2: null,
  width_cm: null,
  height_cm: null,
  package_label: '',
  price_total: null,
})
const pricingSaving = ref(false)

const activeMaterials = computed(() => materials.value.filter((m) => m.is_active))

function openPricingModal(existing?: AdminPricingRow) {
  errorMsg.value = null
  if (existing) {
    pricingEditingId.value = existing.id
    pricingForm.material_id = existing.material_id
    pricingForm.price_per_m2 = existing.price_per_m2 ?? null
    pricingForm.min_charge_m2 = existing.min_charge_m2 ?? null
    pricingForm.width_cm = existing.width_cm ?? null
    pricingForm.height_cm = existing.height_cm ?? null
    pricingForm.package_label = existing.package_label ?? ''
    pricingForm.price_total = existing.price_total ?? null
  } else {
    pricingEditingId.value = null
    pricingForm.material_id = activeMaterials.value[0]?.id ?? ''
    pricingForm.price_per_m2 = null
    pricingForm.min_charge_m2 = null
    pricingForm.width_cm = null
    pricingForm.height_cm = null
    pricingForm.package_label = ''
    pricingForm.price_total = null
  }
  pricingModalOpen.value = true
}

async function savePricing() {
  if (!detailProduct.value) return
  pricingSaving.value = true
  errorMsg.value = null
  try {
    const body: AdminPricingInput = { material_id: pricingForm.material_id }
    if (detailProduct.value.pricing_type === 'per_m2') {
      body.price_per_m2 = pricingForm.price_per_m2 ?? undefined
      body.min_charge_m2 = pricingForm.min_charge_m2 ?? undefined
    } else {
      body.width_cm = pricingForm.width_cm ?? undefined
      body.height_cm = pricingForm.height_cm ?? undefined
      body.price_total = pricingForm.price_total ?? undefined
      body.package_label = pricingForm.package_label.trim() || undefined
    }
    if (pricingEditingId.value) {
      await admin.updatePricing(pricingEditingId.value, body)
      showSuccess('Pricing diperbarui.')
    } else {
      await admin.createPricing(detailProduct.value.id, body)
      showSuccess('Pricing baru ditambahkan.')
    }
    pricingModalOpen.value = false
    await openProductDetail(detailProduct.value.id)
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal menyimpan pricing')
  } finally {
    pricingSaving.value = false
  }
}

async function togglePricingActive(row: AdminPricingRow) {
  errorMsg.value = null
  try {
    if (row.is_active) await admin.deactivatePricing(row.id)
    else await admin.activatePricing(row.id)
    if (detailProduct.value) await openProductDetail(detailProduct.value.id)
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal mengubah status pricing')
  }
}

async function deletePricingRow(row: AdminPricingRow) {
  if (!confirm(`Hapus pricing "${row.material_name}"? Aksi ini permanen.`)) return
  errorMsg.value = null
  try {
    await admin.deletePricing(row.id)
    showSuccess('Pricing dihapus.')
    if (detailProduct.value) await openProductDetail(detailProduct.value.id)
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal menghapus pricing')
  }
}

// -------------------- helpers --------------------
function fmtIDR(v?: number | null): string {
  if (v == null) return '—'
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    maximumFractionDigits: 0,
  }).format(v)
}

// -------------------- init --------------------
onMounted(async () => {
  await Promise.all([fetchMaterials(), fetchProducts()])
})
</script>

<template>
  <section>
    <AdminPageHeader
      title="Katalog"
      subtitle="Kelola bahan, produk, dan aturan harga. Perubahan langsung dipakai form order & POS."
    >
      <template #actions>
        <button
          v-if="tab === 'materials'"
          type="button"
          class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt transition-colors"
          @click="openMaterialModal()"
        >
          <Plus class="h-4 w-4" :stroke-width="1.75" />
          Bahan baru
        </button>
        <button
          v-else-if="tab === 'products' && !detailProduct"
          type="button"
          class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt transition-colors"
          @click="openProductModal()"
        >
          <Plus class="h-4 w-4" :stroke-width="1.75" />
          Produk baru
        </button>
      </template>
    </AdminPageHeader>

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />
    <AlertMessage v-if="successMsg" variant="success" :message="successMsg" class="mb-4" />

    <!-- Tab switcher -->
    <div v-if="!detailProduct" class="mb-6 inline-flex rounded-md border border-hairline overflow-hidden">
      <button
        type="button"
        :class="[
          'inline-flex items-center gap-2 px-4 py-2 text-sm font-medium transition-colors',
          tab === 'materials' ? 'bg-ink-950 text-canvas' : 'bg-canvas text-ink-700 hover:bg-canvas-alt',
        ]"
        @click="tab = 'materials'"
      >
        <Layers class="h-4 w-4" :stroke-width="1.75" />
        Bahan
      </button>
      <button
        type="button"
        :class="[
          'inline-flex items-center gap-2 px-4 py-2 text-sm font-medium border-l border-hairline transition-colors',
          tab === 'products' ? 'bg-ink-950 text-canvas' : 'bg-canvas text-ink-700 hover:bg-canvas-alt',
        ]"
        @click="tab = 'products'"
      >
        <Package class="h-4 w-4" :stroke-width="1.75" />
        Produk
      </button>
    </div>

    <!-- ============================ Bahan tab ============================ -->
    <div v-if="tab === 'materials' && !detailProduct">
      <div class="rounded-lg border border-hairline bg-canvas overflow-hidden">
        <table class="w-full text-sm">
          <thead class="bg-canvas-alt border-b border-hairline text-left">
            <tr>
              <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Code</th>
              <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Nama</th>
              <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 hidden md:table-cell">Deskripsi</th>
              <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 w-28">Status</th>
              <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 w-40 text-right">Aksi</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-hairline">
            <tr v-if="materialsLoading">
              <td colspan="5" class="px-4 py-12 text-center text-sm text-ink-500">
                <Loader2 class="mx-auto h-4 w-4 animate-spin" :stroke-width="1.75" />
                <p class="mt-2">Memuat…</p>
              </td>
            </tr>
            <tr v-else-if="materials.length === 0">
              <td colspan="5" class="px-4 py-12 text-center text-sm text-ink-500">
                Belum ada bahan. Klik <strong class="text-ink-900">Bahan baru</strong> untuk mulai.
              </td>
            </tr>
            <tr v-for="m in materials" :key="m.id" :class="['hover:bg-canvas-alt/60', !m.is_active && 'opacity-60']">
              <td class="px-4 py-3 font-mono text-xs text-ink-700">{{ m.code }}</td>
              <td class="px-4 py-3 text-ink-900">{{ m.name }}</td>
              <td class="px-4 py-3 text-xs text-ink-500 hidden md:table-cell max-w-md truncate">{{ m.description || '—' }}</td>
              <td class="px-4 py-3">
                <span :class="[
                  'inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset',
                  m.is_active
                    ? 'bg-emerald-50 text-emerald-800 ring-emerald-200'
                    : 'bg-canvas-alt text-ink-500 ring-hairline',
                ]">
                  {{ m.is_active ? 'Aktif' : 'Nonaktif' }}
                </span>
              </td>
              <td class="px-4 py-3">
                <div class="flex justify-end gap-1">
                  <button
                    type="button"
                    class="inline-flex items-center gap-1 rounded-md border border-hairline bg-canvas px-2 py-1 text-xs text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
                    @click="openMaterialModal(m)"
                  >
                    <Pencil class="h-3.5 w-3.5" :stroke-width="1.75" />
                    Edit
                  </button>
                  <button
                    type="button"
                    class="inline-flex items-center gap-1 rounded-md border border-hairline bg-canvas px-2 py-1 text-xs text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
                    @click="toggleMaterialActive(m)"
                  >
                    <Power class="h-3.5 w-3.5" :stroke-width="1.75" />
                    {{ m.is_active ? 'Nonaktifkan' : 'Aktifkan' }}
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- ============================ Produk tab (list) ============================ -->
    <div v-if="tab === 'products' && !detailProduct">
      <div class="rounded-lg border border-hairline bg-canvas overflow-hidden">
        <table class="w-full text-sm">
          <thead class="bg-canvas-alt border-b border-hairline text-left">
            <tr>
              <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Slug / Nama</th>
              <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 w-28">Kategori</th>
              <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 w-28">Pricing</th>
              <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 hidden lg:table-cell">Ukuran range</th>
              <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 w-24">Status</th>
              <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 w-48 text-right">Aksi</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-hairline">
            <tr v-if="productsLoading">
              <td colspan="6" class="px-4 py-12 text-center text-sm text-ink-500">
                <Loader2 class="mx-auto h-4 w-4 animate-spin" :stroke-width="1.75" />
                <p class="mt-2">Memuat…</p>
              </td>
            </tr>
            <tr v-else-if="products.length === 0">
              <td colspan="6" class="px-4 py-12 text-center text-sm text-ink-500">
                Belum ada produk.
              </td>
            </tr>
            <tr v-for="p in products" :key="p.id" :class="['hover:bg-canvas-alt/60', !p.is_active && 'opacity-60']">
              <td class="px-4 py-3">
                <p class="text-ink-900 font-medium">{{ p.name }}</p>
                <p class="mt-0.5 font-mono text-[11px] text-ink-500">{{ p.slug }}</p>
              </td>
              <td class="px-4 py-3 text-xs text-ink-700">{{ p.category }}</td>
              <td class="px-4 py-3">
                <span class="inline-flex items-center rounded-full bg-gold-50 px-2 py-0.5 text-[11px] font-medium text-gold-900 ring-1 ring-inset ring-gold-200">
                  {{ p.pricing_type }}
                </span>
              </td>
              <td class="px-4 py-3 hidden lg:table-cell text-xs text-ink-500 font-mono">
                <template v-if="p.min_width_cm && p.max_width_cm">
                  {{ p.min_width_cm }}–{{ p.max_width_cm }} × {{ p.min_height_cm }}–{{ p.max_height_cm }} cm
                </template>
                <template v-else>—</template>
              </td>
              <td class="px-4 py-3">
                <span :class="[
                  'inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset',
                  p.is_active
                    ? 'bg-emerald-50 text-emerald-800 ring-emerald-200'
                    : 'bg-canvas-alt text-ink-500 ring-hairline',
                ]">
                  {{ p.is_active ? 'Aktif' : 'Nonaktif' }}
                </span>
              </td>
              <td class="px-4 py-3">
                <div class="flex justify-end gap-1">
                  <button
                    type="button"
                    class="inline-flex items-center gap-1 rounded-md border border-hairline bg-canvas px-2 py-1 text-xs text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
                    @click="openProductDetail(p.id)"
                  >
                    <Layers class="h-3.5 w-3.5" :stroke-width="1.75" />
                    Pricing
                  </button>
                  <button
                    type="button"
                    class="inline-flex items-center gap-1 rounded-md border border-hairline bg-canvas px-2 py-1 text-xs text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
                    @click="openProductModal(p)"
                  >
                    <Pencil class="h-3.5 w-3.5" :stroke-width="1.75" />
                    Edit
                  </button>
                  <button
                    type="button"
                    class="inline-flex items-center gap-1 rounded-md border border-hairline bg-canvas px-2 py-1 text-xs text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
                    @click="toggleProductActive(p)"
                  >
                    <Power class="h-3.5 w-3.5" :stroke-width="1.75" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- ============================ Product drilldown (pricings editor) ============================ -->
    <div v-if="detailProduct">
      <div class="mb-4 flex flex-wrap items-center gap-3">
        <button
          type="button"
          class="inline-flex items-center gap-1 rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
          @click="closeProductDetail"
        >
          <ChevronLeft class="h-4 w-4" :stroke-width="1.75" />
          Kembali
        </button>
        <div class="flex-1">
          <p class="font-serif text-lg font-semibold text-ink-950">{{ detailProduct.name }}</p>
          <p class="mt-0.5 text-xs text-ink-500">
            <span class="font-mono">{{ detailProduct.slug }}</span>
            · pricing_type <strong class="text-ink-900">{{ detailProduct.pricing_type }}</strong>
            · kategori <strong class="text-ink-900">{{ detailProduct.category }}</strong>
          </p>
        </div>
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt transition-colors"
          :disabled="activeMaterials.length === 0"
          @click="openPricingModal()"
        >
          <Plus class="h-4 w-4" :stroke-width="1.75" />
          Tambah pricing
        </button>
      </div>

      <p v-if="activeMaterials.length === 0" class="mb-4 rounded-md border border-gold-200 bg-gold-50 p-3 text-xs text-gold-900">
        Belum ada bahan aktif. Buat bahan dulu di tab <strong>Bahan</strong> supaya bisa tambah pricing.
      </p>

      <div class="rounded-lg border border-hairline bg-canvas overflow-hidden">
        <table class="w-full text-sm">
          <thead class="bg-canvas-alt border-b border-hairline text-left">
            <tr>
              <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Bahan</th>
              <template v-if="detailProduct.pricing_type === 'per_m2'">
                <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 w-40 text-right">Harga / m²</th>
                <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 w-40 text-right">Min charge (m²)</th>
              </template>
              <template v-else>
                <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 w-40">Ukuran</th>
                <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Label paket</th>
                <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 w-32 text-right">Harga</th>
              </template>
              <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 w-24">Status</th>
              <th class="px-4 py-3 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 w-40 text-right">Aksi</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-hairline">
            <tr v-if="detailLoading">
              <td :colspan="detailProduct.pricing_type === 'per_m2' ? 5 : 6" class="px-4 py-12 text-center text-sm text-ink-500">
                <Loader2 class="mx-auto h-4 w-4 animate-spin" :stroke-width="1.75" />
              </td>
            </tr>
            <tr v-else-if="detailProduct.pricings.length === 0">
              <td :colspan="detailProduct.pricing_type === 'per_m2' ? 5 : 6" class="px-4 py-12 text-center text-sm text-ink-500">
                Belum ada pricing row. Tambahkan minimal 1 supaya produk bisa muncul di order/POS.
              </td>
            </tr>
            <tr
              v-for="row in detailProduct.pricings"
              :key="row.id"
              :class="['hover:bg-canvas-alt/60', !row.is_active && 'opacity-60']"
            >
              <td class="px-4 py-3">
                <p class="text-ink-900">{{ row.material_name }}</p>
                <p class="mt-0.5 font-mono text-[11px] text-ink-500">{{ row.material_code }}</p>
              </td>
              <template v-if="detailProduct.pricing_type === 'per_m2'">
                <td class="px-4 py-3 text-right text-ink-900">{{ fmtIDR(row.price_per_m2) }}</td>
                <td class="px-4 py-3 text-right text-xs text-ink-500 font-mono">
                  {{ row.min_charge_m2 != null ? row.min_charge_m2.toFixed(2) + ' m²' : '—' }}
                </td>
              </template>
              <template v-else>
                <td class="px-4 py-3 text-xs text-ink-700 font-mono">{{ row.width_cm }} × {{ row.height_cm }} cm</td>
                <td class="px-4 py-3 text-xs text-ink-500">{{ row.package_label || '—' }}</td>
                <td class="px-4 py-3 text-right text-ink-900">{{ fmtIDR(row.price_total) }}</td>
              </template>
              <td class="px-4 py-3">
                <span :class="[
                  'inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset',
                  row.is_active
                    ? 'bg-emerald-50 text-emerald-800 ring-emerald-200'
                    : 'bg-canvas-alt text-ink-500 ring-hairline',
                ]">
                  {{ row.is_active ? 'Aktif' : 'Nonaktif' }}
                </span>
              </td>
              <td class="px-4 py-3">
                <div class="flex justify-end gap-1">
                  <button
                    type="button"
                    class="inline-flex items-center rounded-md border border-hairline bg-canvas p-1.5 text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
                    :aria-label="'Edit pricing ' + row.material_name"
                    @click="openPricingModal(row)"
                  >
                    <Pencil class="h-3.5 w-3.5" :stroke-width="1.75" />
                  </button>
                  <button
                    type="button"
                    class="inline-flex items-center rounded-md border border-hairline bg-canvas p-1.5 text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
                    :aria-label="row.is_active ? 'Nonaktifkan' : 'Aktifkan'"
                    @click="togglePricingActive(row)"
                  >
                    <Power class="h-3.5 w-3.5" :stroke-width="1.75" />
                  </button>
                  <button
                    type="button"
                    class="inline-flex items-center rounded-md border border-brand-200 bg-canvas p-1.5 text-brand-700 hover:bg-brand-50 hover:border-brand-300 transition-colors"
                    aria-label="Hapus"
                    @click="deletePricingRow(row)"
                  >
                    <Trash2 class="h-3.5 w-3.5" :stroke-width="1.75" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- ============================ Material modal ============================ -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="materialModalOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="!materialSaving && (materialModalOpen = false)" />
          <div role="dialog" aria-modal="true" class="relative w-full max-w-md rounded-lg bg-canvas shadow-xl ring-1 ring-black/5">
            <form class="p-6" @submit.prevent="saveMaterial">
              <h3 class="font-serif text-lg font-semibold text-ink-950">
                {{ materialEditingId ? 'Edit bahan' : 'Bahan baru' }}
              </h3>
              <div class="mt-4 space-y-4">
                <div>
                  <label for="mat-code" class="block text-sm font-medium text-ink-900">Code <span class="text-brand-500">*</span></label>
                  <input
                    id="mat-code"
                    v-model="materialForm.code"
                    type="text"
                    required
                    placeholder="FLEXI_280"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-mono placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                  <p class="mt-1 text-xs text-ink-500">Huruf besar, angka, tanda <span class="font-mono">-</span> / <span class="font-mono">_</span>. Dipakai internal.</p>
                </div>
                <div>
                  <label for="mat-name" class="block text-sm font-medium text-ink-900">Nama <span class="text-brand-500">*</span></label>
                  <input
                    id="mat-name"
                    v-model="materialForm.name"
                    type="text"
                    required
                    placeholder="Flexi 280 gsm"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                </div>
                <div>
                  <label for="mat-desc" class="block text-sm font-medium text-ink-900">Deskripsi</label>
                  <textarea
                    id="mat-desc"
                    v-model="materialForm.description"
                    rows="2"
                    placeholder="Kain vinyl 280 gsm — banner outdoor standar."
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  />
                </div>
              </div>
              <div class="mt-6 flex justify-end gap-2">
                <button
                  type="button"
                  class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50"
                  :disabled="materialSaving"
                  @click="materialModalOpen = false"
                >
                  Batal
                </button>
                <button
                  type="submit"
                  :disabled="materialSaving"
                  class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60"
                >
                  <Loader2 v-if="materialSaving" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
                  Simpan
                </button>
              </div>
            </form>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- ============================ Product modal ============================ -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="productModalOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="!productSaving && (productModalOpen = false)" />
          <div role="dialog" aria-modal="true" class="relative w-full max-w-xl rounded-lg bg-canvas shadow-xl ring-1 ring-black/5 max-h-[90vh] overflow-y-auto">
            <form class="p-6" @submit.prevent="saveProduct">
              <h3 class="font-serif text-lg font-semibold text-ink-950">
                {{ productEditingId ? 'Edit produk' : 'Produk baru' }}
              </h3>
              <div class="mt-4 grid gap-4 sm:grid-cols-2">
                <div>
                  <label for="prd-slug" class="block text-sm font-medium text-ink-900">Slug <span class="text-brand-500">*</span></label>
                  <input
                    id="prd-slug"
                    v-model="productForm.slug"
                    type="text"
                    required
                    placeholder="banner-flexi"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-mono placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                </div>
                <div>
                  <label for="prd-cat" class="block text-sm font-medium text-ink-900">Kategori <span class="text-brand-500">*</span></label>
                  <input
                    id="prd-cat"
                    v-model="productForm.category"
                    type="text"
                    required
                    placeholder="banner"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                </div>
                <div class="sm:col-span-2">
                  <label for="prd-name" class="block text-sm font-medium text-ink-900">Nama <span class="text-brand-500">*</span></label>
                  <input
                    id="prd-name"
                    v-model="productForm.name"
                    type="text"
                    required
                    placeholder="Banner Flexi (Custom Size)"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                </div>
                <div class="sm:col-span-2">
                  <label for="prd-desc" class="block text-sm font-medium text-ink-900">Deskripsi</label>
                  <textarea
                    id="prd-desc"
                    v-model="productForm.description"
                    rows="2"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  />
                </div>
                <div>
                  <label class="block text-sm font-medium text-ink-900">Pricing type <span class="text-brand-500">*</span></label>
                  <div class="mt-1 flex gap-2">
                    <label
                      :class="[
                        'flex-1 flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors',
                        productForm.pricing_type === 'per_m2'
                          ? 'border-brand-500 bg-brand-50/50 text-ink-950'
                          : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                        productEditingId && 'opacity-60 cursor-not-allowed',
                      ]"
                    >
                      <input v-model="productForm.pricing_type" type="radio" value="per_m2" class="accent-brand-500" :disabled="!!productEditingId">
                      <span class="font-semibold">per_m2</span>
                    </label>
                    <label
                      :class="[
                        'flex-1 flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors',
                        productForm.pricing_type === 'paket'
                          ? 'border-brand-500 bg-brand-50/50 text-ink-950'
                          : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                        productEditingId && 'opacity-60 cursor-not-allowed',
                      ]"
                    >
                      <input v-model="productForm.pricing_type" type="radio" value="paket" class="accent-brand-500" :disabled="!!productEditingId">
                      <span class="font-semibold">paket</span>
                    </label>
                  </div>
                  <p v-if="productEditingId" class="mt-1 text-xs text-ink-500">Pricing type dikunci setelah produk dibuat.</p>
                </div>
                <div>
                  <label for="prd-order" class="block text-sm font-medium text-ink-900">Display order</label>
                  <input
                    id="prd-order"
                    v-model.number="productForm.display_order"
                    type="number"
                    min="0"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                </div>
                <div>
                  <label class="block text-sm font-medium text-ink-900">Ukuran min (cm)</label>
                  <div class="mt-1 grid grid-cols-2 gap-2">
                    <input
                      v-model.number="productForm.min_width_cm"
                      type="number"
                      min="0"
                      placeholder="W"
                      class="rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none"
                    >
                    <input
                      v-model.number="productForm.min_height_cm"
                      type="number"
                      min="0"
                      placeholder="H"
                      class="rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none"
                    >
                  </div>
                </div>
                <div>
                  <label class="block text-sm font-medium text-ink-900">Ukuran max (cm)</label>
                  <div class="mt-1 grid grid-cols-2 gap-2">
                    <input
                      v-model.number="productForm.max_width_cm"
                      type="number"
                      min="0"
                      placeholder="W"
                      class="rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none"
                    >
                    <input
                      v-model.number="productForm.max_height_cm"
                      type="number"
                      min="0"
                      placeholder="H"
                      class="rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none"
                    >
                  </div>
                </div>
              </div>
              <div class="mt-6 flex justify-end gap-2">
                <button
                  type="button"
                  class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50"
                  :disabled="productSaving"
                  @click="productModalOpen = false"
                >
                  Batal
                </button>
                <button
                  type="submit"
                  :disabled="productSaving"
                  class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60"
                >
                  <Loader2 v-if="productSaving" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
                  Simpan
                </button>
              </div>
            </form>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- ============================ Pricing modal ============================ -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="pricingModalOpen && detailProduct" class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="!pricingSaving && (pricingModalOpen = false)" />
          <div role="dialog" aria-modal="true" class="relative w-full max-w-md rounded-lg bg-canvas shadow-xl ring-1 ring-black/5">
            <form class="p-6" @submit.prevent="savePricing">
              <h3 class="font-serif text-lg font-semibold text-ink-950">
                {{ pricingEditingId ? 'Edit pricing' : 'Tambah pricing' }}
              </h3>
              <p class="mt-1 text-xs text-ink-500">
                Produk <strong class="text-ink-900">{{ detailProduct.name }}</strong>
                · type <span class="font-mono">{{ detailProduct.pricing_type }}</span>
              </p>

              <div class="mt-4 space-y-4">
                <div>
                  <label class="block text-sm font-medium text-ink-900">Bahan <span class="text-brand-500">*</span></label>
                  <select
                    v-model="pricingForm.material_id"
                    required
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                    <option value="">Pilih bahan</option>
                    <option v-for="m in activeMaterials" :key="m.id" :value="m.id">
                      {{ m.name }} · {{ m.code }}
                    </option>
                  </select>
                </div>

                <template v-if="detailProduct.pricing_type === 'per_m2'">
                  <div>
                    <label class="block text-sm font-medium text-ink-900">Harga per m² (IDR) <span class="text-brand-500">*</span></label>
                    <input
                      v-model.number="pricingForm.price_per_m2"
                      type="number"
                      min="1"
                      required
                      placeholder="25000"
                      class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                    >
                  </div>
                  <div>
                    <label class="block text-sm font-medium text-ink-900">Minimum charge (m²)</label>
                    <input
                      v-model.number="pricingForm.min_charge_m2"
                      type="number"
                      step="0.01"
                      min="0"
                      placeholder="1.0"
                      class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                    >
                    <p class="mt-1 text-xs text-ink-500">Kosongkan kalau tidak ada minimum. Contoh: 1.0 = customer minimum di-charge 1 m².</p>
                  </div>
                </template>

                <template v-else>
                  <div class="grid grid-cols-2 gap-3">
                    <div>
                      <label class="block text-sm font-medium text-ink-900">Width (cm) <span class="text-brand-500">*</span></label>
                      <input
                        v-model.number="pricingForm.width_cm"
                        type="number"
                        min="1"
                        required
                        placeholder="60"
                        class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                      >
                    </div>
                    <div>
                      <label class="block text-sm font-medium text-ink-900">Height (cm) <span class="text-brand-500">*</span></label>
                      <input
                        v-model.number="pricingForm.height_cm"
                        type="number"
                        min="1"
                        required
                        placeholder="160"
                        class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                      >
                    </div>
                  </div>
                  <div>
                    <label class="block text-sm font-medium text-ink-900">Label paket</label>
                    <input
                      v-model="pricingForm.package_label"
                      type="text"
                      maxlength="100"
                      placeholder="Standard 60×160"
                      class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                    >
                  </div>
                  <div>
                    <label class="block text-sm font-medium text-ink-900">Harga total (IDR) <span class="text-brand-500">*</span></label>
                    <input
                      v-model.number="pricingForm.price_total"
                      type="number"
                      min="1"
                      required
                      placeholder="120000"
                      class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                    >
                  </div>
                </template>
              </div>

              <div class="mt-6 flex justify-end gap-2">
                <button
                  type="button"
                  class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50"
                  :disabled="pricingSaving"
                  @click="pricingModalOpen = false"
                >
                  Batal
                </button>
                <button
                  type="submit"
                  :disabled="pricingSaving || !pricingForm.material_id"
                  class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60"
                >
                  <Loader2 v-if="pricingSaving" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
                  Simpan
                </button>
              </div>
            </form>
          </div>
        </div>
      </Transition>
    </Teleport>
  </section>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
