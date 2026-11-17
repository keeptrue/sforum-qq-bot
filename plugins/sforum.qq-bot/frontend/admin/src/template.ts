import type { PreviewResponse } from './api'
import { el } from './dom'
import { createSection, renderField } from './form'
import type { MessageKey } from './i18n'
import type { AppContext } from './types'

type Variable = Readonly<{ token: string, alias: string }>

type Preset = Readonly<{ label: MessageKey, text: string | null }>

// The nine variables the backend renders, in the order the manifest lists them.
const VARIABLES: readonly Variable[] = [
  { token: 'title', alias: '标题' },
  { token: 'author', alias: '作者' },
  { token: 'category', alias: '分类' },
  { token: 'tags', alias: '标签' },
  { token: 'excerpt', alias: '摘要' },
  { token: 'url', alias: '链接' },
  { token: 'topicId', alias: '主题ID' },
  { token: 'date', alias: '日期' },
  { token: 'signature', alias: '签名' }
]

const PREVIEW_KEYS = ['message_template', 'message_format', 'signature', 'excerpt_limit', 'max_message_chars'] as const
const PREVIEW_DEBOUNCE_MS = 300

// Presets are message bodies, not UI chrome: they stay in the language the
// manifest writes its default in, so a preset reads the same on either UI
// locale. They are component constants and never settings — a click only
// rewrites the message_template draft.
// "Default" carries no text of its own: it reads message_template.default from
// the host payload, so the button can never drift from what "Restore defaults"
// produces.
const PRESETS: readonly Preset[] = [
  { label: 'template.presetDefault', text: null },
  { label: 'template.presetCompact', text: '【新帖】{{title}}\n{{url}}' },
  {
    label: 'template.presetNotice',
    text: '【社区公告】{{title}}\n\n{{excerpt}}\n\n分类：{{category}}｜作者：{{author}}\n详情：{{url}}\n{{signature}}'
  }
]

// Sample overrides: the operator types what the built-in sample should say.
// They live in this component only — never a setting, never persisted.
const SAMPLE_FIELDS: readonly Readonly<{ token: string, label: MessageKey }>[] = [
  { token: 'title', label: 'template.sampleTitle' },
  { token: 'author', label: 'template.sampleAuthor' },
  { token: 'category', label: 'template.sampleCategory' },
  { token: 'excerpt', label: 'template.sampleExcerpt' },
  { token: 'url', label: 'template.sampleUrl' }
]

// An empty or non-numeric field must be omitted, not guessed: the route falls
// back to the saved value for any key the request leaves out.
function optionalInt(value: string): number | undefined {
  if (value.trim() === '') return undefined
  const parsed = Number.parseInt(value, 10)
  return Number.isFinite(parsed) ? parsed : undefined
}

/**
 * One comparison unit for "what does this field say".
 *
 * The draft is always text (it comes from inputs), while `savedSnapshot` sends
 * excerpt_limit / max_message_chars as integers and the rest as strings. Both
 * sides go through this function, so `80` and `'80'` compare equal and a type
 * difference can never fake a dirty state.
 */
function fieldText(value: unknown): string | null {
  if (typeof value === 'string') return value
  if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  return null
}

function insertAtCursor(control: HTMLTextAreaElement, token: string): void {
  const focused = document.activeElement === control
  const start = focused ? (control.selectionStart ?? control.value.length) : control.value.length
  const end = focused ? (control.selectionEnd ?? start) : start
  control.value = `${control.value.slice(0, start)}${token}${control.value.slice(end)}`
  control.dispatchEvent(new Event('input', { bubbles: true }))
  const caret = start + token.length
  control.focus()
  control.setSelectionRange(caret, caret)
}

function chips(ctx: AppContext, getControl: () => HTMLTextAreaElement): HTMLElement {
  const row = el('div', { class: 'qqbot-chips' })
  for (const variable of VARIABLES) {
    const token = `{{${variable.token}}}`
    const button = el(
      'button',
      {
        type: 'button',
        class: 'qqbot-chip',
        title: ctx.zh ? `${token} / {{${variable.alias}}}` : token,
        'aria-label': ctx.zh ? `${token}（${variable.alias}）` : token
      },
      el('code', {}, token),
      ctx.zh ? el('span', { class: 'qqbot-chip__alias' }, variable.alias) : null
    )
    button.addEventListener('click', () => insertAtCursor(getControl(), token))
    row.append(button)
  }
  return el(
    'div',
    { class: 'qqbot-tokens' },
    el('p', { class: 'qqbot-tokens__title' }, ctx.t('template.tokens')),
    row,
    el('p', { class: 'qqbot-tokens__hint' }, ctx.t('template.tokensHint'))
  )
}

