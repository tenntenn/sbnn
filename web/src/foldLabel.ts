/** foldLabel explains a fold in words the reader can check.
 *
 * foldReason is written by the server, for the folds it performs itself;
 * sbnn never folds a file on its own without one. A fold the reader
 * performed has no reason to state and is not the sender's doing, so it
 * says whose it is instead of borrowing an explanation that is not true.
 *
 * The two can be true at once, and then both are said. A file the sender
 * folded can be opened by the reader and folded again by hand, which leaves
 * a reader's fold standing on a file the sender had also folded, for a
 * stated reason. Dropping the reason there loses the one fact the reader
 * cannot recover from the page - why it arrived folded - so the line keeps
 * it and attributes the fold now standing to the reader.
 *
 * The remaining case - folded, no reason, not by the reader - should not
 * arise, and says only what is certain. */
export function foldLabel(byReader: boolean, foldReason: string | undefined): string {
  if (byReader && foldReason) return `Folded by you — the sender had folded it too: ${foldReason}`
  if (byReader) return 'Folded by you'
  if (foldReason) return `Folded — ${foldReason}`
  return 'Folded'
}
