import type { GroupsListResponse } from './api'
import { el } from './dom'
import type { AppContext } from './types'

const GROUP_IDS_KEY = 'group_ids'
const NAME_PREFIX = 6
const DATE_OPTIONS: Intl.DateTimeFormatOptions = {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit'
}

/** A group row after normalization: everything the card renders, nothing else. */
type Row = Readonly<{
  openid: string
  name: string
  nameUnavailable: boolean
  count: number
  lastSeen: number
  lastSeenText: string
}>

function asString(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function asCount(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? Math.floor(value) : 0
}

/**
 * group_ids is a free-form textarea: lines, comma-separated pieces, `#` comment
 * lines, blank lines. The plugin's splitSettingList reads it exactly this way
 * (backend/config.go), so the card edits it the same way instead of inventing a
 * second format.
 */
function pieces(line: string): string[] {
  return line.split(',').map(piece => piece.trim())
}

/** Whether the draft already lists this group id (the checkbox state derives from this). */
export function draftHasId(text: string, openid: string): boolean {
  return text.split('\n').some(line => pieces(line).some(item => item === openid))
}

/** Tick: append one line, leaving everything already in the draft untouched. */
export function draftAddId(text: string, openid: string): string {
  const separator = text === '' || text.endsWith('\n') ? '' : '\n'
  return `${text}${separator}${openid}`
}

/**
 * Untick: drop only the matching pieces. A line that held nothing else
 * disappears, a line that held more ids keeps them, and `#` comments stay put.
 */
export function draftRemoveId(text: string, openid: string): string {
  const kept: string[] = []
  for (const line of text.split('\n')) {
    const parts = pieces(line)
    if (!parts.some(item => item === openid)) {
      kept.push(line)
      continue
    }
    const rest = parts.filter(item => item !== openid && item !== '')
    if (rest.length > 0) kept.push(rest.join(', '))
  }
  const joined = kept.join('\n')
  // The dropped line may have been the last one, taking the trailing newline
  // with it; put it back so an untick does not visibly reflow the textarea.
  return text.endsWith('\n') && joined !== '' && !joined.endsWith('\n') ? `${joined}\n` : joined
}

function normalizeGroups(value: unknown): Row[] {
  if (!Array.isArray(value)) return []
  const rows: Row[] = []
  for (const entry of value) {
    if (!entry || typeof entry !== 'object') continue
    const source = entry as Record<string, unknown>
    const openid = asString(source.openid).trim()
    if (openid === '') continue
    const name = asString(source.name).trim()
    const lastSeenAt = asString(source.lastSeenAt)
    const parsed = Date.parse(lastSeenAt)
    rows.push({
      openid,
      name,
      nameUnavailable: source.nameUnavailable === true,
      count: asCount(source.count),
      // Unparseable timestamps sink to the end but keep their relative order.
      lastSeen: Number.isFinite(parsed) ? parsed : Number.NEGATIVE_INFINITY,
      lastSeenText: lastSeenAt
    })
  }
  // Newest first, as the operator scans for the group they just used.
  return rows.sort((left, right) => right.lastSeen - left.lastSeen)
}

export function createGroupsCard(ctx: AppContext): Readonly<{ root: HTMLElement, destroy: () => void }> {
  const titleId = 'qqbot-groups-title'
  const status = el('p', { class: 'qqbot-groups__status', role: 'status', 'aria-live': 'polite' })
  const note = el('p', { class: 'qqbot-groups__note' })
  note.hidden = true
  const list = el('ul', { class: 'qqbot-groups__list' })
  list.hidden = true
  const refresh = el('button', { type: 'button', class: 'qqbot-btn qqbot-btn--ghost qqbot-groups__refresh' }, ctx.t('groups.refresh'))

  const root = el(
    'div',
    { class: 'qqbot-groups', role: 'group', 'aria-labelledby': titleId },
    el(
      'header',
      { class: 'qqbot-groups__head' },
      el(
        'div',
        { class: 'qqbot-groups__heading' },
        el('h4', { class: 'qqbot-groups__title', id: titleId }, ctx.t('groups.title')),
        el('p', { class: 'qqbot-groups__hint' }, ctx.t('groups.hint'))
      ),
      refresh
    ),
    note,
    list,
    status
  )

  const boxes = new Map<string, HTMLInputElement>()
  let loading = false
  let sequence = 0

  const setNote = (text: string) => {
    note.textContent = text
    note.hidden = text === ''
  }

  const setStatus = (text: string) => {
    status.textContent = text
  }

  /** Checkbox state is never stored: it is read back from the draft every time. */
  const syncBoxes = () => {
    const text = ctx.getValue(GROUP_IDS_KEY)
    for (const [openid, box] of boxes) box.checked = draftHasId(text, openid)
  }

  const toggle = (openid: string, box: HTMLInputElement) => {
    const text = ctx.getValue(GROUP_IDS_KEY)
    // Writing through the context keeps the textarea, the draft and the host's
    // own form values in one place — group_ids stays the single source of truth.
    ctx.setValue(GROUP_IDS_KEY, box.checked ? draftAddId(text, openid) : draftRemoveId(text, openid))
    syncBoxes()
  }

  // The contract makes nameUnavailable the authority on "there is no name";
  // the empty-string check is only a belt-and-braces guard.
  const displayName = (row: Row): string =>
    row.name !== '' && !row.nameUnavailable ? row.name : `${row.openid.slice(0, NAME_PREFIX)}…`

  const displayTime = (row: Row): string => {
    const parsed = Date.parse(row.lastSeenText)
    if (!Number.isFinite(parsed)) return row.lastSeenText
    return new Date(parsed).toLocaleString(ctx.zh ? 'zh-CN' : 'en-US', DATE_OPTIONS)
  }

  const renderRows = (rows: readonly Row[]) => {
    boxes.clear()
    list.replaceChildren()
    rows.forEach((row, index) => {
      const id = `qqbot-group-${index}`
      const box = el('input', { type: 'checkbox', id, class: 'qqbot-groups__box' })
      box.checked = draftHasId(ctx.getValue(GROUP_IDS_KEY), row.openid)
      box.addEventListener('change', () => toggle(row.openid, box))
      boxes.set(row.openid, box)
      list.append(
        el(
          'li',
          { class: 'qqbot-groups__row' },
          box,
          el(
            'label',
            { class: 'qqbot-groups__label', for: id },
            el('span', { class: 'qqbot-groups__name', title: row.openid }, displayName(row)),
            el('span', { class: 'qqbot-groups__meta' }, ctx.t('groups.rowMeta', { time: displayTime(row), count: row.count }))
          )
        )
      )
    })
    list.hidden = rows.length === 0
  }

  const render = (response: GroupsListResponse) => {
    const rows = normalizeGroups(response?.groups)
    const claimEnabled = response?.claimEnabled !== false
    const limit = asCount(response?.recordingLimit)
    renderRows(rows)
    if (!claimEnabled) {
      // Nothing new is being recorded, so the guidance replaces the empty copy.
      setNote(ctx.t('groups.claimOff'))
    } else if (rows.length === 0) {
      setNote(ctx.t('groups.empty'))
    } else if (limit > 0 && rows.length >= limit) {
      setNote(ctx.t('groups.limit', { n: limit }))
    } else {
      setNote('')
    }
    setStatus(ctx.t('groups.updated', { n: rows.length }))
  }

  /**
   * The route may not exist yet (older backend, plugin mid-upgrade). That is a
   * card-level fact, not a settings-page failure: it must never raise the
   * banner, block saving, or leave the page half-rendered.
   */
  const renderUnavailable = () => {
    boxes.clear()
    list.replaceChildren()
    list.hidden = true
    setNote(ctx.t('groups.unavailable'))
    setStatus(ctx.t('groups.unavailable'))
  }

  const load = async (): Promise<void> => {
    if (loading || ctx.disposed) return
    loading = true
    const mine = ++sequence
    refresh.disabled = true
    refresh.textContent = ctx.t('groups.refreshing')
    setStatus(ctx.t('groups.loading'))
    try {
      // bridge.request directly, not ctx.request: the shared helper paints the
      // page-level error banner on failure, and this card degrades on its own.
      const response = await ctx.bridge.request<GroupsListResponse>('/groups/list', {
        method: 'POST',
        body: {},
        responseMode: 'body'
      })
      if (mine !== sequence || ctx.disposed) return
      render(response)
    } catch {
      if (mine !== sequence || ctx.disposed) return
      renderUnavailable()
    } finally {
      if (mine === sequence && !ctx.disposed) {
        loading = false
        refresh.disabled = false
        refresh.textContent = ctx.t('groups.refresh')
      }
    }
  }

  refresh.addEventListener('click', () => { void load() })
  // Hand edits in the textarea move the checkboxes, not the other way round.
  ctx.onInput([GROUP_IDS_KEY], syncBoxes)
  void load()

  return {
    root,
    destroy: () => {
      sequence += 1
      boxes.clear()
      root.remove()
    }
  }
}
