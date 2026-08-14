// Shape response backend catalog module.

export type PricingType = 'per_m2' | 'paket'

export interface CatalogMaterial {
  id: string
  code: string
  name: string
  description?: string
}

export interface CatalogProduct {
  id: string
  slug: string
  name: string
  description?: string
  category: string
  pricing_type: PricingType | string
  display_order: number
}

export interface CatalogPricingRow {
  material_id: string
  material_name: string
  material_code: string
  price_per_m2?: number
  min_charge_m2?: number
  width_cm?: number
  height_cm?: number
  package_label?: string
  price_total?: number
}

export interface CatalogProductDetail {
  id: string
  slug: string
  name: string
  description?: string
  category: string
  pricing_type: PricingType | string
  min_width_cm?: number
  min_height_cm?: number
  max_width_cm?: number
  max_height_cm?: number
  pricings: CatalogPricingRow[]
}

export interface CatalogQuote {
  product_id: string
  product_name: string
  material_id: string
  material_name: string
  pricing_type: PricingType | string
  width_cm: number
  height_cm: number
  total_price: number
  area_m2?: number
  chargeable_m2?: number
  price_per_m2?: number
  package_label?: string
}
