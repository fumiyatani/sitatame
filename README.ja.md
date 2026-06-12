# sitatame

PR を出す前に、自分の git diff を端末上でレビューするための TUI ツール。
`sitatame` は `git diff <base>..HEAD` を bubbletea ベースの TUI で表示し、
4 粒度（review 全体 / file / line / range）でコメントを残せます。
保存結果は `~/.sitatame/<project-slug>/reviews/` 配下に YAML front matter +
Markdown 形式で書き出され、後段のエージェントがそのまま読み取れます。

## 技術スタック

- **言語 / ランタイム**: Go 1.26（`go.mod` は `go 1.26.2`）
- **必須外部ツール**: `$PATH` 上の `git`
- **任意外部ツール**: `ripgrep`（`rg`）— あれば `sitatame search` で利用。
  無くても `internal/search/` の Go regexp フォールバックで動作
- **TUI ランタイム**:
  [`charmbracelet/bubbletea`](https://github.com/charmbracelet/bubbletea) v1
  を Model / Update / View ループの基盤として、
  [`bubbles/textarea`](https://github.com/charmbracelet/bubbles) をコメント
  入力欄に、（間接依存の）`lipgloss` をスタイル付けに使用
- **端末幅計算**: [`mattn/go-runewidth`](https://github.com/mattn/go-runewidth)
  を `EastAsianWidth=false` で運用し、locale 依存の幅ブレを排除
- **永続化**: [`gopkg.in/yaml.v3`](https://gopkg.in/yaml.v3) を `yaml.Node`
  と併用し、未知キーを decode → encode round trip で温存
- **ID 生成**: [`google/uuid`](https://github.com/google/uuid) でコメント
  アンカー ID を採番
- **標準ライブラリ**: `os/exec`（git / ripgrep プラグイン経由）、`regexp`、
  `bufio`、`path/filepath`、`syscall`（`internal/termcheck/` で build tag
  経由 TTY 判定）、`time`、`flag`
- **テスト / ツール**:
  - `go test ./...`（ユニット + 統合）
  - `internal/tui/testdata/` 配下の golden 比較（ANSI を除去してから diff）
  - TUI ホットパス向けの `BenchmarkUpdate_LargeDiff`
  - Makefile target: `make build` / `make build-all`（darwin & linux × amd64
    & arm64）/ `make install` / `make bench` / `make update-golden` /
    `make vet`

## ビルド / インストール

`sitatame` は Go 1.26 以降と `git` コマンドが必要です。

```sh
git clone https://github.com/fumiyatani/sitatame
cd sitatame
make build      # ./sitatame を生成
make install    # go install ./... — $GOBIN に配置
```

サポート対象 4 種のクロスビルド:

```sh
make build-all  # dist/sitatame-{darwin,linux}-{amd64,arm64} を生成
```

> Phase 2 では GitHub Releases によるバイナリ配布、
> `go install github.com/fumiyatani/sitatame@<version>` 形式での導入を予定。

## 使い方

git ワーキングツリー内で実行します:

```sh
sitatame                # base を自動検出（origin/HEAD, @{upstream}, main, …）
sitatame origin/main    # base を明示指定
sitatame --staged       # ステージ済みの変更をレビュー（index vs HEAD）
sitatame --working      # 未コミットの全変更をレビュー（worktree vs HEAD）
sitatame search TODO    # ~/.sitatame/<project-slug>/reviews/ を grep
```

キーバインド:

```
j / k       カーソル下 / 上
n / p       次 / 前のファイル
f           ファイルピッカーモーダル（任意のファイルへジャンプ）
wheel       diff をスクロール（Option/Fn 押下でテキスト選択）
r           範囲選択開始（j/k で拡張、Esc で解除）
c           カーソル位置にコメント（kind は選択 / 行種別から自動決定）
x           カーソル位置のコメントを resolved ↔ open でトグル（stale はスキップ）
Shift+R     review 全体コメント（front matter の review_comment を編集）
s           保存して promote — ~/.sitatame/<project-slug>/reviews/<branch-slug>/<id>.md に書き出し、
            stdout に SITATAME_REVIEW=<絶対パス> を 1 行出力
q           draft として保存し exit 1（~/.sitatame/<project-slug>/drafts/<branch-slug>/<id>.md）
?           ヘルプ表示の切り替え
Esc         モーダルを閉じる / 選択解除

コメントモーダル内:
Ctrl+S      コメントを確定して追加
Esc         保存せずキャンセル
```

操作例:

```
# 行コメント
sitatame → j/k で対象行へ → c → 本文を入力 → Ctrl+S → s

# 範囲コメント
sitatame → j/k で起点へ → r → j/k で範囲を拡張 → c → 本文を入力 → Ctrl+S → s

# review 全体コメント（front matter）
sitatame → Shift+R → 本文を入力 → Ctrl+S → s
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
   SITATAME_REVIEW=/abs/path/to/.sitatame/<project-slug>/reviews/<branch-slug>/<id>.md
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
このスクリプトは promote 済み Markdown を `sh -c` 経由で `$SITATAME_AGENT`
に流すため、`SITATAME_AGENT` には**自分が完全に信頼できるコマンドのみ**を
設定してください。外部由来の値を流し込まないでください。

未コミット / 未ステージの変更をレビューする場合:

```sh
sitatame --staged    # ステージ済みの変更を HEAD と比較（git diff --cached）
sitatame --working   # worktree を HEAD と比較（staged + unstaged の両方）
```

いずれも base 自動検出はスキップし、レビューには `base.ref: HEAD` と
`head.ref: INDEX`（`--staged`）または `head.ref: WORKTREE`（`--working`）が
記録されます。対象の変更がない場合は TUI を起動せずに stderr へメッセージを
出して exit 0 で終了します。

`--staged` / `--working` は相互排他で、明示的な base 引数とも併用できません。
未追跡ファイルは含まれないので、必要なら事前に `git add -N <path>` を実行して
ください。

`q` で抜けた場合は exit 1 となり、draft が
`~/.sitatame/<project-slug>/drafts/<branch-slug>/<id>.md` に残ります。
次回セッションで拾い直すか、draft 段階を意識するエージェントに先に渡しても
良い設計です。

### 保存先

レビュー / draft はリポジトリツリーの外に書き出すため、プロジェクトごとに
ignore ルールを足す必要はありません。出力ルートは次の順で解決します:

1. `$SITATAME_HOME` が設定されていればそれを使用
2. 既定値として `~/.sitatame`
3. 上記が解決できない場合の最終フォールバックとして `$TMPDIR/sitatame`
   （stderr に警告を 1 行出力）

出力ルート配下では、リポジトリの checkout ごとに `<project-slug>/`
ディレクトリが作られます。slug は basename と絶対パスのハッシュから派生
するので、同じリポジトリの別 checkout（worktree など）も衝突しません。

旧版が残した `<repo>/.sitatame/` ディレクトリが存在する場合は起動時に
stderr で 1 行通知を出しますが、自動移行や参照は行いません。必要なデータを
コピーしたら手動で削除してください。

旧 in-repo の draft を引き取りたい場合、stderr に出力される移行先パスを
そのまま使えば `<project-slug>` を手で計算する必要はありません:

```sh
# 旧ディレクトリが残っているリポジトリ内で一度だけ実行。stderr の 2 行目に
#   sitatame: To migrate drafts: mkdir -p '/Users/you/.sitatame/<project-slug>/drafts' && mv '/path/to/repo/.sitatame'/drafts/* '/Users/you/.sitatame/<project-slug>/drafts'/
# が出ているので、その 1 行をそのまま流す（パスは POSIX の single quote で
# 囲まれているため、checkout や `$SITATAME_HOME` にスペースやシェル
# メタ文字が含まれていてもそのまま貼り付けて動く。末尾の `/drafts/*` は
# シェルに glob 展開させるため quote の外に出している。初回アップグレード時は
# 新 drafts root が未作成のため `mkdir -p` を同梱している）。空になった旧
# ディレクトリは別途削除する。下の例は同じ形を `~` で簡略化したもの。
# repo パスや `$SITATAME_HOME` にスペースが含まれる場合は、こちらではなく
# stderr の行をそのままコピペすること。
mkdir -p ~/.sitatame/<project-slug>/drafts && mv .sitatame/drafts/* ~/.sitatame/<project-slug>/drafts/ 2>/dev/null || true
rm -rf .sitatame
```

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
