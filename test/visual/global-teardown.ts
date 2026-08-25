import { existsSync } from 'node:fs'
import { handoff, handoffPath, stop } from './harness'

export default async function globalTeardown(): Promise<void> {
  if (!existsSync(handoffPath)) return
  await stop(handoff())
}