function presets(ctx: AppContext, getControl: () => HTMLTextAreaElement): HTMLElement {
  const titleId = 'qqbot-presets-title'
  const row = el('div', { class: 'qqbot-presets__row', role: 'group', 'aria-labelledby': titleId })
  const buttons: HTMLButtonElement[] = []
  const clearPressed = () => {
    for (const button of buttons) button.setAttribute('aria-pressed', 'false')
  }

  const manifestDefault = ctx.item('message_template')?.default ?? ''

  for (const preset of PRESETS) {
    const text = preset.text ?? manifestDefault
    // No manifest default means there is nothing to restore.
    if (text === '') continue
    const button = el('button', { type: 'button', class: 'qqbot-preset', 'aria-pressed': 'false' }, ctx.t(preset.label))
    button.addEventListener('click', () => {
      const control = getControl()
      // The input event is the same write path the variable chips use: app.ts
      // mirrors the value into the host draft and reschedules the preview.
      // Skipped when the text already matches, so a repeat click cannot spend
      // the preview rate limit for nothing.
      if (control.value !== text) {
        control.value = text
        control.dispatchEvent(new Event('input', { bubbles: true }))
      }
      clearPressed()
      button.setAttribute('aria-pressed', 'true')
      control.focus()
      control.setSelectionRange(text.length, text.length)
    })
    buttons.push(button)
    row.append(button)
  }

  // Typing by hand means the text is no longer one of the presets.
  getControl().addEventListener('input', clearPressed)

  return el(
    'div',
    { class: 'qqbot-presets' },
    el('p', { class: 'qqbot-presets__title', id: titleId }, ctx.t('template.presets')),
    row
  )
}

