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
