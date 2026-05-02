# sitatame

PR を出す前に、自分の git diff を端末上でレビューするための TUI ツール。
`sitatame` は `git diff <base>..HEAD` を bubbletea ベースの TUI で表示し、
4 粒度（review 全体 / file / line / range）でコメントを残せます。
保存結果は `.sitatame/reviews/` 配下に YAML front matter + Markdown 形式で
書き出され、後段のエージェントがそのまま読み取れます。

## ビルド / インストール

`sitatame` は Go 1.26 以降と `git` コマンドが必要です。

```sh
git clone https://github.com/tanifumiya/sitatame
cd sitatame
make build      # ./sitatame を生成
make install    # go install ./... — $GOBIN に配置
```

サポート対象 4 種のクロスビルド:

```sh
make build-all  # dist/sitatame-{darwin,linux}-{amd64,arm64} を生成
```

> Phase 2 では GitHub Releases によるバイナリ配布、
> `go install github.com/tanifumiya/sitatame@<version>` 形式での導入を予定。

## 使い方

git ワーキングツリー内で実行します:

```sh
sitatame                # base を自動検出（origin/HEAD, @{upstream}, main, …）
sitatame origin/main    # base を明示指定
sitatame search TODO    # .sitatame/reviews/ を grep
```

キーバインド:

```
j / k       カーソル下 / 上
n / p       次 / 前のファイル
V           範囲選択開始（j/k で拡張、Esc で解除）
c           カーソル位置にコメント（kind は選択 / 行種別から自動決定）
R           review 全体コメント（front matter の review_comment を編集）
s           保存して promote — .sitatame/reviews/<slug>/<id>.md に書き出し、
            stdout に SITATAME_REVIEW=<絶対パス> を 1 行出力
q           draft として保存し exit 1（drafts/<slug>/<id>.md）
?           ヘルプ表示の切り替え
Esc         モーダルを閉じる / 選択解除
```

ガッタの状態マーカー:

- `*` open
- `~` stale（アンカー先のコンテンツが drift。コメントは read-only）

## エージェント連携

`sitatame` は別プロセス（典型的にはコーディングエージェント）が
画面スクレイプなしでレビュー結果を消費できることを前提に設計しています:

1. レビュー対象のブランチで `sitatame` を起動。
2. レビュアがレビューを終え `s`（save & promote）を押下。
3. `sitatame` は exit 0 で終了し、**stdout** に機械可読行を 1 行出力:

   ```
   SITATAME_REVIEW=/abs/path/to/.sitatame/reviews/<slug>/<id>.md
   ```

4. その path を capture します。ファイルは YAML front matter + Markdown 本文。
   front matter には `schema`、`branch`、`base.{ref,sha}`、`head.{ref,sha}`、
   `comments` リスト（各エントリに `kind: review|file|line|range`、`path`、
   `side`、必要に応じて `line` / `line_start` / `line_end`、staleness 判定用の
   blob ハッシュ、`body` を含む）が並びます。`sitatame` がモデル化していない
   キーは round-trip で温存されるため、エージェント側でスキーマを拡張しても
   次回保存時に失われません。
5. 過去レビューの読み戻しは `sitatame search <pattern>` を使います。

シェルでの最小受け渡しはこの形:

```sh
REVIEW_PATH=$(sitatame HEAD~1 | awk -F= '/^SITATAME_REVIEW=/{print $2}')
test -n "$REVIEW_PATH" || { echo "review が capture できませんでした" >&2; exit 1; }
cat "$REVIEW_PATH" | your-agent --consume-review
```

実行可能なサンプルは
[`examples/agent-handoff.sh`](examples/agent-handoff.sh) にあります。

未コミット / 未ステージの変更を見たい場合は、現状は一時コミットを挟むのが
ワークアラウンドです:

```sh
git add -A
git commit -m "wip:review"
sitatame HEAD~1
git reset --soft HEAD~1   # コミットだけ取り消し、変更は index/working に戻る
```

`--staged` / `--working` フラグでこれを直接サポートするのは Phase 2 の予定です。

`q` で抜けた場合は exit 1 となり、draft が
`.sitatame/drafts/<slug>/<id>.md` に残ります。次回セッションで拾い直すか、
draft 段階を意識するエージェントに先に渡しても良い設計です。

## 開発

```sh
make test            # go test ./...
make bench           # 数千行規模での Update / View ベンチ
make update-golden   # 意図的な見た目変更後に internal/tui/testdata/*.golden を再生成
make vet
make fmt
```

TUI テストは `internal/tui/testdata/` のスナップショットを使います。
比較前に ANSI エスケープを除去するため、ホスト / locale が変わってもテストが
ぐらつかないようになっています。

## Phase 2（MVP 範囲外）

- GitHub Release / goreleaser による配布、`go install <module>@<version>` 経由の導入
- Homebrew tap / aqua / mise 連携
- delta パイプ統合によるシンタックスハイライト diff
- 左ペインツリー / side-by-side レイアウト

---

English version: [README.md](README.md)
