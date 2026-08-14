// Shape response backend design module (§6). Sinkronkan kalau backend berubah.

export type DesignRole = 'customer_upload' | 'customer_asset' | 'staff_draft'

export type DesignApprovalStatus = 'pending' | 'approved' | 'revision_requested'

export interface DesignFile {
  id: string
  order_id: string
  role: DesignRole | string
  file_original_name: string
  file_size_bytes: number
  file_mime_type: string
  is_previewable: boolean
  notes?: string | null
  uploaded_by?: string | null
  uploaded_at: string
  is_purged: boolean
  purged_at?: string | null
  approval_status?: DesignApprovalStatus | string | null
  reviewed_by?: string | null
  reviewed_at?: string | null
  revision_notes?: string | null
}

export interface DesignFileList {
  items: DesignFile[]
}
