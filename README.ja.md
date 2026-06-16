# sitatame

> **プロジェクトステータス (2026-06)**: TUI はメンテナンスモードに入りました。
> 新規機能の開発は [`web/`](web/) 配下の Kotlin Web UI と
> [`intellij/`](intellij/) 配下の IntelliJ Plugin に移行しています。
> TUI のバグ修正は引き続き歓迎しますが、新機能は Web UI または
> IntelliJ Plugin 側で実装します。詳細・全 TUI キーバインドの棚卸し・
> 3 サーフェスでの機能対比表は
> [docs/tui-status.md](docs/tui-status.md) を参照してください。

PR を出す前に、自分の git diff を端末上でレビューするための TUI ツール。
`sitatame` は `git diff <base>..HEAD` を bubbletea ベースの TUI で表示し、
4 粒度（review 全体 / file / line / range）でコメントを残せます。
保存結果は `~/.sitatame/<project-slug>/<branch-slug>/review.md` に
YAML front matter + Markdown 形式で書き出され、後段のエージェントがそのまま読み取れます。

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
- **Web UI**: Kotlin Multiplatform 2.1.10、Wasm client は Compose
  Multiplatform 1.7.3、JVM backend は Ktor 3.0.3、Gradle wrapper
  8.11.1、JDK 21 toolchain
- **IntelliJ Plugin**: Kotlin/JVM 2.1.10、JetBrains IntelliJ Platform
  Gradle Plugin 2.1.0、target は IntelliJ IDEA Community 2024.3
  （`sinceBuild=243`, `untilBuild=251.*`）、bundled `Git4Idea`、JDK 21
  toolchain

## ビルド / インストール

`sitatame` の実行には `git` のみが必要です。Go 1.26 以降は、ソースからの
ビルドまたは `go install` を使う場合のみ必要になります。

### プリビルドバイナリ

