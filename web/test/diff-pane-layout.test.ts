import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

/**
 * The phone's diff pane is a column, and these tests are what keeps it one.
 *
 * `.content` is a flex container with no direction of its own, so its
 * children are laid out in a row. The wide layout hands it a single child and
 * never notices; the phone hands it the file plus the stepper that walks to
 * the next one, and a row put those two side by side - the stepper took 258px
 * of a 360px screen and the diff was squeezed into the 102px left, which made
 * the diff table eight thousand pixels tall.
 *
 * Neither JSX nor CSS can be rendered here: the resolve hook loads .ts and
 * not .tsx, and jsdom is not available offline. So these read the two sources
 * that have to agree - the pane is one element, and that element is a column.
 */
function source(name: string): string {
  return readFileSync(fileURLToPath(new URL(`../src/${name}`, import.meta.url)), 'utf8')
}

/** rule returns the declarations of the first top-level `selector { ... }`. */
function rule(css: string, selector: string): string {
  const at = css.indexOf(`\n${selector} {`)
  assert.notEqual(at, -1, `styles.css has no ${selector} rule`)
  const open = css.indexOf('{', at)
  const close = css.indexOf('}', open)
  return css.slice(open + 1, close)
}

/** narrowDiffPane returns the JSX App renders into `.content` on a phone. */
function narrowDiffPane(app: string): string {
  const at = app.indexOf('const diffPane = narrow ? (')
  assert.notEqual(at, -1, 'App.tsx no longer builds diffPane the way this test reads it')
  const end = app.indexOf('<p className="empty">Select a file.</p>', at)
  assert.notEqual(end, -1, 'App.tsx no longer has the empty branch this test reads up to')
  return app.slice(at, end)
}

test('.content still lays its children out in a row', () => {
  // The premise of everything below. If .content ever gains a column
  // direction of its own the wrapper stops being load-bearing, and this
  // test should be revisited rather than deleted.
  assert.doesNotMatch(rule(source('styles.css'), '.content'), /flex-direction/)
})

test('the phone diff pane hands .content a single element, not a fragment', () => {
  const jsx = narrowDiffPane(source('App.tsx'))
  assert.match(jsx, /<div className="diff-pane">/, 'the file and the stepper need one column to share')
  assert.doesNotMatch(jsx, /<>/, 'a fragment puts the stepper beside the diff, not below it')
})

test('the stepper is inside that column, so it sits below the diff', () => {
  const jsx = narrowDiffPane(source('App.tsx'))
  const wrapper = jsx.indexOf('<div className="diff-pane">')
  const stepper = jsx.indexOf('<FileStepper')
  const close = jsx.indexOf('</div>', wrapper)
  assert.notEqual(stepper, -1, 'the phone pane has no way on to the next file')
  assert.ok(wrapper < stepper && stepper < close, 'FileStepper belongs inside .diff-pane')
})

test('.diff-pane is a column that fills the width it is given', () => {
  const decls = rule(source('styles.css'), '.diff-pane')
  assert.match(decls, /flex-direction:\s*column/)
  assert.match(decls, /display:\s*flex/)
  // Without this the diff table's own width leaks out as a horizontal
  // scrollbar across the whole page.
  assert.match(decls, /min-width:\s*0/)
})

test('the stepper is laid out by the stylesheet, not beside the diff', () => {
  assert.match(source('components/DiffStack.tsx'), /<nav className="file-stepper"/)
  const decls = rule(source('styles.css'), '.file-stepper')
  // A foot takes the height it asks for and leaves the rest to the diff.
  assert.match(decls, /flex:\s*0 0 auto/)
  // And it sits at the bottom of the space a short file leaves, rather than
  // floating in the middle of the screen.
  assert.match(decls, /margin-top:\s*auto/)
})
