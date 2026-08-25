import test from 'node:test'
import assert from 'node:assert/strict'

/**
 * The comments a reviewer writes on an exported page live in localStorage and
 * nowhere else, so a browser that refuses to save them is losing the work
 * itself. These tests stand in for the browser: they drive the real static
 * client against a localStorage that fails the way a private window, blocked
 * site data or a full quota does, and read what the page then says.
 *
 * The DOM here is the smallest thing the banner touches. jsdom is not
 * available offline and the banner is built by hand precisely because it is
 * created before React mounts, so a handful of stubs is enough to read back
 * what a reader would see.
 */
interface FakeNode {
  tagName: string
  textContent: string
  style: { cssText: string }
  children: FakeNode[]
  attrs: Record<string, string>
  removed: boolean
  onClick?: () => void
  text(): string
  css(): string
}

function element(tagName: string): FakeNode {
  const node: FakeNode = {
    tagName,
    textContent: '',
    style: { cssText: '' },
    children: [],
    attrs: {},
    removed: false,
    text() {
      return this.textContent + this.children.map((c) => c.text()).join('')
    },
    css() {
      return this.style.cssText + this.children.map((c) => c.css()).join(' ')
    },
  }
  return node
}

interface FakeDom {
  body: { children: FakeNode[] }
  bars(): FakeNode[]
}

function installDom(): FakeDom {
  const body = {
    children: [] as FakeNode[],
    appendChild(node: FakeNode) {
      body.children.push(node)
      return node
    },
  }
  const doc = {
    body,
    createElement(tagName: string) {
      const node = element(tagName)
      return new Proxy(node, {
        get(target, prop) {
          if (prop === 'setAttribute') return (k: string, v: string) => void (target.attrs[k] = v)
          if (prop === 'append') return (...kids: FakeNode[]) => target.children.push(...kids)
          if (prop === 'remove')
            return () => {
              // The node reaches the page wrapped in this proxy, so the copy
              // in body.children is never the same object as target: the flag
              // is what identifies it.
              target.removed = true
              body.children = body.children.filter((c) => !c.removed)
            }
          if (prop === 'addEventListener')
            return (_type: string, fn: () => void) => void (target.onClick = fn)
          return target[prop as keyof FakeNode]
        },
        set(target, prop, value) {
          target[prop as keyof FakeNode] = value
          return true
        },
      }) as unknown as FakeNode
    },
  }
  ;(globalThis as unknown as { document: unknown }).document = doc
  return { body, bars: () => body.children }
}

/** storage is a localStorage that can be told how to fail. */
const storage = {
  entries: new Map<string, string>(),
  readThrows: false,
  writeThrows: false,
  getItem(key: string): string | null {
    if (storage.readThrows) throw new Error('site data is blocked')
    return storage.entries.get(key) ?? null
  },
  setItem(key: string, value: string) {
    if (storage.writeThrows) throw new Error('QuotaExceededError')
    storage.entries.set(key, value)
  },
}

const dom = installDom()
;(globalThis as unknown as { window: unknown }).window = {
  localStorage: storage,
  __SBNN_DATA__: {
    version: 2,
    generatedAt: '2026-03-04T05:06:07Z',
    group: 'api',
    diffs: [{ id: 'd1', title: 'a change' }],
    comments: [],
    previews: {},
    images: {},
  },
}

const errors: string[] = []
console.error = (...args: unknown[]) => void errors.push(args.map(String).join(' '))

const { client } = await import('../src/client')

const banner = () => dom.bars().at(-1)
const said = () => banner()?.text() ?? ''

/** dismiss presses the banner's own Dismiss button, as a reader would. */
function dismiss(node: FakeNode | undefined): void {
  if (!node) throw new Error('there is no banner to dismiss')
  const button = node.children.find((c) => c.tagName === 'button')
  if (!button?.onClick) throw new Error('the banner has no Dismiss button')
  button.onClick()
}

test('an entry that cannot be read back is reported, without claiming writes are lost', async () => {
  storage.entries.set('sbnn:comments:api:2026-03-04T05:06:07Z', '{ truncated')
  await client.load('api')

  assert.match(said(), /could not be read back/)
  // Storage is answering, so the next write is saved: the reviewer must not be
  // told that nothing they write will survive the tab.
  assert.doesNotMatch(said(), /will not survive|survive this tab/)
  storage.entries.clear()
})

test('a refused write says so even after an earlier trouble was dismissed', async () => {
  // The reader dismisses what the page said about the unreadable entry.
  dismiss(banner())
  assert.equal(dom.bars().length, 0, 'Dismiss left the banner on the page')

  storage.writeThrows = true
  await client.addComment('api', { diffId: 'd1', fileId: 'f1', path: 'a.go', side: 'new', startLine: 1, endLine: 1, snippet: 'x := 1', body: 'this must not vanish' })

  assert.match(said(), /refused to save your comments/)
  assert.match(said(), /Copy prompt/)
})

test('the comment stays on the page, and in the prompt, when it could not be saved', async () => {
  const data = await client.load('api')
  assert.equal(data.comments.length, 1)
  assert.match(await client.prompt('api'), /this must not vanish/)
})

test('the banner paints itself in properties the stylesheet defines', () => {
  const css = banner()?.css() ?? ''
  assert.match(css, /var\(--danger-fg,/)
  assert.doesNotMatch(css, /var\(--danger,/)
})

test('every trouble reaches the console as well as the page', () => {
  assert.ok(
    errors.some((e) => e.includes('could not be read back')),
    errors.join('\n'),
  )
  assert.ok(
    errors.some((e) => e.includes('refused to save your comments')),
    errors.join('\n'),
  )
})
