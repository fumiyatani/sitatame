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

// hasToggleableComment は overlay の対象行が「`x` で状態を切り替え可能なコメント」を
// 1 件以上抱えているかを返す。toggleResolvedAtCursor は StateStale を意図的にスキップ
// するため、stale のみの行で has-comment hint を出すと `x RESOLVE` の案内に反して何も
// 起きず、ユーザーを誤誘導する。hint の出し分けはこの helper で stale-only 行を弾く。
func hasToggleableComment(commentIdxs []int, comments []review.Comment) bool {
	_, ok := resolveActionLabel(commentIdxs, comments)
	return ok
}

// resolveActionLabel は overlay 上のコメント群から `x` キーの実効動作を返す。
// toggleResolvedAtCursor の優先順位 (open ≥ 1 → resolve, それ以外で resolved ≥ 1 →
// reopen, stale のみ → no-op) と一致させ、hint のラベルとキー挙動の不一致を防ぐ。
// ok=false は stale のみ／空 overlay／インデックス無効の「押しても何も起きない」状態。
func resolveActionLabel(commentIdxs []int, comments []review.Comment) (string, bool) {
	hasOpen, hasResolved := false, false
	for _, idx := range commentIdxs {
		if idx < 0 || idx >= len(comments) {
			continue
		}
		switch comments[idx].State {
		case review.StateOpen:
			hasOpen = true
		case review.StateResolved:
			hasResolved = true
		}
	}
	switch {
	case hasOpen:
		return "RESOLVE", true
	case hasResolved:
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
