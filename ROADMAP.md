# sitatame ROADMAP

着手順や優先度はまだ決めていない、思いつき列挙の作業ファイル。
固まったものから Issue に切り出し、本ファイルから `#NN` を貼って参照する。

凡例:
- `[ ]` 未着手
- `[~]` 着手中 / 部分実装あり
- `[x]` 完了（ただし完了したらここから消して CHANGELOG / commit に残す方向でも可）
- `(?)` やるか自体を悩み中

---

## 配布 / インストール

- [ ]

## TUI 描画 / 操作

- [ ] side-by-side レイアウト（GitHub の diff view 風）
    - 左右に base / head を並べて表示するモード。
    - 既存の縦並び表示と切り替えられるように。
- [ ] コメントを入れた行に、わかりやすい色で目印を付ける
    - 行の背景色をうすいオレンジなどに変える、または行番号自体の色を変える。
    - 同じ行に複数コメントを重ねた場合も、コメントが付いているという事実は変わらないので、色は同じ「コメントあり」色のままで OK。

## エージェント連携

- [ ] s で review 完了後、パスを覚える必要があり、どれがレビュー内容だったか忘れてしまう。
    - 今のパスを設定している表示は残しつつ、最後に画面を抜ける前に、パスをクリップボードにコピーしておくか確認するようにしたい。

## レビュー検索 / 閲覧

- [ ]

## 永続化 / スキーマ

- [ ]

## 設定ファイル / プロジェクト設定

- [ ] リポジトリごとの設定ファイル（例: `.sitatame/config.toml`）
    - デフォルト base の上書き（例: `base = "origin/develop"`）
    - 自動検出順（`BaseCandidates`）のカスタマイズ
    - 将来の表示オプションなどの置き場としても使う

## 品質 / 開発ツール

- [ ]

## リファクタリング

- [ ] A. `cmd/save.go` を切り出す
    - 現状 `cmd/save_test.go` (138 行) は既にあるのに `save.go` が無く、テスト対象の `QuitReason` 分岐 / `SaveDraft` / `Promote` / `SITATAME_REVIEW=` 出力は `cmd/root.go:194-219` に埋もれている。命名上の不整合。
    - `RunRoot` の末尾 save 分岐をそのまま `cmd/save.go` に move し、`finalize(env, store, result) int` 程度のシグネチャに揃える。
    - 結果として `RunRoot` も 30 行ほど短くなり、ファイル単位で責務が読める。元案 #1 はこの形でやれば十分。
- [ ] B. TUI `Update` の modal / main を二相分離する
    - `internal/tui/model.go:74-78` で `if m.modal != nil { return updateModal }` と早期分岐しており、modal アクティブ時は textarea が入力を全部食う＝main mode の `switch` と入力規約がそもそも違う。
    - `updateMain(msg) (Model, tea.Cmd)` と `updateModal(msg) (Model, tea.Cmd)` をサブ FSM として分け、`Update` 自体はモード判定だけにする。
    - 将来コメント以外のダイアログ（保存確認、レビュー一覧など）を増やす時、modal 側のディスパッチに閉じて足せるようになる。
- [ ] C. `gitx` を runner / parser / orchestrator に三層化する
    - 現状 `internal/gitx/` に `Repo.run`（exec 層）/ パーサ群（pure）/ `Diff()`（fuse）が同居。テストの依存軸が混ざる。
    - `internal/gitx/internal/parser/` に `parseRawZ` / `parseNumstatZ` / `parsePatch` / `joinRawAndNumstat` を退避し、`Repo.run` は `gitRunner` インターフェース経由に。`Diff()` は runner と parser を組み合わせる薄い層に寄せる。
    - パーサは既に pure なので git 不要でテスト可能。`Diff()` 自体も runner stub で git を呼ばずに検証できる。元案 #3 を精緻化したもの。

## あとでやるかも（再検討候補）

- [ ] D. `cmd.RunRoot` のステージ関数分解
    - A の save 抽出後に `RunRoot` がまだ重いと感じたら prepare / run / finalize に分ける。A 単体で十分かもしれないので、A の後で再評価する。
- [ ] E. TUI キーディスパッチのテーブル駆動化
    - B の二相分離だけで読みやすさは大きく改善する見込み。「リポジトリごとの設定ファイル」でキーバインド上書きが正式に決まった段階で再着手する（先取りすると YAGNI）。
- [ ] F. `internal/tui/` の subpackage 化
    - 約 2000 行・9 ファイルあるが、`Model` の private フィールドに各ファイル（`updateModal` / `extendSelection` / `renderRow` / `overlayMarker` 等）が密にアクセスしており、分割は export 強制とゲッター量産を招く。Go 慣習でもこの規模は単一パッケージで普通。やる場合は `Model` 周辺の API 設計から再検討が要る。

## ドキュメント

- [ ] コマンド入力を sita など短い形式に変更する
    - 現状は `sitatame` でちょっと長いので、`sita` など短くする。

## 検討中（やるか悩み中）

- (?)

## やらない / Out of scope

-
