import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

/** storage is a localStorage that can be told to refuse, the way a
 * sandboxed frame or blocked site data does. */
const storage = {
  entries: new Map<string, string>(),
  refuse: false,
  getItem(key: string): string | null {
    if (storage.refuse) throw new Error('site data is blocked')
    return storage.entries.get(key) ?? null
  },
  setItem(key: string, value: string) {
    if (storage.refuse) throw new Error('site data is blocked')
    storage.entries.set(key, value)
  },
}
;(globalThis as unknown as { window: unknown }).window = { localStorage: storage }

const { markTourSeen, placeCard, shouldAutoStart, TOUR_STEPS } = await import('../src/tour')
const { catalogs } = await import('../src/i18n')

describe('when the tour opens by itself', () => {
  it('opens on the first visit, and again until it is finished', () => {
    storage.entries.clear()
    assert.equal(shouldAutoStart(), true)
    // A reload half way through has not seen the whole of it.
    assert.equal(shouldAutoStart(), true)
    markTourSeen()
    assert.equal(shouldAutoStart(), false)
  })

  // Where nothing can be remembered, "once per browser" would be "on every
  // load", so it waits to be asked for from the help sheet instead.
  it('stays shut where storage is refused, without throwing', () => {
    storage.entries.clear()
    storage.refuse = true
    try {
      assert.equal(shouldAutoStart(), false)
      assert.doesNotThrow(() => markTourSeen())
    } finally {
      storage.refuse = false
    }
  })
})

describe('the tour steps', () => {
  it('say something in the catalog for every step', () => {
    for (const step of TOUR_STEPS) {
      for (const key of [step.title, step.body, step.staticBody]) {
        if (key === undefined) continue
        assert.ok(key in catalogs.en, `${step.id}: ${key} is not in the catalog`)
      }
      assert.ok(step.targets.length > 0, `${step.id} points at nothing`)
    }
  })
})

describe('placeCard', () => {
  const view = { width: 1280, height: 800 }
  const card = { width: 352, height: 180 }
  const inView = (p: { top: number; left: number }) =>
    p.top >= 0 && p.left >= 0 && p.top + card.height <= view.height && p.left + card.width <= view.width

  it('puts the card below a small target', () => {
    const p = placeCard({ top: 10, left: 1100, width: 120, height: 30 }, card, view)
    assert.ok(p.top >= 40, `the card covers the target: ${JSON.stringify(p)}`)
    assert.ok(inView(p), `off screen: ${JSON.stringify(p)}`)
  })

  it('puts the card beside a pane as tall as the window', () => {
    const p = placeCard({ top: 44, left: 0, width: 280, height: 756 }, card, view)
    assert.ok(p.left >= 280, `the card covers the file list: ${JSON.stringify(p)}`)
    assert.ok(inView(p))
  })

  it('stays on screen when nothing has room around it', () => {
    const phone = { width: 390, height: 800 }
    const p = placeCard({ top: 44, left: 0, width: 390, height: 756 }, { width: 366, height: 200 }, phone)
    assert.ok(p.left >= 0 && p.left + 366 <= phone.width)
    assert.ok(p.top >= 0 && p.top + 200 <= phone.height)
  })

  it('centres the card when there is no target', () => {
    const p = placeCard(null, card, view)
    assert.equal(Math.round(p.left), Math.round((view.width - card.width) / 2))
  })
})
