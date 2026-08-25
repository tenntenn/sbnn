// Starts a real sbnn server with the fixture diff loaded, so the assertions
// measure the page the binary actually serves rather than an approximation
// of it built for the test.
//
// Startup, decided by reading cmd/root.go and cmd/server.go:
//
//   - cmd/root.go gives --port, and cmd/server.go's runServer binds it and
//     serves in the foreground. So the test picks a free port itself and
//     passes it, rather than parsing the URL out of stdout: the port is then
//     known before the process starts, and two runs never collide.
//   - Readiness is GET /_/api/status, which cmd/server.go's waitForReady
//     polls for the same purpose.
//   - The fixture goes in over stdin, which is the `git diff | sbnn` path.
import { spawn, spawnSync, type ChildProcess } from 'node:child_process'
import { createServer } from 'node:net'
import { mkdirSync, writeFileSync, readFileSync, existsSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
export const projectRoot = here
export const repoRoot = resolve(here, '..', '..')
export const tmpDir = join(projectRoot, '.tmp')
export const handoffPath = join(tmpDir, 'harness.json')

export type Handoff = {
  baseURL: string
  exportPath: string
  port: number
  pid: number
}

/** Reads what globalSetup left behind for the specs. */
export function handoff(): Handoff {
  return JSON.parse(readFileSync(handoffPath, 'utf8')) as Handoff
}

function freePort(): Promise<number> {
  return new Promise((ok, fail) => {
    const s = createServer()
    s.once('error', fail)
    s.listen(0, '127.0.0.1', () => {
      const addr = s.address()
      if (addr === null || typeof addr === 'string') {
        s.close(() => fail(new Error('could not take a port')))
        return
      }
      const { port } = addr
      s.close(() => ok(port))
    })
  })
}

/**
 * Builds the sbnn binary once. The UI it serves is the committed web/dist,
 * so this needs Go and nothing else - no pnpm build, no node_modules in web/.
 *
 * The binary is built rather than run with `go run` because `go run` stays
 * alive as a parent of the real server: killing it leaves the server holding
 * the port, which makes teardown unreliable.
 */
export function buildBinary(): string {
  mkdirSync(tmpDir, { recursive: true })
  const bin = join(tmpDir, 'sbnn')
  const res = spawnSync('go', ['build', '-o', bin, '.'], {
    cwd: repoRoot,
    encoding: 'utf8',
  })
  if (res.status !== 0) {
    throw new Error(`go build failed:\n${res.stderr ?? ''}${res.stdout ?? ''}`)
  }
  if (!existsSync(bin)) throw new Error(`go build produced no binary at ${bin}`)
  return bin
}

async function waitForStatus(baseURL: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs
  let last = ''
  while (Date.now() < deadline) {
    try {
      const r = await fetch(`${baseURL}/_/api/status`)
      if (r.ok) return
      last = `status ${r.status}`
    } catch (e) {
      last = e instanceof Error ? e.message : String(e)
    }
    await new Promise((r) => setTimeout(r, 100))
  }
  throw new Error(`sbnn on ${baseURL} did not become ready: ${last}`)
}

/** Starts the server, feeds it the fixture, and writes the export page. */
export async function start(): Promise<Handoff> {
  const bin = buildBinary()
  const port = await freePort()
  const baseURL = `http://localhost:${port}`

  // sbnn keeps its session, cache and review log under XDG directories.
  // Point them inside .tmp so a test run never touches the developer's own.
  const env = {
    ...process.env,
    XDG_STATE_HOME: join(tmpDir, 'state'),
    XDG_CACHE_HOME: join(tmpDir, 'cache'),
  }
  mkdirSync(env.XDG_STATE_HOME, { recursive: true })
  mkdirSync(env.XDG_CACHE_HOME, { recursive: true })

  const child: ChildProcess = spawn(
    bin,
    ['--foreground', '--port', String(port), '--history-file', 'off'],
    { env, stdio: ['ignore', 'pipe', 'pipe'] },
  )
  child.stdout?.resume()
  child.stderr?.resume()
  child.unref()

  await waitForStatus(baseURL, 30_000)

  // The fixture arrives the way a real diff does: on stdin.
  const fixture = readFileSync(join(projectRoot, 'fixtures', 'visual.diff'))
  const add = spawnSync(bin, ['--port', String(port), '--no-open'], {
    env,
    input: fixture,
    encoding: 'utf8',
  })
  if (add.status !== 0) {
    throw new Error(`feeding the fixture failed:\n${add.stderr ?? ''}${add.stdout ?? ''}`)
  }

  // The exported page is the same UI with no server behind it, which is
  // where the icon font is loaded differently (#55).
  const exportPath = join(tmpDir, 'export.html')
  const exp = spawnSync(bin, ['export', '--port', String(port), exportPath], {
    env,
    encoding: 'utf8',
  })
  if (exp.status !== 0) {
    throw new Error(`sbnn export failed:\n${exp.stderr ?? ''}${exp.stdout ?? ''}`)
  }

  const h: Handoff = { baseURL, exportPath, port, pid: child.pid ?? -1 }
  writeFileSync(handoffPath, JSON.stringify(h, null, 2))
  return h
}

/** Asks the server to shut down, then makes sure it is gone. */
export async function stop(h: Handoff): Promise<void> {
  try {
    await fetch(`${h.baseURL}/_/api/shutdown`, { method: 'POST' })
  } catch {
    // Already gone.
  }
  if (h.pid > 0) {
    try {
      process.kill(h.pid, 'SIGTERM')
    } catch {
      // Already gone.
    }
  }
}
