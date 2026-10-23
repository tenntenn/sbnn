interface Props {
  small?: boolean
}

/**
 * CommentIcon is a speech bubble with a plus in it: "add a comment".
 *
 * It is drawn here rather than taken from the icon font because the font is a
 * hand-picked subset (see styles.css) without a comment glyph, and growing
 * the subset needs the full Material Symbols font and a subsetter that a
 * contributor to this page should not have to install. Like MoIcon it takes
 * currentColor, so it follows the button it sits in through both themes.
 */
export function CommentIcon({ small }: Props) {
  const size = small ? 14 : 16
  return (
    <svg
      className="icon"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      style={{ flex: 'none' }}
    >
      <path d="M5 4h14a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H9l-5 4V6a2 2 0 0 1 1-2z" />
      <path d="M12 7.5v6M9 10.5h6" />
    </svg>
  )
}