function createPreview(ctx: AppContext) {
  const titleId = 'qqbot-preview-title'
  const body = el('pre', { class: 'qqbot-preview__body' })
  const count = el('span', { class: 'qqbot-count' })
  const badge = el('span', { class: 'qqbot-badge' })
  const truncated = el('span', { class: 'qqbot-badge qqbot-badge--warn' }, ctx.t('template.truncated'))
  truncated.hidden = true
  const unknown = el('p', { class: 'qqbot-preview__unknown' })
  unknown.hidden = true
  const dirtyText = el('p', { class: 'qqbot-preview__dirty' })
  const toggle = el('button', { type: 'button', class: 'qqbot-btn qqbot-btn--ghost qqbot-preview__mode' })
  const notice = el('div', { class: 'qqbot-preview__notice' }, dirtyText, toggle)
  notice.hidden = true
  const sampleNote = el('span', { class: 'qqbot-preview__sampled' }, ctx.t('template.sampleUsed'))
  sampleNote.hidden = true

  const sampleValues: Record<string, string> = {}
  const sampleGrid = el('div', { class: 'qqbot-preview__sample-grid' })
  for (const field of SAMPLE_FIELDS) {
    sampleValues[field.token] = ''
    const id = `qqbot-sample-${field.token}`
    const input = el('input', {
      type: 'text',
      id,
      class: 'qqbot-input',
      autocomplete: 'off',
      spellcheck: 'false',
      placeholder: `{{${field.token}}}`
    })
    input.addEventListener('input', () => {
      sampleValues[field.token] = input.value
      schedule()
    })
    sampleGrid.append(
      el(
        'div',
        { class: 'qqbot-preview__sample-field' },
        el('label', { for: id }, ctx.t(field.label)),
        input
      )
    )
  }
  const sampleSection = el(
    'details',
    { class: 'qqbot-preview__sample' },
    el('summary', { class: 'qqbot-preview__sample-summary' }, ctx.t('template.sample')),
    el('p', { class: 'qqbot-preview__sample-hint' }, ctx.t('template.sampleHint')),
    sampleGrid
  )
  // Screen-reader announcement only: the visible character count is right here.
  const live = el('p', { class: 'qqbot-sr-only', role: 'status', 'aria-live': 'polite' })

  const root = el(
    'div',
    { class: 'qqbot-preview', 'aria-labelledby': titleId },
    el(
      'header',
      { class: 'qqbot-preview__head' },
      el('h4', { class: 'qqbot-preview__title', id: titleId }, ctx.t('template.preview')),
      badge,
      sampleNote,
      count,
      truncated
    ),
    notice,
    sampleSection,
    body,
    unknown,
    live
  )

  let timer: number | undefined
  let sequence = 0
  let mode: 'saved' | 'draft' = 'saved'
  // The last payload actually asked for. Saved mode sends a constant payload,
  // so without this every keystroke would re-ask the rate-limited route for an
  // answer that cannot have changed.
  let lastKey = ''
  let lastOk = false

  // PREVIEW_KEYS is also the set of keys that change what gets rendered, which
  // is what the saved/draft comparison has to look at.
  const isDirty = () => PREVIEW_KEYS.some(key => fieldText(ctx.getValue(key)) !== fieldText(ctx.savedValues[key]))

  /** Repaints the mode badge and the dirty hint; returns whether the draft is being previewed. */
  const paint = () => {
    const dirty = isDirty()
    const draftMode = dirty && mode === 'draft'
    badge.textContent = ctx.t(draftMode ? 'template.previewDraftBadge' : 'template.previewSavedBadge')
    badge.classList.toggle('qqbot-badge--quiet', draftMode)
    notice.hidden = !dirty
    toggle.hidden = !dirty
    if (dirty) {
      dirtyText.textContent = ctx.t(draftMode ? 'template.previewDirtyDraft' : 'template.previewDirty')
      toggle.textContent = ctx.t(draftMode ? 'template.previewUseSaved' : 'template.previewUseDraft')
    }
    return draftMode
  }

  /** Only the fields the operator actually filled; nothing at all when all are blank. */
  const sampleOf = (): Record<string, string> | undefined => {
    const sample: Record<string, string> = {}
    for (const field of SAMPLE_FIELDS) {
      const value = sampleValues[field.token] ?? ''
      if (value.trim() !== '') sample[field.token] = value
    }
    return Object.keys(sample).length > 0 ? sample : undefined
  }

  /**
   * Adopt the server's own saved configuration as the baseline. A missing or
   * malformed snapshot leaves the previous baseline untouched: guessing here is
   * exactly what the savedSnapshot contract exists to avoid, and a stale
   * baseline fails safe (the dirty hint stays visible).
   */
  const applySnapshot = (value: unknown) => {
    // Present-or-not is decided by the field itself: a draft answer simply has
    // no savedSnapshot (not null), so nothing here may treat "empty" as truthy.
    if (value === undefined || value === null || typeof value !== 'object') return
    const snapshot = value as Record<string, unknown>
    for (const key of PREVIEW_KEYS) {
      const next = fieldText(snapshot[key])
      if (next !== null) ctx.savedValues[key] = next
    }
    // The baseline just moved, so the dirty hint has to be recomputed.
    paint()
  }

  /**
   * Saved mode carries no draft field at all: the route renders the stored
   * settings, which is what a push would actually send.
   */
  const buildPayload = (draftMode: boolean): Record<string, unknown> => {
    const sample = sampleOf()
    // sample is orthogonal to the source: it replaces the built-in sample in
    // both modes, so it rides along either way.
    const withSample = sample === undefined ? {} : { sample }
    if (!draftMode) return { source: 'saved', ...withSample }
    const excerptLimit = optionalInt(ctx.getValue('excerpt_limit'))
    const maxMessageChars = optionalInt(ctx.getValue('max_message_chars'))
    return {
      source: 'draft',
      template: ctx.getValue('message_template'),
      format: ctx.getValue('message_format') || 'text',
      signature: ctx.getValue('signature'),
      ...(excerptLimit === undefined ? {} : { excerptLimit }),
      ...(maxMessageChars === undefined ? {} : { maxMessageChars }),
      ...withSample
    }
  }

  const setText = (text: string, characters: number) => {
    const empty = text === ''
    body.classList.toggle('is-empty', empty)
    body.textContent = empty ? ctx.t('template.previewEmpty') : text
    count.textContent = ctx.t('template.count', { n: characters })
    live.textContent = ctx.t('template.live', { n: characters })
  }

  const run = async () => {
    const mine = ++sequence
    const draftMode = paint()
    const payload = buildPayload(draftMode)
    const key = JSON.stringify(payload)
    if (key === lastKey && lastOk) return
    lastKey = key
    lastOk = false
    try {
      // bridge.request rather than ctx.request: a preview failure degrades to
      // the one line below and must not paint the page-level error banner.
      const response = await ctx.bridge.request<PreviewResponse>('/preview', {
        method: 'POST',
        body: payload,
        responseMode: 'body'
      })
      if (mine !== sequence || ctx.disposed) return
      const text = typeof response?.text === 'string' ? response.text : ''
      const variables = Array.isArray(response?.unknownVariables)
        ? response.unknownVariables.filter((value): value is string => typeof value === 'string' && value !== '')
        : []
      // The route counts characters after truncation; only fall back to the
      // local count when it does not report one (older backend).
      const reported = typeof response?.characters === 'number' && Number.isFinite(response.characters) && response.characters >= 0
        ? Math.floor(response.characters)
        : Array.from(text).length
      setText(text, reported)
      truncated.hidden = response?.truncated !== true
      sampleNote.hidden = response?.sampleUsed !== true
      // Only a saved answer can move the baseline; draft answers never carry a
      // snapshot, and one that did would still be ignored here.
      if (!draftMode) applySnapshot(response?.savedSnapshot)
      unknown.hidden = variables.length === 0
      if (variables.length > 0) {
        unknown.replaceChildren(
          document.createTextNode(`${ctx.t('template.unknown')}: `),
          ...variables.map(name => el('code', { class: 'qqbot-preview__var' }, `{{${name}}}`))
        )
      }
      lastOk = true
      return true
    } catch {
      if (mine !== sequence || ctx.disposed) return false
      truncated.hidden = true
      sampleNote.hidden = true
      unknown.hidden = true
      body.classList.add('is-empty')
      body.textContent = ctx.t('template.previewFailed')
      count.textContent = ''
      return false
    }
  }

  const schedule = (delay = PREVIEW_DEBOUNCE_MS) => {
    if (timer !== undefined) window.clearTimeout(timer)
    timer = window.setTimeout(() => {
      timer = undefined
      void run()
    }, delay)
  }

  toggle.addEventListener('click', () => {
    mode = mode === 'draft' ? 'saved' : 'draft'
    schedule(0)
  })

  ctx.onSavedChange(() => {
    // Save or reset moved the baseline: back to "what will actually be sent",
    // and the previous answer is stale even though the payload looks the same.
    mode = 'saved'
    lastKey = ''
    lastOk = false
    schedule(0)
  })

  // Listeners are registered per key, so it does not matter that
  // max_message_chars lives in a section rendered after this one.
  ctx.onInput(PREVIEW_KEYS, () => { paint() })
  ctx.onInput(PREVIEW_KEYS, schedule)
  paint()
  schedule(0)

  return {
    root,
    stop: () => {
      if (timer !== undefined) window.clearTimeout(timer)
      timer = undefined
    }
  }
}

export function createTemplateSection(ctx: AppContext): Readonly<{ root: HTMLElement, stop: () => void }> {
  const section = createSection(ctx, {
    id: 'template',
    keys: ['message_format', 'excerpt_limit'],
    title: ctx.t('group.template'),
    hint: ctx.t('group.template.hint')
  })
  const grid = section.querySelector('.qqbot-grid')

  const templateField = renderField(ctx, 'message_template')
  if (grid && templateField) {
    const control = () => templateField.control as HTMLTextAreaElement
    // Presets replace the whole body, the chips edit what is already there:
    // presets sit above the chips, both above the textarea.
    templateField.root.insertBefore(presets(ctx, control), templateField.control)
    templateField.root.insertBefore(chips(ctx, control), templateField.control)
    grid.append(templateField.root)
  }

  // Preview sits directly under the textarea (the operator is editing it right
  // there), and the signature field closes the section.
  const preview = createPreview(ctx)
  if (grid) grid.append(preview.root)

  const signatureField = renderField(ctx, 'signature')
  if (grid && signatureField) grid.append(signatureField.root)

  return { root: section, stop: preview.stop }
}
