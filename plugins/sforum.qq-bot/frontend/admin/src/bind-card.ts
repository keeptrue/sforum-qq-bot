import type { BindPollResponse, BindStartResponse } from './api'
import { el } from './dom'
import type { AppContext } from './types'
import { renderSVG } from 'uqr'

type Phase = 'idle' | 'ready' | 'starting' | 'waiting' | 'saving' | 'bound' | 'expired' | 'error'

const POLL_INTERVAL_MS = 1500
const TICK_INTERVAL_MS = 500
const DEFAULT_TTL_SECONDS = 300
const MAX_POLL_FAILURES = 3

function qrNode(connectUrl: string): Node | null {
  try {
    const markup = renderSVG(connectUrl, {
      ecc: 'M',
      border: 2,
      pixelSize: 4,
      blackColor: '#111111',
      whiteColor: '#ffffff'
    })
    // Parsed as XML and imported as a node, never assigned as HTML: a malformed
    // or hostile string cannot become markup inside the admin page.
    const parsed = new DOMParser().parseFromString(markup, 'image/svg+xml')
    if (!parsed.documentElement) return null
    return document.importNode(parsed.documentElement, true)
  } catch {
    return null
  }
}

function clock(seconds: number): string {
  const safe = Math.max(0, Math.floor(seconds))
  const minutes = Math.floor(safe / 60)
  return `${String(minutes).padStart(2, '0')}:${String(safe % 60).padStart(2, '0')}`
}