[releases ページ](https://github.com/fumiyatani/sitatame/releases) から
プラットフォームに合わせた未署名バイナリをダウンロードできます。タグごとに
`sitatame-{darwin,linux}-{amd64,arm64}` の 4 種類と、SHA-256 ハッシュを記録
した `checksums.txt` が添付されます。

```sh
# macOS arm64 の例 — OS/arch に合わせてアセット名を差し替えてください。
curl -L https://github.com/fumiyatani/sitatame/releases/latest/download/sitatame-darwin-arm64 \
  -o /usr/local/bin/sitatame
chmod +x /usr/local/bin/sitatame
```

macOS バイナリはまだ codesign / notarization をしていないため、初回起動時に
Gatekeeper が隔離します。ダウンロード元を信頼している場合は手動で属性を
解除してください:

```sh
xattr -d com.apple.quarantine /usr/local/bin/sitatame
```

署名 / notarization と Homebrew tap は Phase 2 で対応予定です。

### `go install`

```sh
go install github.com/fumiyatani/sitatame@latest
# タグを固定する場合:
go install github.com/fumiyatani/sitatame@v0.1.0
```

### ソースからのビルド

```sh
git clone https://github.com/fumiyatani/sitatame
cd sitatame
make build      # ./sitatame を生成
make install    # go install ./... — $GOBIN に配置
```

リリースワークフローと同じ 4 ターゲットのクロスビルド:

```sh
make build-all  # dist/sitatame-{darwin,linux}-{amd64,arm64} を生成
```

## Web UI

[`web/`](web/) の Web UI は、sitatame レビューインターフェースの Kotlin
Multiplatform 実装です。JVM target の Ktor backend が現在のリポジトリを読み、
`git diff origin/main..HEAD` と共有 sitatame storage 内の最新 review Markdown
を JSON として返します。Wasm target はそのデータを Compose for Web で表示します。

**機能一覧** (Phase 1 step 2 + Issue #18 UX):

- **読み取り**: unified diff 表示、ファイル / hunk ナビゲーション、コメント表示
- **書き込み**:
  - line / range / file / overall の各粒度でコメントを追加
  - コメントの resolve / reopen（optimistic UI）
  - review-level のサマリーコメントを編集
  - 同時編集検知: ETag ベースの 412 ハンドリング（Reload + retry または Discard）
- **コメント UX**（Issue #18）:
  - GitHub 風スレッド: 同じアンカーを共有するコメントを 1 つの折りたたみ可能なスレッドに集約
  - 状態フィルタ（`All / Open / Done / Stale`）でサイドバーのスレッドを絞り込み
  - Open / Stale スレッドはデフォルト展開、Resolved スレッドはデフォルト折りたたみ
  - 「Reply to this thread」ボタンで既存アンカーを引き継いで返信
  - 状態別の視覚区別: Open（デフォルト背景）、Resolved（暗いグレー）、Stale（アンバー）

range コメントは **long-press** で range モードを開始し、同じ hunk 内の終了行を
クリックして確定します。ファイルヘッダの "Add range comment" ボタンからも同じ
フローに入れます。

必要なもの:

- JDK 21
- `$PATH` 上の `git`
- JDK toolchain や Gradle dependencies がキャッシュされていない初回実行時は
  network access

リポジトリルートから backend を起動:

```sh
cd web
./gradlew :run
```

server は localhost の空き port に bind し、stdout に URL を出力します:

```text
SITATAME_WEB_URL=http://127.0.0.1:<port>
```

出力された URL をブラウザで開くと操作できます。

**別プロジェクトをレビューする場合** — `--repo` と必要に応じて `--base` を渡します:

```sh
# 別プロジェクトをレビュー
cd web && ./gradlew :run --args="--repo /path/to/other-project"

# カスタム base ref を指定
cd web && ./gradlew :run --args="--repo /path/to/project --base origin/develop"

# 環境変数を使う場合
SITATAME_REPO=/path/to/project SITATAME_BASE=origin/develop cd web && ./gradlew :run
```

`--repo` に指定するパスは git リポジトリのルート（`.git` を含むディレクトリ）である
必要があります。git リポジトリでないディレクトリを指定するとエラーメッセージを出力して
即座に終了します。解決優先順位: CLI フラグ > 環境変数 > カレントディレクトリ。

**fat jar として配布（対象マシンに Gradle 不要）:**

```sh
# Compose Wasm UI + Ktor server をまとめた単一 jar を生成
make web-jar
# 出力: web/build/libs/sitatame-web-<version>-fat.jar  (~20 MB)

# 任意のディレクトリで実行可能 — 必要なのは JDK 21 と git のみ
java -jar /path/to/sitatame-web-0.2.0-fat.jar --repo /path/to/other-project
java -jar /path/to/sitatame-web-0.2.0-fat.jar --repo /path/to/project --base origin/develop
```

fat jar には JVM ランタイム依存ライブラリとビルド済みの Wasm UI バンドルが含まれます。
stdout に `SITATAME_WEB_URL=http://127.0.0.1:<port>` が出力されるので、
ブラウザで開いてください。

UI 開発時のホットリロードが必要な場合は別 shell で Wasm frontend の dev server も
起動します:

```sh
cd web
./gradlew :wasmJsBrowserDevelopmentRun
```

build / smoke test:

```sh
cd web
./gradlew :jvmTest
./gradlew :wasmJsBrowserDistribution
```

Kotlin codec や storage compatibility test を変更する前に、Go 実装から共有
YAML fixture を再生成できます:

```sh
make web-fixtures
```

現時点の制約:

- production Wasm distribution は Ktor static resources にまだ自動連携されて
  いないため、local UI development では `:wasmJsBrowserDevelopmentRun` を使います。
- Compose for Web (CMP 1.7.x) の制約により Shift+click での range モード開始は
  未対応です。long-press を使ってください。
- コメント削除 (DELETE) と conflict 時の force overwrite は未実装です。
- WebSocket/SSE push はありません。TUI や IntelliJ Plugin による変更はブラウザを
  手動でリロードするか、次の write 操作時の 412 で初めて検知されます。

詳細な write 経路の仕様は
[`docs/web-ui-phase1-step2-spec.md`](docs/web-ui-phase1-step2-spec.md) を参照して
ください。

module layout、API routes、environment variables、既知の制約は
[`web/README.md`](web/README.md) にまとめています。

## IntelliJ Plugin

[`intellij/`](intellij/) の IntelliJ plugin は、sitatame review を IDE 内で
書く / 読むための experimental surface です。editor から line / range comment
を追加し、comment state を toggle し、`SitatameReview` tool window で一覧し、
draft を `reviews/` に promote し、AI-ready prompt を clipboard にコピーし、
plugin level の `SITATAME_HOME` override を設定できます。

必要なもの:

- JDK 21
- IntelliJ IDEA Community / Ultimate 2024.3 以降
- Android Studio 2024.3.x は動作想定ですが、現 CI matrix では未検証です。
- IntelliJ SDK や dependencies がキャッシュされていない初回実行時は network
  access

plugin zip を build:

```sh
cd intellij
./gradlew :buildPlugin
```

zip は次に出力されます:

```text
intellij/build/distributions/sitatame-intellij-0.1.0.zip
```

IDE では **Settings -> Plugins -> gear icon -> Install Plugin from Disk...**
から生成された zip を選択し、再起動してください。

plugin を読み込んだ sandbox IDE を起動:

```sh
cd intellij
./gradlew :runIde
```

plugin test:

```sh
cd intellij
./gradlew :test
```

install 後の主な entry point:

- editor context menu: `sitatame: Add Comment`
- shortcut: macOS は `Cmd+Shift+C`、それ以外は `Ctrl+Shift+C`
- editor context menu: `sitatame: Toggle Resolved`
- shortcut: macOS は `Cmd+Shift+R`、それ以外は `Ctrl+Shift+R`
- tool window: `SitatameReview`
- settings: **Settings -> Tools -> sitatame review**

plugin は CLI / Web UI と同じ storage shape を使います:

```text
$SITATAME_HOME/<project-slug>/<branch-slug>/review.md
```

詳細な feature list、storage notes、plugin 固有の制約は
[`intellij/README.md`](intellij/README.md) を参照してください。

## 使い方

git ワーキングツリー内で実行します:

```sh
sitatame                # base を自動検出（origin/HEAD, @{upstream}, main, …）
sitatame origin/main    # base を明示指定
sitatame --staged       # ステージ済みの変更をレビュー（index vs HEAD）
sitatame --working      # 未コミットの全変更をレビュー（worktree vs HEAD）
sitatame --new          # review.md が既に存在する場合は起動を拒否
sitatame --force-new    # review.md を review.md.bak にバックアップして新規開始
sitatame search TODO    # ~/.sitatame/<project-slug>/ を grep
```

キーバインド:

```
j / k       カーソル下 / 上
n / p       次 / 前のファイル
↑ ↓ ← →     矢印キーは k / j / p / n の alias
f           ファイルピッカーモーダル（任意のファイルへジャンプ）
wheel       diff をスクロール（Option/Fn 押下でテキスト選択）
r           範囲選択開始（j/k で拡張、Esc で解除）
c           カーソル位置にコメント（kind は選択 / 行種別から自動決定）
x           カーソル位置のコメントを resolved ↔ open でトグル（stale はスキップ）
Shift+R     review 全体コメント（front matter の review_comment を編集）
s           保存して exit 0 — ~/.sitatame/<project-slug>/<branch-slug>/review.md を
            アトミックに書き出し、stdout に SITATAME_REVIEW=<絶対パス> を 1 行出力
q           破棄して exit 1 — review.md は変更されない
?           ヘルプ表示の切り替え
Esc         モーダルを閉じる / 選択解除（トップレベルでは何もしない）

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
2. レビュアがレビューを終え `s`（save & exit）を押下。
3. `sitatame` は exit 0 で終了し、**stdout** に機械可読行を 1 行出力:

   ```
   SITATAME_REVIEW=/abs/path/to/.sitatame/<project-slug>/<branch-slug>/review.md
   ```

4. その path を capture します。ファイルは YAML front matter + Markdown 本文。
   front matter には `schema`、`branch`、`base.{ref,sha}`、`head.{ref,sha}`、
   `comments` リスト（各エントリに `kind: review|file|line|range`、`path`、
   `side`、必要に応じて `line` / `line_start` / `line_end`、staleness 判定用の
   blob ハッシュ、`body` を含む）が並びます。`sitatame` がモデル化していない
   キーは round-trip で温存されるため、エージェント側でスキーマを拡張しても
   次回保存時に失われません。全フィールドの定義・Side 決定ルール・state
   遷移・forward-compat 戦略は
   [docs/review-schema.md](docs/review-schema.md) にまとめています。
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

`q` で抜けた場合は exit 1 となり、`review.md` は変更されません（前回
セッションで保存した内容がそのまま残ります）。

同じブランチで再度 `sitatame` を起動すると既存の `review.md` が自動的に
読み込まれ、前回のコメントが TUI に復元されます。再度 `s` を押すと
同じファイルをアトミックに上書きします。

### 保存先

レビューはリポジトリツリーの外に書き出すため、プロジェクトごとに
ignore ルールを足す必要はありません。出力ルートは次の順で解決します:

1. `$SITATAME_HOME` が設定されていればそれを使用
2. 既定値として `~/.sitatame`
3. 上記が解決できない場合の最終フォールバックとして `$TMPDIR/sitatame`
   （stderr に警告を 1 行出力）

出力ルート配下では、リポジトリの checkout ごとに `<project-slug>/`
ディレクトリが作られます。slug は basename と絶対パスのハッシュから派生
するので、同じリポジトリの別 checkout（worktree など）も衝突しません。
その配下では `<branch-slug>/review.md` という形でブランチごとに 1 ファイルが
置かれます。

#### pre-#76 レイアウトからの移行

以前のバージョン（issue #76 以前）の `drafts/` / `reviews/` ディレクトリが
`~/.sitatame/<project-slug>/` 配下に残っている場合、初回起動時に
`MigrateLegacyLayout` が自動実行されます。旧ディレクトリは
`.legacy-<timestamp>/` に移動（データは削除されない）され、ブランチごとの
最新 `.md` ファイルが新 `<branch-slug>/review.md` にコピーされます。
移行サマリは stderr に出力されます。

旧版が残した `<repo>/.sitatame/` ディレクトリ（in-repo storage, pre-#38）が
存在する場合は起動時に stderr で 1 行通知を出しますが、自動移行や参照は
行いません。必要なデータをコピーしたら手動で削除してください。

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

- darwin バイナリの codesign / notarization（現状は未署名で配布）
- Homebrew tap / aqua / mise 連携
- delta パイプ統合によるシンタックスハイライト diff
- 左ペインツリー / side-by-side レイアウト

## 関連ドキュメント

- [`docs/tui-status.md`](docs/tui-status.md) — TUI メンテナンス方針 /
  キーバインド・機能棚卸し / TUI ・ Web UI ・ IntelliJ Plugin の機能対比表。
  TUI に対する feature request・bug fix の前にまず読む。
- [`docs/config.md`](docs/config.md) — リポジトリ単位の
  `<repo>/.sitatame/config.yaml` スキーマ。
- [`web/README.md`](web/README.md) — Web UI のスコープ / ビルド手順 /
  YAML round-trip Kill criteria。

---

English version: [README.md](README.md)
