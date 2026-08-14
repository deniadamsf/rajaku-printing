/**
 * useAdminCatalog — API wrapper untuk admin CRUD modul catalog (§9/§10).
 * Semua endpoint di-gate permission catalog.manage di backend.
 */
import type {
  AdminMaterial,
  AdminMaterialInput,
  AdminProduct,
  AdminProductDetail,
  AdminProductInput,
  AdminPricingInput,
  AdminPricingRow,
} from '~/types/catalog-admin'

export function useAdminCatalog() {
  const api = useApi()

  // ---------- Materials ----------
  function listMaterials(): Promise<{ materials: AdminMaterial[] }> {
    return api.get<{ materials: AdminMaterial[] }>('/admin/catalog/materials')
  }
  function createMaterial(body: AdminMaterialInput): Promise<AdminMaterial> {
    return api.post<AdminMaterial>('/admin/catalog/materials', body)
  }
  function updateMaterial(id: string, body: AdminMaterialInput): Promise<AdminMaterial> {
    return api.patch<AdminMaterial>(`/admin/catalog/materials/${id}`, body)
  }
  function activateMaterial(id: string): Promise<{ ok: boolean; is_active: boolean }> {
    return api.post<{ ok: boolean; is_active: boolean }>(`/admin/catalog/materials/${id}/activate`)
  }
  function deactivateMaterial(id: string): Promise<{ ok: boolean; is_active: boolean }> {
    return api.post<{ ok: boolean; is_active: boolean }>(`/admin/catalog/materials/${id}/deactivate`)
  }

  // ---------- Products ----------
  function listProducts(): Promise<{ products: AdminProduct[] }> {
    return api.get<{ products: AdminProduct[] }>('/admin/catalog/products')
  }
  function getProduct(id: string): Promise<AdminProductDetail> {
    return api.get<AdminProductDetail>(`/admin/catalog/products/${id}`)
  }
  function createProduct(body: AdminProductInput): Promise<AdminProduct> {
    return api.post<AdminProduct>('/admin/catalog/products', body)
  }
  function updateProduct(id: string, body: AdminProductInput): Promise<AdminProductDetail> {
    return api.patch<AdminProductDetail>(`/admin/catalog/products/${id}`, body)
  }
  function activateProduct(id: string): Promise<{ ok: boolean; is_active: boolean }> {
    return api.post<{ ok: boolean; is_active: boolean }>(`/admin/catalog/products/${id}/activate`)
  }
  function deactivateProduct(id: string): Promise<{ ok: boolean; is_active: boolean }> {
    return api.post<{ ok: boolean; is_active: boolean }>(`/admin/catalog/products/${id}/deactivate`)
  }

  // ---------- Pricings ----------
  function createPricing(productId: string, body: AdminPricingInput): Promise<AdminPricingRow> {
    return api.post<AdminPricingRow>(`/admin/catalog/products/${productId}/pricings`, body)
  }
  function updatePricing(pid: string, body: AdminPricingInput): Promise<AdminPricingRow> {
    return api.patch<AdminPricingRow>(`/admin/catalog/pricings/${pid}`, body)
  }
  function activatePricing(pid: string): Promise<{ ok: boolean; is_active: boolean }> {
    return api.post<{ ok: boolean; is_active: boolean }>(`/admin/catalog/pricings/${pid}/activate`)
  }
  function deactivatePricing(pid: string): Promise<{ ok: boolean; is_active: boolean }> {
    return api.post<{ ok: boolean; is_active: boolean }>(`/admin/catalog/pricings/${pid}/deactivate`)
  }
  function deletePricing(pid: string): Promise<{ ok: boolean }> {
    return api.delete<{ ok: boolean }>(`/admin/catalog/pricings/${pid}`)
  }

  return {
    listMaterials,
    createMaterial,
    updateMaterial,
    activateMaterial,
    deactivateMaterial,
    listProducts,
    getProduct,
    createProduct,
    updateProduct,
    activateProduct,
    deactivateProduct,
    createPricing,
    updatePricing,
    activatePricing,
    deactivatePricing,
    deletePricing,
  }
}
