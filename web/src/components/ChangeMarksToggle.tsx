import { useT } from '../i18n'

interface Props {
  on: boolean
  onChange: (on: boolean) => void
}

/**
 * ChangeMarksToggle turns the preview's change marks (see richDiff.ts) on and
 * off. It is one setting for the whole page, shown in the preview pane's
 * toolbar, or in the file's own header in the narrow layout, which has no
 * toolbar.
 *
 * The glyph is text rather than an Icon: the icon font is a subset, and a
 * name it does not carry would be drawn as the name.
 */
export function ChangeMarksToggle({ on, onChange }: Props) {
  const t = useT()
  return (
    <span title={on ? t('preview.marksOn') : t('preview.marksOff')}>
      <button
        className={`ghost icon-only change-marks-toggle${on ? ' active' : ''}`}
        aria-pressed={on}
        aria-label={t('preview.marks')}
        onClick={() => onChange(!on)}
      >
        <span className="change-marks-glyph" aria-hidden>
          ±
        </span>
      </button>
    </span>
  )
}
