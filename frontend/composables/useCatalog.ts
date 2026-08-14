/**
 * useCatalog — API wrapper untuk modul catalog (§9).
 *
 * Public endpoints (no auth needed, aman untuk fetch di halaman publik & admin).
 * Quote dipakai kalkulator harga POS + form order publik.
 */
import type {
  CatalogMaterial,
  CatalogProduct,
  CatalogProductDetail,
  CatalogQuote,
} from '~/types/catalog'

export function useCatalog() {
  const api = useApi()

  function listProducts(): Promise<{ products: CatalogProduct[] }> {
    return api.get<{ products: CatalogProduct[] }>('/catalog/products')
  }

  function getProduct(slug: string): Promise<CatalogProductDetail> {
    return api.get<CatalogProductDetail>(`/catalog/products/${slug}`)
  }

  function listMaterials(): Promise<{ materials: CatalogMaterial[] }> {
    return api.get<{ materials: CatalogMaterial[] }>('/catalog/materials')
  }

  function quote(body: {
    product_id: string
    material_id: string
    width_cm: number
    height_cm: number
  }): Promise<CatalogQuote> {
    return api.post<CatalogQuote>('/catalog/quote', body)
  }

  return { listProducts, getProduct, listMaterials, quote }
}
