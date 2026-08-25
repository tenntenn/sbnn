import { start } from './harness'

export default async function globalSetup(): Promise<void> {
  const h = await start()
  process.env.SBNN_BASE_URL = h.baseURL
}
