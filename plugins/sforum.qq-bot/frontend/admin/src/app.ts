import type { AdminMicroFrontendBridgeV1 } from '@sforum/admin-sdk'
import { createBindCard } from './bind-card'
import { el } from './dom'
import { createFooter } from './footer'
import { createSection } from './form'
import { createGroupsCard } from './groups-card'
import { createI18n } from './i18n'
import type { MessageKey } from './i18n'
import { createTemplateSection } from './template'
import type { AppContext, FieldControl, SettingItem } from './types'

const GROUP_TITLE: Record<string, MessageKey> = {
  connection: 'group.connection',
  targets: 'group.targets',
  template: 'group.template',
  limits: 'group.limits',
  probe: 'group.probe'
}

const GROUP_HINT: Record<string, MessageKey> = {
  connection: 'group.connection.hint',
  targets: 'group.targets.hint',
  template: 'group.template.hint',
  limits: 'group.limits.hint',
  probe: 'group.probe.hint'
}

const GROUP_ORDER: readonly string[] = ['connection', 'targets', 'template', 'limits', 'probe']
const PREVIEW_KEYS: readonly string[] = ['message_template', 'message_format', 'signature', 'excerpt_limit', 'max_message_chars']

type BannerKind = 'route' | 'permission' | 'rate' | 'service'

const BANNER_KEYS: Record<BannerKind, Readonly<{ title: MessageKey, detail: MessageKey }>> = {
  route: { title: 'error.routeTitle', detail: 'error.routeDetail' },
  permission: { title: 'error.permissionTitle', detail: 'error.permissionDetail' },
  rate: { title: 'error.rateTitle', detail: 'error.rateDetail' },
  service: { title: 'error.serviceTitle', detail: 'error.serviceDetail' }
}

/**
 * The host request layer rejects with its own error object; the HTTP status can
 * live on a few shapes depending on where the failure happened. Read it
 * defensively and never surface the raw message.
 */
function errorStatus(value: unknown): number | undefined {
  if (!value || typeof value !== 'object') return undefined
  const source = value as {
    status?: unknown
    statusCode?: unknown
    response?: { status?: unknown }
    cause?: { status?: unknown }
  }
  for (const candidate of [source.status, source.statusCode, source.response?.status, source.cause?.status]) {
    if (typeof candidate === 'number' && candidate >= 400 && candidate <= 599) return candidate
  }
  return undefined
}

function classifyError(value: unknown): BannerKind {
  const status = errorStatus(value)
  // No status at all means the request never reached the plugin (offline,
  // aborted, or the route does not exist yet): that is the "enable it first"
  // case, not a server fault.
  if (status === undefined || status === 404) return 'route'
  if (status === 401 || status === 403) return 'permission'
  if (status === 429) return 'rate'
  return 'service'
}

export type SettingsApp = Readonly<{ root: HTMLElement, destroy: () => void }>

