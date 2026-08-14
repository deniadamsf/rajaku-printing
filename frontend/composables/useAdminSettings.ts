/**
 * useAdminSettings — API wrapper untuk setting global aplikasi (§19).
 *
 * Semua endpoint di-gate permission `settings.manage` di backend (hanya
 * super_admin yang punya secara default).
 */

export interface AppSetting {
  key: string
  value: string
  display_name: string
  description?: string
  updated_at: string
  updated_by?: string
}

/** Key setting yang dikenal — samakan dengan settingsapi di backend. */
export const SETTING_DESIGN_RETENTION_DAYS = 'design_retention_days'

export function useAdminSettings() {
  const api = useApi()

  function list(): Promise<{ items: AppSetting[] }> {
    return api.get<{ items: AppSetting[] }>('/admin/settings')
  }

  function update(key: string, value: string): Promise<AppSetting> {
    return api.put<AppSetting>(`/admin/settings/${key}`, { value })
  }

  return { list, update }
}
