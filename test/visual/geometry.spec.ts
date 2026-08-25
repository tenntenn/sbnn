// Geometry and computed style, measured in a real browser against a real
// sbnn server. No golden images: every assertion here is a number that is
// stable across machines, and every failure names the number that is wrong.
//
// Some of these assert defects that are open right now. Those carry
// test.fail(), which means Playwright expects them to fail: the suite is
// green while the defect exists, and goes red the moment the defect is
// fixed, which is the signal to delete the annotation. The value measured
// when the test was written is in the comment above it.
import { test, expect, type Page } from '@playwright/test'
import { handoff } from './harness'

/** True in the two projects that run at 390x844. */
function onPhone(): boolean {
  return test.info().project.name.startsWith('phone')
}

/**
 * Opens the review page. The page holds an EventSource open on /_/events,
 * so "networkidle" never fires - the diff table is in both layouts, so it
 * is the signal that the UI has rendered whatever this viewport gets.
 */
async function open(page: Page): Promise<void> {
  // Read lazily: spec files are loaded to be listed, which happens before
  // globalSetup has started a server for them to talk to.
  await page.goto(handoff().baseURL, { waitUntil: 'domcontentloaded' })
  await page.locator('.diff-table').first().waitFor({ state: 'visible' })
}

/**
 * Brings the file list on screen. Below 720px the panes become tabs and the
 * diff is the one showing, so the file list has to be asked for; above it
 * the sidebar is always there and this does nothing.
 */
async function showFiles(page: Page): Promise<void> {
  if ((await page.locator('.file-item').count()) > 0) return
  await page.locator('.tabs button', { hasText: 'Files' }).click()
  await page.locator('.file-item').first().waitFor({ state: 'visible' })
}

type Painted = { source: string; painted: string } | null

/**
 * Reads the characters of an element in the order they are painted, left to
 * right, by measuring each one. For a path this must reconstruct the source
 * string; when it does not, the glyphs are on screen in an order the reader
 * did not write.
 */
function paintedOrder(el: Element): Painted {
  const node = [...el.childNodes].find((n) => n.nodeType === 3)
  if (!node) return null
  const text = node.textContent ?? ''
  // A truncated path has glyphs replaced by an ellipsis, so only measure
  // paths that fit: the reordering under test does not need truncation.
  if (el.scrollWidth > el.clientWidth + 1) return null
  const items: Array<{ ch: string; x: number }> = []
  for (let i = 0; i < text.length; i++) {
    const r = document.createRange()
    r.setStart(node, i)
    r.setEnd(node, i + 1)
    const rect = r.getBoundingClientRect()
    if (rect.width === 0 && rect.height === 0) continue
    items.push({ ch: text[i], x: rect.left })
  }
  items.sort((a, b) => a.x - b.x)
  return { source: text, painted: items.map((i) => i.ch).join('') }
}

test.describe('rendered geometry', () => {
  // #73, open in every project. .file-path is direction: rtl, so the bidi
  // algorithm moves the leading dot of a dotfile to the end: the source
  // ".github/workflows/ci.yml" is painted "github/workflows/ci.yml.".
  test('paths are painted in the order they are written (#73)', async ({ page }) => {
    test.fail(true, 'file paths are laid out right to left (#73)')
    await open(page)
    await showFiles(page)
    const paths = page.locator('.file-path')
    const n = await paths.count()
    expect(n).toBeGreaterThan(0)

    const mismatched: Array<{ source: string; painted: string }> = []
    for (let i = 0; i < n; i++) {
      const r = await paths.nth(i).evaluate(paintedOrder)
      if (r === null) continue
      if (r.painted !== r.source) mismatched.push(r)
    }
    expect(mismatched, 'paths whose glyphs are painted out of order').toEqual([])
  })

  // #119, open in every project. .disclosure is a 9.6px box around a 14px
  // icon, so the icon overflows it: clientWidth 10, scrollWidth 14.
  test('no element is wider than the box that holds it (#119)', async ({ page }) => {
    test.fail(true, '.disclosure is narrower than the icon inside it (#119)')
    await open(page)
    const overflowing = await page.evaluate(() => {
      const bad: Array<{ cls: string; clientWidth: number; scrollWidth: number }> = []
      document.querySelectorAll('.disclosure, .badge, .stat').forEach((el) => {
        const e = el as HTMLElement
        // Only boxes that do not scroll: one that scrolls is meant to be
        // larger inside than out.
        if (getComputedStyle(e).overflowX !== 'visible') return
        if (e.scrollWidth > e.clientWidth + 1) {
          bad.push({ cls: e.className, clientWidth: e.clientWidth, scrollWidth: e.scrollWidth })
        }
      })
      return bad
    })
    expect(overflowing, 'elements whose content is wider than they are').toEqual([])
  })

  // #74, open on the desktop layout only. The preview pane lays out wider
  // than the column it is given: clientWidth 517, scrollWidth 576. The
  // narrow layout shows one pane at a time and does not have the defect,
  // so there the assertion is expected to hold.
  test('the page does not scroll sideways (#74)', async ({ page }) => {
    test.fail(!onPhone(), 'the preview pane is wider than its column (#74)')
    await open(page)
    const wide = await page.evaluate(() => {
      const doc = document.scrollingElement as HTMLElement
      const panes: Array<{ cls: string; clientWidth: number; scrollWidth: number }> = []
      document.querySelectorAll('.split-pane, .preview-stack, .diff-stack, .sidebar').forEach((el) => {
        const e = el as HTMLElement
        if (e.scrollWidth > e.clientWidth + 1) {
          panes.push({ cls: e.className, clientWidth: e.clientWidth, scrollWidth: e.scrollWidth })
        }
      })
      return { docClient: doc.clientWidth, docScroll: doc.scrollWidth, panes }
    })
    expect(wide.docScroll, 'document scrollWidth').toBe(wide.docClient)
    expect(wide.panes, 'panes wider inside than out').toEqual([])
  })

  // #79. Hover is a pointer affordance, so this only means anything on the
  // desktop layout; the narrow layout paints no hover state at all, which
  // is correct rather than a defect.
  test('hover and selected are different colours (#79)', async ({ page }) => {
    test.skip(onPhone(), 'hover is a pointer affordance; the narrow layout has no pointer')
    await open(page)
    await showFiles(page)
    const items = page.locator('.file-item')
    expect(await items.count()).toBeGreaterThan(1)

    // Select the first file, then hover a different one, so the two states
    // are on screen at the same time and cannot be confused for each other.
    await items.nth(0).click()
    await expect(page.locator('.file-item.active')).toHaveCount(1)
    await items.nth(1).hover()

    const colours = await page.evaluate(() => {
      const active = document.querySelector('.file-item.active') as HTMLElement | null
      const hovered = document.querySelector('.file-item:hover') as HTMLElement | null
      return {
        selected: active ? getComputedStyle(active).backgroundColor : null,
        hover: hovered ? getComputedStyle(hovered).backgroundColor : null,
      }
    })
    expect(colours.selected).not.toBeNull()
    expect(colours.hover).not.toBeNull()
    expect(colours.hover, 'hover background equals selected background').not.toBe(colours.selected)
  })
})