export function createApp(bridge: AdminMicroFrontendBridgeV1): SettingsApp {
  const i18n = createI18n(bridge.locale)
  const t = i18n.t

  // The host payload also carries groupId/width; see SettingItem.
  const itemList = (bridge.settings.items ?? []) as readonly SettingItem[]
  const items = new Map<string, SettingItem>()
  const draft: Record<string, string> = {}
  for (const item of itemList) {
    items.set(item.key, item)
    draft[item.key] = typeof item.value === 'string' ? item.value : (item.default ?? '')
  }

  // One banner, four honest copies: "not enabled" and "you lack the permission"
  // are different problems for the operator, and a 429 asks for patience rather
  // than a settings change.
  const bannerTitle = el('strong')
  const bannerDetail = el('span', { class: 'qqbot-banner__detail' })
  const banner = el('div', { class: 'qqbot-banner', role: 'alert' }, bannerTitle, bannerDetail)
  banner.hidden = true

  const showBanner = (kind: BannerKind) => {
    const keys = BANNER_KEYS[kind]
    bannerTitle.textContent = t(keys.title)
    bannerDetail.textContent = t(keys.detail)
    banner.hidden = false
  }

  const controls = new Map<string, FieldControl>()
  const inputListeners = new Map<string, Set<() => void>>()
  const secretListeners = new Set<() => void>()

  const notifyInput = (key: string) => {
    const listeners = inputListeners.get(key)
    if (!listeners) return
    for (const listener of listeners) listener()
  }

  const writeControl = (control: FieldControl, value: string) => {
    if (control instanceof HTMLInputElement && control.type === 'checkbox') {
      control.checked = value === 'true'
      return
    }
    if (control.value !== value) control.value = value
  }

  const ctx: AppContext = {
    bridge,
    i18n,
    t,
    zh: i18n.zh,
    draft,
    disposed: false,
    // secretSet is the host telling us "a secret is stored"; the value itself is
    // never sent back. The input stays empty and the draft keeps the host value.
    secretSaved: itemList.some(item => item.key === 'app_secret' && item.secretSet === true),
    item: key => items.get(key),
    async request<T>(path: string, options: Record<string, unknown> = {}): Promise<T> {
      try {
        // The settings bridge forwards options to the host client but (unlike
        // the admin-page bridge) does not pin responseMode. Plugin routes return
        // their own bare JSON, so ask for the whole body explicitly; otherwise
        // the host unwraps it as a `{ code, message, data }` envelope.
        const result = await bridge.request<T>(path, { ...options, responseMode: 'body' })
        banner.hidden = true
        return result
      } catch (error) {
        showBanner(classifyError(error))
        throw error
      }
    },
    toast: (kind, title, description) => bridge.toast({ kind, title, description }),
    getValue: key => draft[key] ?? '',
    setValue: (key, value, syncControl = true) => {
      draft[key] = value
      // The host persists its own draft (formValues), so every programmatic
      // write has to go through updateValue or save() would persist stale data.
      bridge.settings.updateValue(key, value)
      const control = controls.get(key)
      if (syncControl && control) writeControl(control, value)
    },
    registerControl: (key, control, read) => {
      controls.set(key, control)
      const sync = () => {
        const next = read()
        if (draft[key] !== next) {
          draft[key] = next
          bridge.settings.updateValue(key, next)
        }
        notifyInput(key)
      }
      control.addEventListener('input', sync)
      control.addEventListener('change', sync)
    },
    onInput: (keys, listener) => {
      for (const key of keys) {
        const set = inputListeners.get(key) ?? new Set<() => void>()
        set.add(listener)
        inputListeners.set(key, set)
      }
    },
    focus: key => controls.get(key)?.focus(),
    syncFromBridge: () => {
      const values = bridge.settings.values()
      let previewDirty = false
      for (const key of Object.keys(draft)) {
        const next = values[key]
        if (typeof next !== 'string' || next === draft[key]) continue
        draft[key] = next
        const control = controls.get(key)
        if (control) writeControl(control, next)
        if (PREVIEW_KEYS.includes(key)) previewDirty = true
      }
      if (previewDirty) notifyInput('message_template')
    },
    markSecretSaved: () => {
      ctx.secretSaved = true
      for (const listener of secretListeners) listener()
    },
    onSecretSavedChange: listener => { secretListeners.add(listener) }
  }
  const bind = createBindCard(ctx)
  const template = createTemplateSection(ctx)
  const groups = createGroupsCard(ctx)
  const footer = createFooter(ctx)

  const buckets = new Map<string, SettingItem[]>()
  for (const item of itemList) {
    const groupId = (item.groupId || '').trim() || 'other'
    const bucket = buckets.get(groupId)
    if (bucket) bucket.push(item)
    else buckets.set(groupId, [item])
  }
  const orderedGroups = [
    ...GROUP_ORDER.filter(groupId => buckets.has(groupId)),
    ...[...buckets.keys()].filter(groupId => !GROUP_ORDER.includes(groupId))
  ]

  const sections: HTMLElement[] = []
  for (const groupId of orderedGroups) {
    const groupItems = buckets.get(groupId) ?? []
    if (groupId === 'template') {
      sections.push(template.root)
      continue
    }
    const title = groupId === 'other'
      ? t('group.other')
      : GROUP_TITLE[groupId] ? t(GROUP_TITLE[groupId]) : groupId
    const hint = groupId === 'other'
      ? t('group.other.hint')
      : GROUP_HINT[groupId] ? t(GROUP_HINT[groupId]) : undefined
    const section = createSection(ctx, { id: groupId, keys: groupItems.map(item => item.key), title, hint })
    if (groupId === 'targets') {
      // The card sits directly above the group ID list it writes into; it is a
      // helper for that field, not a setting of its own.
      const anchor = section.querySelector('[data-key="group_ids"]')
      if (anchor?.parentElement) anchor.parentElement.insertBefore(groups.root, anchor)
    }
    sections.push(section)
  }

  const root = el('div', { class: 'qqbot-settings' }, banner, bind.root, ...sections, footer.root)

  return {
    root,
    destroy: () => {
      ctx.disposed = true
      bind.destroy()
      template.stop()
      groups.destroy()
      footer.destroy()
      root.remove()
    }
  }
}
