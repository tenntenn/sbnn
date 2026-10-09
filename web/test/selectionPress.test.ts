import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { isSelectingPress } from '../src/selectionPress'

describe('isSelectingPress', () => {
  // The bug (#378): a right click on selected preview text was handled like
  // the start of a new selection, which cleared it before the browser's
  // context menu (translate, read aloud) could act on it.
  const cases = [
    { name: 'primary mouse button selects', ev: { type: 'mousedown', button: 0 }, want: true },
    { name: 'primary mouse button release selects', ev: { type: 'mouseup', button: 0 }, want: true },
    { name: 'middle button does not select', ev: { type: 'mousedown', button: 1 }, want: false },
    { name: 'secondary button press does not select', ev: { type: 'mousedown', button: 2 }, want: false },
    { name: 'secondary button release does not select', ev: { type: 'mouseup', button: 2 }, want: false },
    { name: 'touchstart selects', ev: { type: 'touchstart' }, want: true },
    { name: 'touchend selects', ev: { type: 'touchend' }, want: true },
    { name: 'ctrl+click selects off macOS', ev: { type: 'mousedown', button: 0, ctrlKey: true }, want: true },
    { name: 'ctrl+click is the context menu on macOS', ev: { type: 'mousedown', button: 0, ctrlKey: true }, mac: true, want: false },
    { name: 'plain click selects on macOS', ev: { type: 'mousedown', button: 0 }, mac: true, want: true },
  ]
  for (const c of cases) {
    it(c.name, () => {
      assert.equal(isSelectingPress(c.ev, c.mac), c.want)
    })
  }
})
