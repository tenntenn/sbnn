// A review of a few hundred files must not put a few hundred files in the
// DOM (#390). Before sections were mounted lazily, 300 files of 40 lines were
// about 150,000 elements on load; now only the sections near the viewport
// carry a body, whatever the size of the review.
import { test, expect, type Page } from '@playwright/test'
import { MANY_FILES, MANY_GROUP, handoff } from './harness'

// Generous: a mounted section here is a few hundred elements and the page
// keeps a few of them. The failure this guards is an order of magnitude
// above it, and so is the margin.
const MAX_ELEMENTS = 20_000

async function open(page: Page): Promise<void> {
  await page.goto(`${handoff().baseURL}/${MANY_GROUP}`, { waitUntil: 'domcontentloaded' })
  await page.locator('.diff-table').first().waitFor({ state: 'visible' })
}

function elements(page: Page): Promise<number> {
  return page.evaluate(() => document.getElementsByTagName('*').length)
}

test.describe('lazily mounted sections', () => {
  test.skip(({ viewport }) => (viewport?.width ?? 0) < 800, 'the one-file phone layout mounts a single section anyway')

  test('only the sections near the viewport have a body', async ({ page }) => {
    await open(page)
    await expect(page.locator('.diff-stack .file-section')).toHaveCount(MANY_FILES)
    expect(await elements(page), `DOM elements with ${MANY_FILES} files`).toBeLessThan(MAX_ELEMENTS)
    expect(await page.locator('.diff-stack .file-section:has(table)').count()).toBeLessThan(MANY_FILES / 5)
  })

  test('a file far down is mounted once the sidebar jumps to it, and the first one is let go', async ({ page }) => {
    await open(page)
    await page.locator('.sidebar .file-item').last().click()
    await expect(page.locator('.diff-stack .file-section').last().locator('table')).toBeVisible()
    await expect(page.locator('.diff-stack .file-section').first().locator('table')).toHaveCount(0)
    expect(await elements(page)).toBeLessThan(MAX_ELEMENTS)
  })

  // The browser's find bar only searches what is on the page, and it sends the
  // page nothing but the key that opens it (#398). Headless Chromium has no
  // find bar, so window.find stands in for it.
  test('Ctrl+F mounts every body in batches so find can see a far file, and Escape lets go', async ({ page }) => {
    await open(page)
    const far = '39 * 299'
    const find = () => page.evaluate((t) => window.find(t, true, false, true), far)
    expect(await find(), 'the last file is not searchable before the key').toBe(false)
    await page.keyboard.press('Control+f')
    await expect(page.locator('.diff-stack .file-section:has(table)')).toHaveCount(MANY_FILES, { timeout: 60_000 })
    expect(await find(), 'the last file is searchable once everything is mounted').toBe(true)
    await page.evaluate(() => window.getSelection()?.removeAllRanges())
    await page.keyboard.press('Escape')
    await expect.poll(() => page.locator('.diff-stack .file-section:has(table)').count()).toBeLessThan(MANY_FILES / 5)
    expect(await elements(page)).toBeLessThan(MAX_ELEMENTS)
  })
})
