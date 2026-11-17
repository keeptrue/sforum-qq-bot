import type { AdminMicroFrontendBridgeV1, AdminMicroFrontendCleanup } from '@sforum/admin-sdk'
import { createApp } from './app'
import './settings.css'

// Contract for a prebuilt settings component: the host replaces the schema form
// with this module and calls mount(target, bridge) once per trusted load.
export const apiVersion = 1

export function mount(target: HTMLElement, bridge: AdminMicroFrontendBridgeV1): AdminMicroFrontendCleanup {
  const app = createApp(bridge)
  target.append(app.root)
  return () => app.destroy()
}
