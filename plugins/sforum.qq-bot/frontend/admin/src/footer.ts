import type { StatusResponse, TestPushResponse } from './api'
import { el } from './dom'
import type { AppContext } from './types'

type Busy = null | 'save' | 'reset' | 'test' | 'status'

const RESET_ARM_MS = 4000

function stamp(ctx: AppContext): string {
  return new Date().toLocaleTimeString(ctx.zh ? 'zh-CN' : 'en-US', { hour: '2-digit', minute: '2-digit' })
}

export function createFooter(ctx: AppContext): Readonly<{ root: HTMLElement, destroy: () => void }> {
  const titleId = 'qqbot-footer-title'
  const output = el('pre', { class: 'qqbot-output' })
  output.hidden = true
  const live = el('p', { class: 'qqbot-live', role: 'status', 'aria-live': 'polite' })

  const make = (variant: 'primary' | 'secondary' | 'ghost', onClick: () => void): HTMLButtonElement => {
    const node = el('button', { type: 'button', class: `qqbot-btn qqbot-btn--${variant}` })
    node.addEventListener('click', onClick)
    return node
  }

  let busy: Busy = null
  let resetArmed = false
  let resetTimer: number | undefined

  const saveButton = make('primary', () => { void save() })
  const resetButton = make('ghost', () => armOrReset())
  const testButton = make('secondary', () => { void testPush() })
  const statusButton = make('secondary', () => { void status() })

  const root = el(
    'section',
    { class: 'qqbot-card qqbot-footer', 'aria-labelledby': titleId },
    el(
      'header',
      { class: 'qqbot-card__header' },
      el('h3', { class: 'qqbot-card__title', id: titleId }, ctx.t('footer.title')),
      el('p', { class: 'qqbot-card__hint' }, ctx.t('footer.hint'))
    ),
    el(
      'div',
      { class: 'qqbot-footer__row' },
      el('div', { class: 'qqbot-footer__group' }, saveButton, resetButton),
      el('div', { class: 'qqbot-footer__group' }, testButton, statusButton)
    ),
    output,
    live
  )

  const paint = () => {
    saveButton.disabled = busy !== null
    resetButton.disabled = busy !== null
    testButton.disabled = busy !== null
    statusButton.disabled = busy !== null
    saveButton.textContent = busy === 'save' ? ctx.t('footer.saveBusy') : ctx.t('footer.save')
    resetButton.textContent = resetArmed ? ctx.t('footer.resetArmed') : ctx.t('footer.reset')
    testButton.textContent = busy === 'test' ? ctx.t('footer.testBusy') : ctx.t('footer.test')
    statusButton.textContent = busy === 'status' ? ctx.t('footer.statusBusy') : ctx.t('footer.status')
  }

  /** Defensive: the backend must never echo a secret, and neither may we. */
  const redact = (text: string): string => {
    const secret = ctx.getValue('app_secret')
    if (secret.length < 4) return text
    return text.split(secret).join('***')
  }

  const showOutput = (text: string, tone: 'ok' | 'bad') => {
    const clean = text.trim()
    output.hidden = clean === ''
    output.classList.toggle('qqbot-output--bad', tone === 'bad')
    output.textContent = clean
  }

  const disarmReset = () => {
    if (resetTimer !== undefined) window.clearTimeout(resetTimer)
    resetTimer = undefined
    resetArmed = false
    paint()
  }

  function armOrReset() {
    if (busy) return
    if (!resetArmed) {
      resetArmed = true
      paint()
      resetTimer = window.setTimeout(disarmReset, RESET_ARM_MS)
      return
    }
    void reset()
  }

  async function save() {
    if (busy) return
    busy = 'save'
    paint()
    try {
      // The host renders its own success/failure toast for a settings save, so
      // this button only mirrors the draft back into the form.
      await ctx.bridge.settings.save()
      ctx.syncFromBridge()
      live.textContent = `${ctx.t('footer.savedAt', { time: stamp(ctx) })}`
    } catch {
      showOutput(ctx.t('error.generic'), 'bad')
      live.textContent = ctx.t('error.generic')
    } finally {
      busy = null
      paint()
    }
  }

  async function reset() {
    if (busy) return
    disarmReset()
    busy = 'reset'
    paint()
    try {
      await ctx.bridge.settings.reset()
      ctx.syncFromBridge()
      live.textContent = ctx.t('footer.resetDone', { time: stamp(ctx) })
    } catch {
      showOutput(ctx.t('error.generic'), 'bad')
      live.textContent = ctx.t('error.generic')
    } finally {
      busy = null
      paint()
    }
  }

  async function testPush() {
    if (busy) return
    busy = 'test'
    paint()
    try {
      const response = await ctx.request<TestPushResponse>('/test-push', { method: 'POST', body: {} })
      const ok = response?.ok === true
      const message = redact(typeof response?.message === 'string' ? response.message : ctx.t('error.generic'))
      showOutput(message, ok ? 'ok' : 'bad')
      live.textContent = ok ? ctx.t('footer.testOk') : ctx.t('footer.testFail')
      ctx.toast(ok ? 'success' : 'error', ok ? ctx.t('footer.testOk') : ctx.t('footer.testFail'), message)
    } catch {
      showOutput(ctx.t('error.generic'), 'bad')
      live.textContent = ctx.t('footer.testFail')
      ctx.toast('error', ctx.t('footer.testFail'), ctx.t('error.generic'))
    } finally {
      busy = null
      paint()
    }
  }

  async function status() {
    if (busy) return
    busy = 'status'
    paint()
    try {
      const response = await ctx.request<StatusResponse>('/status', { method: 'POST', body: {} })
      const text = redact(typeof response?.text === 'string' ? response.text : ctx.t('error.generic'))
      showOutput(text, 'ok')
      live.textContent = ctx.t('footer.statusOk')
      ctx.toast('success', ctx.t('footer.statusOk'))
    } catch {
      showOutput(ctx.t('error.generic'), 'bad')
      live.textContent = ctx.t('footer.statusFail')
      ctx.toast('error', ctx.t('footer.statusFail'), ctx.t('error.generic'))
    } finally {
      busy = null
      paint()
    }
  }

  paint()

  return {
    root,
    destroy: () => {
      if (resetTimer !== undefined) window.clearTimeout(resetTimer)
      resetTimer = undefined
    }
  }
}
