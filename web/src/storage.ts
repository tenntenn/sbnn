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
 * What follows reads a setting back as the type the caller wanted.
 *
 * Everything in localStorage is a string that some older build, another tab,
 * or a person with the devtools console open may have written, so a stored
 * value is input rather than state. The rule for all of it is the same one
 * the guards above already follow: a value that is missing, blank, unparsable
 * or out of range means the default. None of these throw, and none of them
 * hand back something the caller then has to check again.
 */

/** readEnumSetting returns the stored value only if it is still one of the
 * ones this build knows. A setting that has been renamed or dropped since it
 * was written therefore reads as the default rather than as a string nothing
 * downstream can match. */
export function readEnumSetting<T extends string>(
  key: string,
  allowed: readonly T[],
  fallback: T,
): T {
  const raw = readSetting(key)
  if (raw === null) return fallback
  return (allowed as readonly string[]).includes(raw) ? (raw as T) : fallback
}

/** readNumberSetting parses a number and holds it to a range. A blank entry
 * is treated as absent, because Number('') is 0 and 0 is a plausible-looking
 * width - the sort of accident that collapses a pane and looks like a bug. */
export function readNumberSetting(
  key: string,
  min: number,
  max: number,
  fallback: number,
): number {
  const raw = readSetting(key)
  if (raw === null || raw.trim() === '') return fallback
  const value = Number(raw)
  if (!Number.isFinite(value) || value < min || value > max) return fallback
  return value
}

/** readBoolSetting and writeBoolSetting agree on 'on' / 'off', which reads
 * better in devtools than '1' and survives a round trip unambiguously -
 * unlike the empty string, which is both a falsy value and a missing one. */
export function readBoolSetting(key: string, fallback: boolean): boolean {
  const raw = readSetting(key)
  if (raw === 'on') return true
  if (raw === 'off') return false
  return fallback
}

export function writeBoolSetting(key: string, value: boolean): void {
  writeSetting(key, value ? 'on' : 'off')
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
