package diff

import (
	"regexp"
	"strings"

	"github.com/tenntenn/sbnn/internal/model"
)

// generatedTop is how far into a file the declaration is looked for. The
// convention puts it first; a licence header or a shebang can push it down a
// little, and past that it is no longer a declaration anybody reads.
const generatedTop = 10

// generatedMarkers are the ways a file says it was generated.
//
// Every one of them is the file speaking about itself: a generator wrote the
// line so that tools would leave the file alone. That is what makes folding
// such a file honest, and it is why sbnn looks for nothing else - not a size,
// not a directory, not a name. Those would be sbnn guessing about a project it
// knows nothing about, and a wrong guess hides code from a review.
var generatedMarkers = []*regexp.Regexp{
	// The Go convention, also emitted by many tools outside Go.
	regexp.MustCompile(`Code generated .* DO NOT EDIT\.?`),
	// The @generated tag, understood by GitHub, Phabricator and others.
	regexp.MustCompile(`@generated\b`),
	// The same statement in the words other generators use.
	regexp.MustCompile(`(?i)(automatically |auto-?)?generated( file)?[ ,.:;!-]*.{0,40}do not (edit|modify)`),
	regexp.MustCompile(`(?i)do not (edit|modify)[ ,.:;!-]*.{0,40}(automatically |auto-?)?generated`),
	// The same statement in Japanese. A file whose header says it was
	// generated and not to be edited is speaking about itself just as
	// plainly as one that says @generated, and a project cannot be asked to
	// write that header in English before its diff can be reviewed properly.
	//
	// The two halves are required together, and within 40 characters of each
	// other, exactly as the English patterns above require them. Either half
	// alone is ordinary prose: "テンプレートから Go のコードを自動生成する"
	// is a package saying what it does, "編集不可" is a UI label, and
	// "設定ファイルは自動生成されません" says the opposite of a
	// declaration. Matching those folded away hand-written code, and the
	// English side never did - it has always wanted "generated" *and* "do
	// not edit".
	regexp.MustCompile(jaGenerated + `.{0,40}` + jaDoNotEdit),
	regexp.MustCompile(jaDoNotEdit + `.{0,40}` + jaGenerated),
	// "この(ファイル|コード)は ... 生成された" needs no second half: naming
	// this very file as the thing that was generated is the declaration, the
	// way @generated is. The topic marker は is what carries that - "この
	// ファイルの生成ロジック" is a generator talking about its own work and
	// is left alone.
	//
	// Being generated has to be the whole of what the sentence says, though.
	// "このコードはテンプレートから生成されるデータを扱う" names this file
	// as the handler of generated data, not as the generated thing, and the
	// only way to tell the two apart is that a declaration stops once it has
	// been made: it ends on 生成され(た|ます|ました), with nothing after it
	// but a full stop and the closer of the comment it lives in.
	regexp.MustCompile(`この(ファイル|コード)は[^。]{0,30}生成され(ました|ます|ています|ている|た)` + jaLineEnd),
	// A sentence whose predicate is "is a generated file", and which ends
	// there. "自動生成されたコードが正しいかを検証する" is the same words
	// used as the subject of another clause, and is not a declaration.
	regexp.MustCompile(`(自動生成|自動的に生成|自動作成|自動的に作成)された?(ファイル|コード)(です|である|だ)`),
}

// jaGenerated is a Japanese file saying it was generated. The English verb is
// in here too, because a header that mixes the two - "編集不可: generator が
// 上書きします" - is a real thing generators emit.
//
// The bare verb is deliberately not in here. "生成する" is what a package
// does ("ID を生成する") and "生成され" is any clause about generated
// something ("生成される値"); pairing either with a "do not edit" that
// happens to be about a different thing on the same line folds hand-written
// code away - "// ID を生成する。結果は変更しないこと。" is a comment on a
// function, not a header. English is read the same way: "generated" is
// matched, "generate" only as the stem of "generator"/"generating", and no
// pattern here matches a file that merely talks about generating.
const jaGenerated = `(自動生成|自動的に生成|自動作成|自動的に作成|生成され(ました|ます|ている|ていました|た)|(?i:generat(e|es|ed|or|ors|ing|ion)))`

// jaLineEnd is what may follow a declaration while it is still the last thing
// the line says: a full stop, and the closer of the comment it sits in.
const jaLineEnd = `[。．.!！、\s]*((-->|\*/|\*\)|-}|\?>)\s*)?$`

// jaDoNotEdit is a Japanese file saying not to edit it.
const jaDoNotEdit = `((編集|変更|修正|改変)(を|は)?(し)?ない|(編集|変更|修正|改変)(禁止|不可)|(手|変更)を加えない|書き換えない)`

// GeneratedMarker returns the line by which a file declares itself
// generated, or "" when it does not. The line is returned rather than a
// yes: whatever acts on it can then show why, and be argued with.
func GeneratedMarker(content string) string {
	for i, line := range strings.SplitN(content, "\n", generatedTop+1) {
		if i >= generatedTop {
			break
		}
		for _, re := range generatedMarkers {
			if re.MatchString(line) {
				return strings.TrimSpace(line)
			}
		}
	}
	return ""
}

// VisibleTop returns the beginning of the new side of a file as the diff
// shows it, or "" when the diff does not reach that far.
//
// A unified diff carries only the hunks that changed, so the top of a
// modified file is usually not in it. Nothing is inferred from that absence:
// no top, no declaration, and the file is left alone.
func VisibleTop(f *model.File) string {
	if len(f.Hunks) == 0 || f.Hunks[0].NewStart != 1 {
		return ""
	}
	lines := make([]string, 0, generatedTop)
	for _, l := range f.Hunks[0].Lines {
		if l.Kind == model.LineDelete {
			continue
		}
		lines = append(lines, l.Content)
		if len(lines) == generatedTop {
			break
		}
	}
	return strings.Join(lines, "\n")
}
