import { readBoolSetting, writeBoolSetting } from './storage'

/**
 * Telling a reader that a new diff arrived while the tab was out of sight.
 *
 * It is one setting, off by default, and it only ever fires when the tab is
 * hidden: a reader looking at the page already sees the diff arrive. The
 * browser's permission prompt is asked for from the settings toggle and
 * nowhere else, because a page that asks on load is a page whose prompt gets
 * dismissed. A refused or unavailable Notification API leaves the tab-title
 * marker, which needs no permission.
 */
export const NOTIFY_KEY = 'sbnn.notify'

export function storedNotify(): boolean {
  return readBoolSetting(NOTIFY_KEY, false)
}

export function storeNotify(on: boolean): void {
  writeBoolSetting(NOTIFY_KEY, on)
}

/** newDiffIds returns the ids in `current` that `seen` has not met. A null
 * `seen` is the first load, when everything is already there rather than new. */
export function newDiffIds(seen: ReadonlySet<string> | null, current: readonly string[]): string[] {
  if (seen === null) return []
  return current.filter((id) => !seen.has(id))
}

const UNREAD = /^\(\d+\) /

/** withUnread puts the unread count in front of a title, replacing a count
 * that is already there. */
export function withUnread(title: string, count: number): string {
  const base = title.replace(UNREAD, '')
  return count > 0 ? `(${count}) ${base}` : base
}

/** unreadCount reads back the count withUnread wrote, or 0. */
export function unreadCount(title: string): number {
  const match = /^\((\d+)\) /.exec(title)
  return match ? Number(match[1]) : 0
}

function permission(): NotificationPermission | 'unsupported' {
  return typeof Notification === 'undefined' ? 'unsupported' : Notification.permission
}

/** requestNotifyPermission asks the browser once, from a click. The answer is
 * only informational: the setting stays on either way, since the title marker
 * does not need it. */
export async function requestNotifyPermission(): Promise<NotificationPermission | 'unsupported'> {
  if (typeof Notification === 'undefined') return 'unsupported'
  if (Notification.permission !== 'default') return Notification.permission
  try {
    return await Notification.requestPermission()
  } catch {
    return Notification.permission
  }
}

/** announceNewDiffs marks the tab title and, where allowed, raises a system
 * notification. It does nothing for a visible tab. */
export function announceNewDiffs(group: string, count: number): void {
  if (count <= 0 || !document.hidden) return
  document.title = withUnread(document.title, unreadCount(document.title) + count)
  if (permission() !== 'granted') return
  try {
    const name = group === '' || group === 'default' ? 'sbnn' : group
    const n = new Notification(`${name}: new diff`, {
      body: count === 1 ? 'A new diff is ready to review.' : `${count} new diffs are ready to review.`,
      tag: `sbnn-${group}`,
    })
    n.onclick = () => {
      window.focus()
      n.close()
    }
  } catch {
    // Some browsers refuse the constructor (Android Chrome); the title stays.
  }
}

/** clearUnread drops the title marker once the tab is looked at again. */
export function clearUnread(): void {
  if (unreadCount(document.title) > 0) document.title = withUnread(document.title, 0)
}
