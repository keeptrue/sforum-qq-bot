import { el } from './dom'
import type { AppContext, FieldControl, FieldHandle, SettingItem } from './types'

const TEXTAREA_ROWS: Record<string, number> = {
  group_ids: 4,
  category_filter: 4,
  message_template: 10
}

type NumberRange = { min: number, max: number, step: number }

// The manifest has no min/max on number fields; these mirror the limits the
// backend enforces so the browser stepper cannot walk into nonsense values.
const NUMBER_RANGE: Record<string, NumberRange> = {
  excerpt_limit: { min: 0, max: 5000, step: 10 },
  max_message_chars: { min: 0, max: 5000, step: 50 },
  daily_push_cap_per_group: { min: 0, max: 1000, step: 10 },
  max_pushes_per_minute: { min: 1, max: 600, step: 1 },
  max_retries: { min: 0, max: 10, step: 1 },
  send_timeout_seconds: { min: 1, max: 120, step: 1 }
}

function readControl(control: FieldControl): string {
  if (control instanceof HTMLInputElement && control.type === 'checkbox') {
    return control.checked ? 'true' : 'false'
  }
  return control.value
}

function descriptionNode(item: SettingItem, id: string): HTMLElement | null {
  if (!item.description) return null
  return el('p', { class: 'qqbot-field__desc', id }, item.description)
}

export function renderField(ctx: AppContext, key: string): FieldHandle | null {
  const item = ctx.item(key)
  if (!item) return null

  const id = `sforum-qqbot-${key}`
  const descId = `${id}-desc`
  const describedBy = item.description ? { 'aria-describedby': descId } : {}

  if (item.type === 'boolean') {
    const input = el('input', { type: 'checkbox', id, ...describedBy })
    input.checked = ctx.getValue(key) === 'true'
    const root = el(
      'div',
      { class: 'qqbot-field qqbot-field--boolean', 'data-key': key },
      el('label', { class: 'qqbot-check', for: id }, input, el('span', {}, item.label || key)),
      descriptionNode(item, descId)
    )
    ctx.registerControl(key, input, () => readControl(input))
    return { root, control: input, item }
  }

  let control: FieldControl
  let extra: HTMLElement | null = null

  if (item.type === 'select') {
    const options = item.options ?? []
    const select = el('select', { id, class: 'qqbot-input', ...describedBy })
    for (const option of options) {
      select.append(el('option', { value: option.value }, option.label || option.value))
    }
    const current = options.some(option => option.value === ctx.getValue(key))
    select.value = current ? ctx.getValue(key) : (options[0]?.value ?? '')
    control = select

    const optionHint = el('p', { class: 'qqbot-field__hint' })
    const paintOptionHint = () => {
      const selected = options.find(option => option.value === select.value)
      optionHint.textContent = selected?.description || ''
      optionHint.hidden = !selected?.description
    }
    paintOptionHint()
    ctx.onInput([key], paintOptionHint)
    extra = optionHint
  } else if (item.type === 'textarea') {
    const textarea = el('textarea', {
      id,
      rows: TEXTAREA_ROWS[key] ?? 3,
      class: key === 'message_template' ? 'qqbot-input qqbot-input--mono' : 'qqbot-input',
      placeholder: item.placeholder || '',
      spellcheck: 'false',
      ...describedBy
    })
    textarea.value = ctx.getValue(key)
    control = textarea
  } else if (item.type === 'number') {
    const range = NUMBER_RANGE[key]
    const input = el('input', {
      type: 'number',
      id,
      class: 'qqbot-input',
      inputmode: 'numeric',
      ...(range ?? {}),
      ...describedBy
    })
    input.value = ctx.getValue(key)
    control = input
  } else if (item.type === 'secret') {
    const input = el('input', {
      type: 'password',
      id,
      class: 'qqbot-input',
      autocomplete: 'new-password',
      spellcheck: 'false',
      placeholder: ctx.secretSaved ? ctx.t('field.keepSecret') : ctx.t('field.secretPlaceholder'),
      ...describedBy
    })
    // The host never returns a stored secret, so this draft always starts blank.
    input.value = ''
    control = input
  } else {
    const input = el('input', {
      type: 'text',
      id,
      class: 'qqbot-input',
      autocomplete: 'off',
      spellcheck: 'false',
      placeholder: item.placeholder || '',
      ...describedBy
    })
    input.value = ctx.getValue(key)
    control = input
  }

  const badge = item.type === 'secret' ? el('span', { class: 'qqbot-badge' }, ctx.t('field.saved')) : null
  if (badge) badge.hidden = !ctx.secretSaved

  const root = el(
    'div',
    { class: item.width === 'full' ? 'qqbot-field qqbot-field--full' : 'qqbot-field', 'data-key': key },
    el('div', { class: 'qqbot-field__head' }, el('label', { class: 'qqbot-field__label', for: id }, item.label || key), badge),
    control,
    extra,
    descriptionNode(item, descId)
  )

  if (item.type === 'secret') {
    const input = control as HTMLInputElement
    ctx.onSecretSavedChange(() => {
      input.placeholder = ctx.secretSaved ? ctx.t('field.keepSecret') : ctx.t('field.secretPlaceholder')
      if (badge) badge.hidden = !ctx.secretSaved
    })
  }

  ctx.registerControl(key, control, () => readControl(control))
  return { root, control, item }
}

export function createSection(
  ctx: AppContext,
  options: Readonly<{ id: string, keys: readonly string[], title: string, hint?: string }>
): HTMLElement {
  const titleId = `qqbot-section-${options.id}`
  const grid = el('div', { class: 'qqbot-grid' })
  for (const key of options.keys) {
    const field = renderField(ctx, key)
    if (field) grid.append(field.root)
  }
  return el(
    'section',
    { class: 'qqbot-card', 'aria-labelledby': titleId },
    el(
      'header',
      { class: 'qqbot-card__header' },
      el('h3', { class: 'qqbot-card__title', id: titleId }, options.title),
      options.hint ? el('p', { class: 'qqbot-card__hint' }, options.hint) : null
    ),
    grid
  )
}
