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
import { pathToFileURL } from 'node:url'
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

/**
 * Waits until nothing on the page is still animating.
 *
 * getComputedStyle during a transition returns the value part way along it,
 * not the value the rule asks for: two elements transitioning to the same
 * colour from different starting points read as two different colours for as
 * long as the transition lasts. A test that compares colours without waiting
 * therefore passes whatever the stylesheet says -- which is how the #79
 * assertion below stayed green while the shipped bundle painted hover and
 * selected in exactly the same colour.
 *
 * Waiting is used rather than emulating prefers-reduced-motion: the guard
 * that honours that query is in the stylesheet under test, so a bundle
 * without it would go unwaited and the test would be back where it started.
 */
async function settled(page: Page, selector: string): Promise<void> {
  await page.waitForFunction(async (sel) => {
    const els = [...document.querySelectorAll(sel)]
    await Promise.all(
      // A transition replaced by another one rejects rather than resolving;
      // either way it is over, which is all this asks.
      els.flatMap((e) => e.getAnimations().map((a) => a.finished.catch(() => undefined))),
    )
    return els.every((e) => e.getAnimations().length === 0)
  }, selector)
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
    test.fail(true, 'hover and selected are the same colour in the shipped bundle (#79)')
    await open(page)
    await showFiles(page)
    const items = page.locator('.file-item')
    expect(await items.count()).toBeGreaterThan(1)

    // Select the first file, then hover a different one, so the two states
    // are on screen at the same time and cannot be confused for each other.
    await items.nth(0).click()
    await expect(page.locator('.file-item.active')).toHaveCount(1)
    await items.nth(1).hover()
    // Both rows are transitioning towards their new background; read the
    // colours only once they have arrived.
    await settled(page, '.file-item')

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

  // A guard rather than a pinned defect: this holds in the shipped bundle
  // today, and the point of the harness is that it stops holding loudly.
  //
  // It is the half of #79 that does work. Which file is on screen is said
  // with a background, so a row that is the current file must not be painted
  // like a row that is not - and it is read through the same settling wait,
  // because the selected row is transitioning towards its background from
  // the moment it is clicked. Take the wait away and the two colours differ
  // by however far along the transition is, which is what let the assertion
  // above pass while the bundle painted both states the same.
  test('the selected file is painted differently from an unselected one', async ({ page }) => {
    await open(page)
    await showFiles(page)
    const items = page.locator('.file-item')
    expect(await items.count()).toBeGreaterThan(1)

    await items.nth(0).click()
    // On the narrow layout, choosing a file switches to the diff tab and
    // takes the list off screen; ask for it back before measuring it.
    await showFiles(page)
    await expect(page.locator('.file-item.active')).toHaveCount(1)
    // Park the pointer somewhere that is not a row, so nothing measured
    // here is carrying a hover state as well.
    await page.mouse.move(0, 0)
    await settled(page, '.file-item')

    const colours = await page.evaluate(() => {
      const rows = [...document.querySelectorAll('.file-item')] as HTMLElement[]
      const active = rows.find((r) => r.classList.contains('active')) ?? null
      const plain = rows.find((r) => !r.classList.contains('active')) ?? null
      return {
        selected: active ? getComputedStyle(active).backgroundColor : null,
        plain: plain ? getComputedStyle(plain).backgroundColor : null,
      }
    })
    expect(colours.selected).not.toBeNull()
    expect(colours.plain).not.toBeNull()
    expect(colours.selected, 'the selected row is painted like every other row')
      .not.toBe(colours.plain)
  })

  // A guard rather than a pinned defect. `sbnn export` writes one file to be
  // read from disk or handed to someone; a page that reaches for a font, a
  // script or an image on some host is a page that renders differently
  // offline and tells that host who opened the review. Measured as request
  // count rather than by reading the markup, because a data: URI, an inline
  // <style> and a CDN link all look alike in the source and only one of them
  // goes out.
  test('the exported page contacts no network host (#55)', async ({ page }) => {
    const external: string[] = []
    page.on('request', (r) => {
      if (!r.url().startsWith('file://') && !r.url().startsWith('data:')) external.push(r.url())
    })
    await page.goto(pathToFileURL(handoff().exportPath).href, { waitUntil: 'domcontentloaded' })
    await page.locator('.diff-table').first().waitFor({ state: 'visible' })
    // The page is static, so once the table is up nothing else is coming;
    // give a late request a moment to show up anyway.
    await page.waitForTimeout(500)

    // A page that rendered nothing would also make no requests, so say what
    // was on screen while the count was taken. The narrow layout shows one
    // file at a time (8 rows, 90 nodes); the wide one shows all seven tables
    // (55 rows, 617 nodes).
    const shown = await page.evaluate(() => ({
      nodes: document.querySelectorAll('*').length,
      rows: document.querySelectorAll('.diff-table tr').length,
    }))
    expect(shown.rows, 'diff rows in the exported page').toBeGreaterThan(5)
    expect(shown.nodes, 'DOM nodes in the exported page').toBeGreaterThan(50)
    expect(external, 'requests the exported page made off the local file').toEqual([])
  })
})
