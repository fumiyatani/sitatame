package tui

import "github.com/fumiyatani/sitatame/internal/review"

// commentColorFG は「コメントあり」を示す ANSI 256-color 文字色 SGR。
// うすオレンジ系の 215 を初期値とし、実機視認性の調整余地としてここに集約する。
const (
	commentColorFG = "\x1b[38;5;215m"
	commentReset   = "\x1b[0m"
)

// hasComment は overlay の「行 → コメントインデックス配列」エントリが
// 1 件以上を持つかを返す。buildOverlay は KindReview を skip 済みなので、
// 空配列はそのまま「色を付けない行」と同義になる。
func hasComment(commentIdxs []int) bool {
	return len(commentIdxs) > 0
}

// resolveActionLabel は対象行で `x` キーを押したときに発生する動作のラベルを返す。
// 内部で Model.resolveTarget を呼ぶため、sticky な lastToggledAnchor も考慮した
// 「次に実際に flip される comment の現在状態」に基づいてラベルを決める。これにより
// 例えば open A + open B が並ぶ行で A を resolve した直後 (sticky=A) は、A が
// 既に resolved なので label が "REOPEN" になり、次の `x` (= A を open に戻す) と
// 一致する。stale-only / 空 overlay は ok=false で stale-only fallthrough に倒れる。
func (m Model) resolveActionLabel(row int) (string, bool) {
	target, ok := m.resolveTarget(row)
	if !ok {
		return "", false
	}
	switch m.Review.Comments[target].State {
	case review.StateOpen:
		return "RESOLVE", true
	case review.StateResolved:
		return "REOPEN", true
	default:
		return "", false
	}
}

// applyCommentHighlight は文字列を commentColorFG で囲んで返す。
// 空文字は SGR を巻いても意味が無いのでそのまま返し、行末リセット漏れを防ぐ。
func applyCommentHighlight(s string) string {
	if s == "" {
		return s
	}
	return commentColorFG + s + commentReset
}
