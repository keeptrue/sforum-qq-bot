// Route bodies are the plugin's own JSON (Protocol V2 routes), not host envelopes.

export type BindStartResponse = {
  taskId?: unknown
  connectUrl?: unknown
  expiresInSeconds?: unknown
}

export type BindPollResponse = {
  status?: unknown
  appId?: unknown
  appSecret?: unknown
}

export type PreviewResponse = {
  text?: unknown
  truncated?: unknown
  unknownVariables?: unknown
}

export type TestPushResponse = {
  ok?: unknown
  message?: unknown
}

export type StatusResponse = {
  text?: unknown
}

/**
 * One entry of POST /groups/list. Every field is `unknown` on purpose: the
 * component normalizes what it gets instead of trusting the plugin's JSON.
 */
export type GroupSummary = {
  openid?: unknown
  name?: unknown
  nameUnavailable?: unknown
  eventType?: unknown
  firstSeenAt?: unknown
  lastSeenAt?: unknown
  count?: unknown
}

export type GroupsListResponse = {
  groups?: unknown
  claimEnabled?: unknown
  recordingLimit?: unknown
}
