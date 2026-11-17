import type { AdminExtensionSettingItem, AdminMicroFrontendBridgeV1 } from '@sforum/admin-sdk'
import type { I18n, MessageKey } from './i18n'

/**
 * The host settings payload carries `groupId` (schema-document group id) and
 * `width` (schema control width), but the published @sforum/admin-sdk type has
 * not caught up with them yet. Verified against
 * apps/api/app/Models/Extensions/types.go (ExtensionSettingValue).
 */
export type SettingItem = AdminExtensionSettingItem & Readonly<{
  groupId?: string
  width?: string
}>

export type FieldControl = HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement

export type FieldHandle = Readonly<{
  root: HTMLElement
  control: FieldControl
  item: SettingItem
}>

export type AppContext = {
  bridge: AdminMicroFrontendBridgeV1
  i18n: I18n
  t: (key: MessageKey, params?: Record<string, string | number>) => string
  zh: boolean
  draft: Record<string, string>
  /**
   * The saved baseline the preview compares the draft against.
   *
   * Neither host-side source can supply it: `settings.values()` is the live form
   * state our own writes keep in sync (page passes `values: formValues`), and
   * `settings.items` is a snapshot taken when the component was mounted
   * (SFTrustedSettingsComponent.vue:141 + :223, never refreshed because
   * renderKey()/:168-177 and the watch at :271 do not include settings).
   *
   * So the authority is the saved preview's `savedSnapshot` — the server's own
   * config, echoed back only for `source=saved`. `item.value` from the mount
   * payload is merely the fallback until that first answer arrives. Only the
   * preview card writes here.
   */
  savedValues: Record<string, string>
  disposed: boolean
  secretSaved: boolean
  item: (key: string) => SettingItem | undefined
  request: <T>(path: string, options?: Record<string, unknown>) => Promise<T>
  toast: (kind: 'success' | 'error', title: string, description?: string) => void
  getValue: (key: string) => string
  /** Draft write used by the component itself (bind write-back, host sync). */
  setValue: (key: string, value: string, syncControl?: boolean) => void
  registerControl: (key: string, control: FieldControl, read: () => string) => void
  onInput: (keys: readonly string[], listener: () => void) => void
  focus: (key: string) => void
  /** Re-read the host draft after save/reset so the form never shows stale values. */
  syncFromBridge: () => void
  /**
   * Fires after a save/reset *attempt*: the host swallows failures (it only
   * raises a toast), so this is not a success signal. It asks the preview to
   * re-ask the saved source, whose `savedSnapshot` then tells us the truth.
   */
  onSavedChange: (listener: () => void) => void
  markSecretSaved: () => void
  onSecretSavedChange: (listener: () => void) => void
}
