import test from 'node:test'
import assert from 'node:assert/strict'

import { renderBlocks } from '../src/lineMarks'

/**
 * A comment made from a selection in the preview used to cover the whole top
 * level block the selection touched, so one word in a long list or code block
 * commented on dozens of lines. The preview now marks the line of every
 * paragraph line, list item, table row and code line, and the page reads a
 * selection back as the lines of the marks it lies between.
 *
 * Reading the selection needs a DOM and cannot be driven here. What can is
 * the half that decides the numbers: which mark lands in front of which
 * text. Each case renders a Markdown source with the marks written as
 * "m" and reduces the result to the text with each mark shown as [line], so
 * a case reads as "this word is on this line".
 */

const render = (source: string, startLine = 1) =>
  renderBlocks(source, startLine, 'm', (html) => html)
    .replace(/<span m="([\d-]+)"><\/span>/g, '[$1]')
    .replace(/<div data-ln="([\d-]+)">/g, '{$1:')
    .replace(/<\/div>/g, '}')

const cases = [
  {
    name: 'a paragraph marks each of its lines',
    source: 'one\ntwo\nthree\n',
    want: '{1-3:<p>[1]one\n[2]two\n[3]three</p>\n}',
  },
  {
    name: 'emphasis running over a line break stays whole and the next line is marked',
    source: 'a *b\nc* d\n',
    want: '{1-2:<p>[1]a <em>b\n[2]c</em> d</p>\n}',
  },
  {
    name: 'a hard break marks the line after it',
    source: 'a  \nb\n',
    want: '{1-2:<p>[1]a<br>[2]b</p>\n}',
  },
  {
    name: 'a heading marks the lines it spans',
    source: '# T\n',
    want: '{1-1:<h1>[1]T</h1>\n}',
  },
  {
    name: 'each list item is its own line, nested ones included',
    source: '- a\n  b\n- c\n  - d\n- e\n',
    want: '{1-5:<ul>\n<li>[1][1]a\n[2]b</li>\n<li>[3][3]c<ul>\n<li>[4][4]d</li>\n</ul>\n</li>\n<li>[5][5]e</li>\n</ul>\n}',
  },
  {
    name: 'a loose list item marks its paragraphs',
    source: '1. x\n\n   y\n2. z\n',
    want: '{1-4:<ol>\n<li>[1]<p>[1]x</p>\n<p>[3]y</p>\n</li>\n<li>[4]<p>[4]z</p>\n</li>\n</ol>\n}',
  },
  {
    name: 'a fenced code block marks its lines and not its fences',
    source: 'x\n\n```go\nl1\n\nl2\n```\n',
    want: '{1-1:<p>[1]x</p>\n}{3-7:<pre><code class="language-go">[4]l1\n[5]\n[6]l2\n</code></pre>\n}',
  },
  {
    name: 'an indented code block starts on its own first line',
    source: '    a\n    b\n',
    want: '{1-2:<pre><code>[1]a\n[2]b\n</code></pre>\n}',
  },
  {
    name: 'a table marks the header and every row, skipping the delimiter row',
    source: '| h |\n|---|\n| 1 |\n| 2 |\n',
    want: '{1-4:<table>\n<thead>\n<tr>\n<th>[1]h</th>\n</tr>\n</thead>\n<tbody><tr>\n<td>[3]1</td>\n</tr>\n<tr>\n<td>[4]2</td>\n</tr>\n</tbody></table>\n}',
  },
  {
    name: 'a quote marks the lines of its paragraph',
    source: '> q1\n> q2\n',
    want: '{1-2:<blockquote>\n<p>[1]q1\n[2]q2</p>\n</blockquote>\n}',
  },
  {
    name: 'a block after a frontmatter is numbered from where the body starts',
    source: 'a\nb\n',
    startLine: 5,
    want: '{5-6:<p>[5]a\n[6]b</p>\n}',
  },
  {
    name: 'a rule has no finer mark and stands for its own line',
    source: 'a\n\n---\n\nb\n',
    want: '{1-1:<p>[1]a</p>\n}{3-3:<hr>\n}{5-5:<p>[5]b</p>\n}',
  },
]

for (const c of cases) {
  test(c.name, () => {
    assert.equal(render(c.source, c.startLine), c.want)
  })
}
