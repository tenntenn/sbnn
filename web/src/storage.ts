/**
 * Remembered settings - pane sizes, the theme - kept where the browser lets
 * us keep them.
 *
 * Reaching for localStorage is not always allowed: a sandboxed iframe throws
 * on the property access itself, and so does a browser told to block site
 * data. An exported page is meant to be embedded in exactly such a frame
 * (`sbnn export --fragment`, an artifact), where an unguarded read during
 * render leaves the reader with a blank page instead of the review. A
 * forgotten pane width is not worth that, so every access is guarded and a
 * refusal simply means the defaults.
 */
export function readSetting(key: string): string | null {
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

export function writeSetting(key: string, value: string): void {
  try {
    window.localStorage.setItem(key, value)
  } catch {
    // The setting lasts for this page view, which is the best on offer.
  }
}

/**
 * readStringSet and writeStringSet keep a set of ids under one key.
 *
 * What comes back out is input, not state: an older build, another tab, or a
 * devtools console may have written it, so anything that is not a list of
 * strings reads as an empty set rather than throwing during render. Losing a
 * set of marks is a bad afternoon; throwing here is a blank page.
 */
export function readStringSet(key: string): Set<string> {
  const raw = readSetting(key)
  if (raw === null) return new Set()
  try {
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return new Set()
    return new Set(parsed.filter((item): item is string => typeof item === 'string'))
  } catch {
    return new Set()
  }
}

/** The entries are sorted on the way out so that the stored value only
 * changes when the set does, which keeps it diffable by hand and stops an
 * unchanged set from looking like a write. */
export function writeStringSet(key: string, values: Set<string>): void {
  writeSetting(key, JSON.stringify([...values].sort()))
}
