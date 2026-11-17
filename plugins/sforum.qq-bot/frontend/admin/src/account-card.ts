import type { AccountStatusResponse } from './api'
import { el } from './dom'
import type { MessageKey } from './i18n'
import type { AppContext } from './types'

const POLL_MS = 30_000

type GatewayState = 'connected' | 'connecting' | 'disabled' | 'unauthorized' | 'error'

const STATE_LABEL: Record<GatewayState, MessageKey> = {
  connected: 'account.state.connected',
  connecting: 'account.state.connecting',
  disabled: 'account.state.disabled',
  unauthorized: 'account.state.unauthorized',
  error: 'account.state.error'
}

// Same five states, two tones: only "connected" is good news, "connecting" is
// pending, and the remaining three are all "look here".
const STATE_TONE: Record<GatewayState, string> = {
  connected: 'qqbot-account__badge--ok',
  connecting: 'qqbot-account__badge--warn',
  disabled: 'qqbot-account__badge--idle',
  unauthorized: 'qqbot-account__badge--bad',
  error: 'qqbot-account__badge--bad'
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function asCount(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? Math.floor(value) : 0
}

function asRecord(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === 'object' ? value as Record<string, unknown> : {}
}

function asState(value: unknown): GatewayState {
  return typeof value === 'string' && value in STATE_LABEL ? value as GatewayState : 'error'
}

/** First code point, so an emoji or a Chinese character is not cut in half. */
function initial(name: string): string {
  return Array.from(name)[0] ?? '?'
}

export function createAccountCard(ctx: AppContext): Readonly<{ root: HTMLElement, destroy: () => void }> {
  const titleId = 'qqbot-account-title'

  const avatar = el('span', { class: 'qqbot-account__avatar' })
  const name = el('p', { class: 'qqbot-account__name' })
  const appId = el('code', { class: 'qqbot-account__appid-value' })
  const identity = el(
    'div',
    { class: 'qqbot-account__identity' },
    avatar,
    el(
      'div',
      { class: 'qqbot-account__ident' },
      name,
      el('p', { class: 'qqbot-account__appid' }, el('span', { class: 'qqbot-account__appid-label' }, ctx.t('account.appId')), appId)
    )
  )

  const badge = el('span', { class: 'qqbot-account__badge', role: 'status', 'aria-live': 'polite' })
  const reason = el('p', { class: 'qqbot-account__reason' })
  reason.hidden = true
  const status = el('div', { class: 'qqbot-account__status' }, badge, reason)

  const meta = el('button', { type: 'button', class: 'qqbot-account__meta' })
  const note = el('p', { class: 'qqbot-account__note' })
  const body = el('div', { class: 'qqbot-account__body' }, identity, status)

  const refresh = el('button', { type: 'button', class: 'qqbot-btn qqbot-btn--ghost qqbot-account__refresh' }, ctx.t('account.refresh'))

  const root = el(
    'section',
    { class: 'qqbot-card qqbot-account', 'aria-labelledby': titleId },
    el(
      'header',
      { class: 'qqbot-account__header' },
      el('h3', { class: 'qqbot-card__title', id: titleId }, ctx.t('account.title')),
      refresh
    ),
    body,
    meta,
    note
  )

  body.hidden = true
  meta.hidden = true
  note.textContent = ctx.t('account.loading')

  let loading = false
  let sequence = 0
  let timer: number | undefined
  let whitelisted = 0

  const stopPolling = () => {
    if (timer === undefined) return
    window.clearInterval(timer)
    timer = undefined
  }

  const startPolling = () => {
    stopPolling()
    timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') void load()
    }, POLL_MS)
  }

  const paintAvatar = (url: string, label: string) => {
    const fallback = () => avatar.replaceChildren(
      el('span', { class: 'qqbot-account__initial', 'aria-hidden': 'true' }, initial(label))
    )
    avatar.replaceChildren()
    if (url === '') {
      fallback()
      return
    }
    const image = el('img', {
      src: url,
      alt: label,
      class: 'qqbot-account__avatar-img',
      loading: 'lazy',
      decoding: 'async'
    })
    // A broken avatar must not leave an empty ring behind.
    image.addEventListener('error', fallback, { once: true })
    avatar.append(image)
  }

  const paintBadge = (state: GatewayState, credentialsConfigured: boolean) => {
    badge.className = 'qqbot-account__badge'
    if (!credentialsConfigured) {
      badge.textContent = ctx.t('account.credentialsMissing')
      badge.classList.add('qqbot-account__badge--warn')
      return
    }
    badge.textContent = ctx.t(STATE_LABEL[state])
    badge.classList.add(STATE_TONE[state])
  }

  const render = (response: AccountStatusResponse) => {
    const bot = asRecord(response?.bot)
    const gateway = asRecord(response?.gateway)
    const groups = asRecord(response?.groups)
    const credentialsConfigured = response?.credentialsConfigured === true
    const botName = asString(bot.name)
    const botAvailable = bot.available === true
    const reasonText = asString(gateway.reason)

    // Bot identity is optional; the connection state never is, so only these
    // two nodes hide when the bot cannot be looked up.
    avatar.hidden = !botAvailable
    name.hidden = !botAvailable
    if (botAvailable) {
      name.textContent = botName === '' ? ctx.t('account.noBot') : botName
      paintAvatar(asString(bot.avatarUrl), botName)
    }

    appId.textContent = asString(response?.appId)
    appId.parentElement?.classList.toggle('is-empty', appId.textContent === '')

    paintBadge(asState(gateway.state), credentialsConfigured)

    reason.textContent = reasonText
    reason.hidden = reasonText === ''

    const recognized = asCount(groups.recognized)
    whitelisted = asCount(groups.whitelisted)
    meta.textContent = ctx.t('account.groups', { recognized, whitelisted })
    // Only a non-empty whitelist has somewhere to jump to.
    meta.disabled = whitelisted === 0
    meta.classList.toggle('qqbot-account__meta--link', whitelisted > 0)
    if (whitelisted > 0) meta.title = ctx.t('account.groupsJump')
    else meta.removeAttribute('title')

    body.hidden = false
    meta.hidden = false
    note.hidden = true
  }

  const load = async (): Promise<void> => {
    if (loading || ctx.disposed) return
    loading = true
    const mine = ++sequence
    refresh.disabled = true
    refresh.textContent = ctx.t('account.refreshing')
    root.setAttribute('aria-busy', 'true')
    try {
      // bridge.request directly, not ctx.request: an unavailable status route
      // is a card-level fact and must not raise the page-level banner.
      const response = await ctx.bridge.request<AccountStatusResponse>('/account/status', {
        method: 'POST',
        body: {},
        responseMode: 'body'
      })
      if (mine !== sequence || ctx.disposed) return
      render(response)
    } catch {
      if (mine !== sequence || ctx.disposed) return
      // Whatever was rendered stays: the note says the numbers may be stale.
      note.textContent = ctx.t('account.unavailable')
      note.hidden = false
    } finally {
      if (mine === sequence && !ctx.disposed) {
        loading = false
        refresh.disabled = false
        refresh.textContent = ctx.t('account.refresh')
        root.removeAttribute('aria-busy')
      }
    }
  }

  refresh.addEventListener('click', () => { void load() })
  meta.addEventListener('click', () => {
    if (whitelisted === 0) return
    ctx.focus('group_ids')
  })

  // A save can change credentials or the whitelist, so re-read after it.
  ctx.onSavedChange(() => { void load() })

  // Polling pauses while the tab is hidden (nothing is watching the answer) and
  // resumes with an immediate read, so the first look is never 30s stale.
  const onVisibilityChange = () => {
    if (document.visibilityState === 'visible') {
      startPolling()
      void load()
      return
    }
    stopPolling()
  }
  document.addEventListener('visibilitychange', onVisibilityChange)
  if (document.visibilityState === 'visible') startPolling()
  void load()

  return {
    root,
    destroy: () => {
      sequence += 1
      stopPolling()
      document.removeEventListener('visibilitychange', onVisibilityChange)
      root.remove()
    }
  }
}
