package tui

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

// applyCommentHighlight は文字列を commentColorFG で囲んで返す。
// 空文字は SGR を巻いても意味が無いのでそのまま返し、行末リセット漏れを防ぐ。
func applyCommentHighlight(s string) string {
	if s == "" {
		return s
	}
	return commentColorFG + s + commentReset
}
