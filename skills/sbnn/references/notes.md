# Notes

- New files are shown as a unified diff, because there is no old side to put
  next to them.
- Markdown files get a preview pane next to the diff. The preview shows the
  working tree file when it exists; otherwise sbnn rebuilds what it can from
  the diff, and unified diffs only carry the changed hunks, so such a preview
  is partial by nature.
- A comment can also be made by selecting text in that preview, in which case
  its line range covers the whole Markdown blocks the selection touched -
  paragraphs, list, code fence - rather than only the words highlighted. The
  quoted snippet is what was selected.
- Comments are stored by the sbnn server, not in the browser, which is why they
  survive a reload and why you can read them from the command line.
- `sbnn --status --json` is the reliable way to check whether comments are
  waiting: it reports `comments`, `unresolved` and `reviewed` per group.
  `reviewed` is true once the human has submitted, and false again as soon as
  a newer diff arrives.