export function createBindCard(ctx: AppContext): Readonly<{ root: HTMLElement, destroy: () => void }> {
  const titleId = 'qqbot-bind-title'
  const body = el('div', { class: 'qqbot-bind__body' })
  const live = el('p', { class: 'qqbot-live', role: 'status', 'aria-live': 'polite' })

  const root = el(
    'section',
    { class: 'qqbot-card qqbot-bind', 'aria-labelledby': titleId },
    el(
      'header',
      { class: 'qqbot-card__header' },
      el('h3', { class: 'qqbot-card__title', id: titleId }, ctx.t('bind.title')),
      el('p', { class: 'qqbot-card__hint' }, ctx.t('bind.hint'))
    ),
    body,
    live
  )

  let phase: Phase = ctx.getValue('app_id') !== '' || ctx.secretSaved ? 'ready' : 'idle'
  let appId = ctx.getValue('app_id')
  let taskId = ''
  let connectUrl = ''
  let expiresAt = 0
  let generation = 0
  let pollFailures = 0
  let pollTimer: number | undefined
  let tickTimer: number | undefined
  let countdown: HTMLElement | null = null
  let qr: Node | null = null

  const clearTimers = () => {
    if (pollTimer !== undefined) window.clearInterval(pollTimer)
    if (tickTimer !== undefined) window.clearInterval(tickTimer)
    pollTimer = undefined
    tickTimer = undefined
    countdown = null
  }

  const paintCountdown = () => {
    if (!countdown) return
    countdown.textContent = ctx.t('bind.expires', { time: clock((expiresAt - Date.now()) / 1000) })
  }

  const button = (
    label: string,
    variant: 'primary' | 'secondary' | 'ghost',
    onClick: () => void
  ): HTMLButtonElement => {
    const node = el('button', { type: 'button', class: `qqbot-btn qqbot-btn--${variant}` }, label)
    node.addEventListener('click', onClick)
    return node
  }

  const manualButton = (): HTMLButtonElement => button(ctx.t('bind.manual'), 'ghost', () => ctx.focus('app_id'))

  const paint = () => {
    clearTimers()
    root.removeAttribute('aria-busy')
    const nodes: Node[] = []
    let focusAfter: HTMLElement | null = null

    if (phase === 'idle') {
      nodes.push(el('div', { class: 'qqbot-bind__actions' },
        button(ctx.t('bind.start'), 'primary', () => { void start() }),
        manualButton()))
    } else if (phase === 'ready') {
      if (appId) nodes.push(el('p', { class: 'qqbot-bind__current' }, ctx.t('bind.boundId', { id: appId })))
      nodes.push(el('div', { class: 'qqbot-bind__actions' },
        button(ctx.t('bind.rebind'), 'secondary', () => { void start() }),
        manualButton()))
    } else if (phase === 'starting') {
      root.setAttribute('aria-busy', 'true')
      nodes.push(el('p', { class: 'qqbot-muted' }, ctx.t('bind.starting')))
    } else if (phase === 'waiting') {
      countdown = el('p', { class: 'qqbot-bind__countdown' })
      paintCountdown()
      const frame = el('div', { class: 'qqbot-qr', role: 'img', 'aria-label': ctx.t('bind.title') })
      if (qr) frame.append(qr)
      nodes.push(el('div', { class: 'qqbot-bind__layout' },
        frame,
        el('div', { class: 'qqbot-bind__side' },
          el('p', { class: 'qqbot-bind__waiting' }, ctx.t('bind.waiting')),
          countdown,
          el('a', {
            class: 'qqbot-bind__link',
            href: connectUrl,
            target: '_blank',
            rel: 'noopener noreferrer'
          }, ctx.t('bind.openLink')),
          el('div', { class: 'qqbot-bind__actions' },
            button(ctx.t('bind.retry'), 'secondary', () => { void start() }),
            manualButton())
        )))
    } else if (phase === 'saving') {
      root.setAttribute('aria-busy', 'true')
      nodes.push(el('p', { class: 'qqbot-muted' }, ctx.t('bind.saving')))
    } else if (phase === 'bound') {
      nodes.push(
        el('p', { class: 'qqbot-bind__done' }, ctx.t('bind.bound')),
        el('p', { class: 'qqbot-muted' }, ctx.t('bind.boundId', { id: appId })),
        el('div', { class: 'qqbot-bind__actions' },
          button(ctx.t('bind.rebind'), 'secondary', () => { void start() }),
          manualButton())
      )
      live.textContent = ctx.t('bind.bound')
    } else if (phase === 'expired') {
      const retry = button(ctx.t('bind.retry'), 'primary', () => { void start() })
      focusAfter = retry
      nodes.push(
        el('p', { class: 'qqbot-bind__error' }, ctx.t('bind.expired')),
        el('div', { class: 'qqbot-bind__actions' }, retry, manualButton())
      )
      live.textContent = ctx.t('bind.expired')
    } else {
      const retry = button(ctx.t('bind.retry'), 'secondary', () => { void start() })
      focusAfter = retry
      nodes.push(
        el('p', { class: 'qqbot-bind__error' }, ctx.t('bind.error')),
        el('div', { class: 'qqbot-bind__actions' }, retry, manualButton())
      )
      live.textContent = ctx.t('bind.error')
    }

    body.replaceChildren(...nodes)
    if (focusAfter) focusAfter.focus()
  }

  const ttlSeconds = (value: unknown): number => {
    const seconds = typeof value === 'number' ? value : Number(value)
    return Number.isFinite(seconds) && seconds > 0 ? seconds : DEFAULT_TTL_SECONDS
  }

  const expire = () => {
    clearTimers()
    phase = 'expired'
    paint()
  }

  const finish = async (mine: number, response: BindPollResponse) => {
    clearTimers()
    phase = 'saving'
    paint()
    const nextAppId = typeof response.appId === 'string' ? response.appId.trim() : ''
    const nextSecret = typeof response.appSecret === 'string' ? response.appSecret : ''
    if (!nextAppId || !nextSecret) {
      if (mine === generation && !ctx.disposed) {
        phase = 'error'
        paint()
      }
      return
    }
    ctx.setValue('app_id', nextAppId)
    // The secret goes into the host draft only; it is never written into an
    // input value, a toast, or any status text.
    ctx.setValue('app_secret', nextSecret, false)
    await ctx.bridge.settings.save()
    if (mine !== generation || ctx.disposed) return
    // save() resolves even when the host request failed (the host owns that
    // toast), so the persisted values are the honest signal: on success the
    // host re-reads settings and masks the secret back to "".
    const values = ctx.bridge.settings.values()
    const persisted = values['app_id'] === nextAppId && (values['app_secret'] ?? '') === ''
    if (!persisted) {
      phase = 'error'
      paint()
      live.textContent = ctx.t('error.saveFailed')
      return
    }
    appId = nextAppId
    ctx.markSecretSaved()
    ctx.syncFromBridge()
    phase = 'bound'
    paint()
    ctx.toast('success', ctx.t('bind.bound'), ctx.t('bind.boundId', { id: nextAppId }))
  }

  const poll = async (mine: number) => {
    if (mine !== generation || ctx.disposed || phase !== 'waiting') return
    if (Date.now() >= expiresAt) {
      expire()
      return
    }
    let response: BindPollResponse
    try {
      response = await ctx.request<BindPollResponse>('/bind/poll', { method: 'POST', body: { taskId } })
    } catch {
      if (mine !== generation || ctx.disposed) return
      pollFailures += 1
      if (pollFailures >= MAX_POLL_FAILURES) {
        phase = 'error'
        paint()
      }
      return
    }
    if (mine !== generation || ctx.disposed) return
    pollFailures = 0
    // Completion is "both credentials present", not a status value: the route
    // schema says appId and appSecret are the end of the flow, independent of
    // the status label.
    const appIdReady = typeof response?.appId === 'string' && response.appId.trim() !== ''
    const secretReady = typeof response?.appSecret === 'string' && response.appSecret !== ''
    if (appIdReady && secretReady) {
      await finish(mine, response)
      return
    }
    if (response?.status === 'expired') expire()
  }

  const start = async () => {
    clearTimers()
    const mine = ++generation
    pollFailures = 0
    phase = 'starting'
    paint()
    try {
      const response = await ctx.request<BindStartResponse>('/bind/start', { method: 'POST', body: {} })
      if (mine !== generation || ctx.disposed) return
      const nextTaskId = typeof response?.taskId === 'string' ? response.taskId : ''
      const nextUrl = typeof response?.connectUrl === 'string' ? response.connectUrl : ''
      if (!nextTaskId || !nextUrl) {
        phase = 'error'
        paint()
        return
      }
      taskId = nextTaskId
      connectUrl = nextUrl
      expiresAt = Date.now() + ttlSeconds(response.expiresInSeconds) * 1000
      qr = qrNode(connectUrl)
      phase = 'waiting'
      paint()
      // The ticker owns expiry so the card flips to "expired" within half a
      // second of the countdown hitting zero, not at the next poll boundary.
      tickTimer = window.setInterval(() => {
        if (phase !== 'waiting') return
        if (Date.now() >= expiresAt) {
          expire()
          return
        }
        paintCountdown()
      }, TICK_INTERVAL_MS)
      pollTimer = window.setInterval(() => { void poll(mine) }, POLL_INTERVAL_MS)
    } catch {
      if (mine !== generation || ctx.disposed) return
      phase = 'error'
      paint()
    }
  }

  paint()

  return {
    root,
    destroy: () => {
      generation += 1
      clearTimers()
    }
  }
}
