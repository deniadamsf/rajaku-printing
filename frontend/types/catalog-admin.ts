// Admin-only shape untuk modul catalog. Public shape ada di catalog.ts.

import type { PricingType } from './catalog'

export interface AdminMaterial {
  id: string
  code: string
  name: string
  description?: string
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface AdminMaterialInput {
  code: string
  name: string
  description?: string
}

export interface AdminProduct {
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
  is_active: boolean
  display_order: number
  created_at: string
  updated_at: string
}

export interface AdminPricingRow {
  id: string
  product_id: string
  material_id: string
  material_name?: string
  material_code?: string
  price_per_m2?: number
  min_charge_m2?: number
  width_cm?: number
  height_cm?: number
  package_label?: string
  price_total?: number
  is_active: boolean
}

export interface AdminProductDetail extends AdminProduct {
  pricings: AdminPricingRow[]
}

export interface AdminProductInput {
  slug: string
  name: string
  description?: string
  category: string
  pricing_type?: PricingType | string
  min_width_cm?: number | null
  min_height_cm?: number | null
  max_width_cm?: number | null
  max_height_cm?: number | null
  display_order?: number
}

export interface AdminPricingInput {
  material_id: string
  // per_m2
  price_per_m2?: number
  min_charge_m2?: number
  // paket
  width_cm?: number
  height_cm?: number
  package_label?: string
  price_total?: number
}
