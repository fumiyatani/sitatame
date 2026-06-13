# Review YAML schema (v1)

`sitatame` がレビュー結果を書き出す Markdown ファイルの YAML フロント
マターと、その読み書き契約の仕様書です。対象読者は次の 3 種類です:

- `sitatame` 利用者で、保存物を直接開いて確認したい人
- `sitatame` の出力を消費する外部ツール / AI agent の実装者
- `sitatame` 本体に手を入れるコントリビューター

このドキュメントは現行スキーマ (`schema: 1`) を対象にしています。スキーマ
バージョンを上げるときは末尾の「将来の schema v2 bump 時のガイドライン」
を参照してください。

## 1. 概要

`sitatame` は git diff に対するレビューを **1 ファイル 1 レビュー** で
書き出します。ファイル形式は

```
YAML フロントマター (--- で囲む)
本文 Markdown (renderer の表示用、真の情報源ではない)
```

の 2 段構成です。フロントマターが構造化された **真の情報源** で、本文
Markdown はフロントマターから renderer が組み立てた人間用の表示で、
別ツールから読み戻す場合は本文ではなくフロントマターを参照します。

このフォーマットの最大の目的は、人間レビュアーが残したコメントを
**コーディング agent や別ツール (Web UI / MCP server / サードパーティ
CLI など) が画面スクレイプなしで消費できる** ようにすることです。
スキーマは

- 後方互換を強く意識する (`schema` フィールドで明示バージョン管理)
- 未知のキーは decode → encode で原形保存する (Extras / forward-compat)
- 1 つのレビューが「全体コメント・file 単位・行単位・範囲」の 4 粒度を
  同じ構造で扱える (`kind` フィールド)

という 3 点を中心に組まれています。Agent 連携が core value なので、
新しい reader が古いファイルを壊さないこと、新しい writer が古い
reader 用のフィールドを落とさないこと、が schema 全体の不変条件です。

## 2. ファイル配置と命名規則

レビューは `~/.sitatame/<project-slug>/{reviews,drafts}/<branch-slug>/<id>.md`
に書き出されます。出力ルート (`~/.sitatame` 部分) は次の順で解決します:

1. `$SITATAME_HOME` が設定されていればそれ
2. `~/.sitatame` (既定)
3. `$TMPDIR/sitatame` (最終フォールバック、stderr に警告)

`reviews/` は `s` キーで promote されたもの、`drafts/` は `q` キーで
途中保存されたもので、どちらも同じ schema です。

### project-slug

リポジトリ checkout ごとに 1 ディレクトリ。生成規則は

```
<safe-basename> + "__" + sha1(<repo absolute path>)[:8]
```

- `<safe-basename>` は repo root の basename を `[a-zA-Z0-9._-]` 以外を
  `_` に置換したもの (`internal/review/slug.go` の `safePrefix`)
- sha1 を 8 文字付けるのは、同名の別 checkout (worktree / 別 clone) が
  衝突しないようにするため
- ルート解決前に `EvalSymlinks` + `filepath.Abs` で正規化しているので
  `/tmp/x` と `/private/tmp/x` が同じ slug になる

### branch-slug

ブランチごとに 1 ディレクトリ。生成規則は

```
<safe-prefix> + "__" + sha1(<branch name as UTF-8>)[:8]
```

- `<safe-prefix>` はブランチ名の先頭 32 文字を `[a-zA-Z0-9._-]` 以外を
  `_` に置換したもの。安全な文字が 1 つも無い場合は `branch` に
  フォールバック
- 空ブランチ (`""`) は detached HEAD 用に `branch__da39a3ee` に倒れる
  (cmd/root.go では detached HEAD は `detached/<sha[:12]>` に正規化
  してからここに渡される)

### id

レビューファイル名 (の拡張子を除いた部分) です。生成規則は

```
yyyyMMddTHHmmss + "-" + slug(review_comment の先頭行)
```

- タイムスタンプは保存時の UTC、`20060102T150405` フォーマット
- slug は `review_comment` の先頭 1 行から `[a-zA-Z0-9._-]` 以外を `_`
  に置換し、最大 32 文字に切ったもの。前後の `_` / 先頭の `.` は除去。
  空または unsafe しか無いときは `review` にフォールバック
- 同じ (timestamp, slug) のファイルが既に存在する場合は `-1`, `-2`, ...
  サフィックスが追加される (`internal/review/store.go` の `GenerateID`)

最終的なフルパスの例:

```
~/.sitatame/sitatame__a1b2c3d4/reviews/feature_auth__9f8e7d6c/20260501T152300-fix_auth.md
```

## 3. ファイル形式: YAML frontmatter + Markdown body

```markdown
---
schema: 1
id: 20260501T152300-fix_auth
created_at: 2026-05-01T15:23:00+09:00
branch: feature/auth-refactor
base:
  ref: origin/main
  sha: 1a2b3c4d
head:
  ref: HEAD
  sha: abc123def
files:
  - path: src/auth.ts
    blob_base: 4e5f6a7b
    blob_head: 9c8d7e6f
    status: modified
review_comment: |
  全体方針への一言。
comments:
  - anchor_id: 1f3d9f2c-7c1e-4a3a-9c10-aa23bb45cc67
    kind: line
    path: src/auth.ts
    side: head
    blob: 9c8d7e6f
    line: 22
    state: open
    body: |
      早期 return にしたい。
---

# Review: feature/auth-refactor

（renderer が再構成する人間向けの表示。フロントマターの comments[]
 が単一の真実情報源で、本文は表示の都合で書き換わってもよい）
```

- フロントマターは行頭の `---` で開始、次の `---` 単独行で閉じる。
  BOM や先頭 whitespace は許容
- 閉じ `---` の直後に 1 行の空行を空けて本文 Markdown を続ける
- パーサ実装は `internal/review/codec.go` の `Decode` / `Encode`

## 4. フロントマター全フィールド

`internal/review/types.go` の `Review` 構造体を一次情報として参照して
ください。以下はその対応表です。

### Top-level

| フィールド       | 型                          | 必須 | 意味 |
|------------------|-----------------------------|------|------|
| `schema`         | int                         | 必須 | スキーマバージョン。現行は `1` 固定。reader は値を見て分岐する |
| `id`             | string                      | 必須 | ファイル名 (拡張子無し) と同一の id |
| `created_at`     | RFC 3339 timestamp          | 必須 | 最初に保存した時刻 (UTC)。再保存時は維持される |
| `branch`         | string                      | 必須 | レビュー対象のブランチ名。`branch-slug` の元 |
| `base`           | `{ref, sha}`                | 必須 | 比較元 (`git diff base..head` の base) |
| `head`           | `{ref, sha}`                | 必須 | 比較先 (`git diff base..head` の head) |
| `files`          | `[]FileMeta`                | 任意 | diff に含まれるファイルのメタ情報一覧 |
| `review_comment` | string (multi-line)         | 任意 | レビュー全体に対する一言コメント |
| `comments`       | `[]Comment`                 | 任意 | 各コメント。詳細は次節 |

### `base` / `head` (`Ref`)

| フィールド | 型     | 意味 |
|------------|--------|------|
| `ref`      | string | ブランチ名 / リモート名 / `HEAD` / `INDEX` / `WORKTREE` など |
| `sha`      | string | commit SHA。worktree モードでは空になり得る |

### `files[]` (`FileMeta`)

| フィールド     | 型     | 必須 | 意味 |
|----------------|--------|------|------|
| `path`         | string | 必須 | 表示用パス。rename 後は head 側のパス |
| `blob_base`    | string | 任意 | base 側 git blob SHA。新規追加ファイルでは空 |
| `blob_head`    | string | 任意 | head 側 git blob SHA。削除ファイルでは空 |
| `status`       | string | 任意 | `added` / `modified` / `deleted` / `renamed` など |
| `rename_from`  | string | 任意 | rename 元パス (`status: renamed` 時) |
| `rename_to`    | string | 任意 | rename 先パス (`status: renamed` 時) |
| `similarity`   | int    | 任意 | rename の git 検出類似度 (%) |

### `review_comment`

`kind: review` の Comment を Encode で書き戻したくないユースケース
(レビュー全体の感想だけ残す) 用に、Comment 配列とは独立に持つフィー
ルドです。複数行可。空文字なら省略。

### `comments[]`

各コメント。`Anchor` と Comment 固有フィールドが同じ map にフラット
化されています。

| フィールド    | 型     | 必須 | 意味 |
|---------------|--------|------|------|
| `anchor_id`   | string | 必須 | UUID。同一ファイル内で一意。state 書き換えのキーになる |
| `kind`        | enum   | 必須 | `review` / `file` / `line` / `range` のいずれか |
| `path`        | string | kind≠review | 対象ファイルパス |
| `side`        | enum   | 任意 | `head` / `base`。決定ロジックは 6 節 |
| `blob`        | string | 任意 | 対象ファイルの blob SHA (`side` 側) |
| `line`        | int    | kind=line | 行番号 (1-origin) |
| `line_start`  | int    | kind=range | 範囲開始行 |
| `line_end`    | int    | kind=range | 範囲終了行 |
| `rename_from` | string | 任意 | rename 元パス (anchor 単位の上書き) |
| `rename_to`   | string | 任意 | rename 先パス (同上) |
| `similarity`  | int    | 任意 | rename 類似度 (同上) |
| `state`       | enum   | 必須 | `open` / `resolved` / `stale` |
| `body`        | string | 必須 | コメント本文 (multi-line) |

## 5. Comment の Anchor

`kind` によって有効フィールドが変わります。

### `kind: review` (レビュー全体)

`anchor` を持たないレビュー全体コメント。`path` / `side` / `line` を
持たず、コード位置と紐づきません。`review_comment` フィールドとの
住み分けは「`kind: review` は state を持つ恒久コメント、`review_comment`
は state を持たない自由記述」です。

```yaml
- anchor_id: 3c9...
  kind: review
  state: open
  body: |
    全体方針は OK。argon2 移行は別 PR で。
```

### `kind: file` (ファイル単位)

`path` のみを anchor とするコメント。行情報なし。

```yaml
- anchor_id: 4d8...
  kind: file
  path: src/auth.ts
  side: head
  blob: 9c8d7e6f
  state: open
  body: |
    このファイル全体をリネームしたい。
```

### `kind: line` (1 行単位)

`path` + `side` + `line` を anchor とするコメント。

```yaml
- anchor_id: 5e7...
  kind: line
  path: src/auth.ts
  side: head
  blob: 9c8d7e6f
  line: 22
  state: open
  body: 早期 return にしたい。
```

### `kind: range` (連続行範囲)

`path` + `side` + `line_start` + `line_end` を anchor とするコメント。
`line_start <= line_end` (片端 1 行のみは `line` ではなく range で表現
することもある)。

```yaml
- anchor_id: 6f6...
  kind: range
  path: src/auth.ts
  side: head
  blob: 9c8d7e6f
  line_start: 10
  line_end: 14
  state: open
  body: |
    bcrypt ではなく argon2 を使ってほしい。
```

## 6. Side の決定ロジック

`side` は「行番号がどちらの revision を指しているか」を示します。
`internal/tui/modal.go` の `openCommentModal` / `lineSideForRow` /
`fileScopeSide` で決まります (issues #36 / #19 / #61 で整理された
最終形)。

### 行単位 (kind=line)

| diff 上の行種別 | `side` | `line` | `blob` |
|-----------------|--------|--------|--------|
| `-` 行 (削除)   | `base` | base 側の行番号 | `blob_base` |
| `+` 行 (追加)   | `head` | head 側の行番号 | `blob_head` |
| ` ` 行 (context)| `head` | head 側の行番号 | `blob_head` |

context 行はどちらの revision でも同じ内容ですが、レビュアーの視点に
合わせて `head` 側で記録します。

### ファイル単位 (kind=file)

| ファイル status | `side` | `blob`        |
|-----------------|--------|---------------|
| `deleted`       | `base` | `blob_base`   |
| `added`/`modified`/`renamed` | `head` | `blob_head` |

削除ファイルは `blob_head` が空なので、`side: head` で書くと validate
が stale 化してしまいます。

### 範囲 (kind=range)

- 全行が `-` のみ → `side: base`
- 1 つでも `+` 行を含む → `side: head` (mixed range)
- mixed range のときはステータスバーに `range spans add+delete —
  anchored to head side` という警告を出します

## 7. State と遷移

| state      | 意味 |
|------------|------|
| `open`     | コメント作成直後、または `x` で reopened されたもの |
| `resolved` | `x` で完了印を付けたもの (open ↔ resolved は双方向) |
| `stale`    | anchor がドリフトして元の行 / blob に解決できないもの |

### 遷移ルール

- `open` ⇄ `resolved`: `x` で双方向に切り替え (`internal/tui/model.go`
  の `toggleResolvedAtCursor`)
- `*` → `stale`: `internal/review/validate.go` の `Validate` が、diff
  上で blob も path も解決できないコメントを自動的に `stale` に格上げ
- `stale` → `*`: 自動復帰しない。anchor を手動で修正するか、コメント
  自体を消す
- `stale` のコメントは `x` トグルの対象外で、AI 経由の resolve でも
  触らないのが基本方針

stale 判定の詳細は `internal/review/validate.go` の `validateAnchor`
を参照してください。blob 一致 → path 一致 → 不一致 の優先順位で判定
されます。

## 8. Extras (未知キー保持) と forward-compat 戦略

`sitatame` の codec は **bit-exact round-trip** を契約として持ちます。
すなわち:

> Decode → Encode した結果は、元の YAML と (構造的に) 同じ内容を保つ。
> 未知のキーは無くならない。

実装は `internal/review/codec.go` の `Decode` / `Encode` で、各レベル
(top / `files[]` / `comments[]`) の構造体に `Extras map[string]*yaml.Node`
を持たせ、struct field に存在しない key をそこに退避することで実現
しています。Encode は struct を marshal した後で Extras を mapping に
merge し、決定的な順序 (struct 順 → extras はキー昇順) で書き戻します。

この契約のおかげで:

- **新しい reader が古いファイルを壊さない**: 古いファイルに無い
  フィールドは struct のゼロ値になるだけ
- **古い reader が新しいファイルを壊さない**: 新しい writer が増やした
  キーは Extras に拾われ、書き戻すときに失われない
- **複数の writer (Web UI / MCP server / 他 AI agent / 自作スクリプト)
  が同じファイルを編集しても壊れない**: 各 writer が自分の知らない
  キーを Extras で温存できる

新しい reader / writer を実装する側のルールは:

1. 知らないキーは捨てずに保持する (`Extras` 相当の機構を持つ)
2. 既存キーの **意味と必須/任意を変えない** (互換性破壊は schema bump)
3. 値の意味を拡張する場合は別キーを足す (例: 既存 `state` の意味を
   変えるのではなく `state_v2` を新設する) — 旧 reader は無視するだけ
   で済む

## 9. Examples

### 例 1: kind=review (全体コメントのみ)

```yaml
---
schema: 1
id: 20260501T100000-overall
created_at: 2026-05-01T10:00:00+09:00
branch: feature/x
base:
  ref: origin/main
  sha: 1111
head:
  ref: HEAD
  sha: 2222
comments:
  - anchor_id: 11111111-1111-1111-1111-111111111111
    kind: review
    state: open
    body: |
      全体方針は OK。argon2 移行は別 PR で進めたい。
---

# Review: feature/x
```

### 例 2: kind=file

```yaml
---
schema: 1
id: 20260501T100100-rename
created_at: 2026-05-01T10:01:00+09:00
branch: feature/x
base:
  ref: origin/main
  sha: 1111
head:
  ref: HEAD
  sha: 2222
files:
  - path: src/auth.ts
    blob_base: aaaa
    blob_head: bbbb
    status: renamed
    rename_from: src/auth_legacy.ts
    rename_to: src/auth.ts
    similarity: 92
comments:
  - anchor_id: 22222222-2222-2222-2222-222222222222
    kind: file
    path: src/auth.ts
    side: head
    blob: bbbb
    state: open
    body: |
      このファイル全体を別モジュールに移したい。
---

# Review: feature/x
```

### 例 3: kind=line (削除行 = side base)

```yaml
---
schema: 1
id: 20260501T100200-deleted-line
created_at: 2026-05-01T10:02:00+09:00
branch: feature/x
base:
  ref: origin/main
  sha: 1111
head:
  ref: HEAD
  sha: 2222
files:
  - path: src/auth.ts
    blob_base: aaaa
    blob_head: bbbb
    status: modified
comments:
  - anchor_id: 33333333-3333-3333-3333-333333333333
    kind: line
    path: src/auth.ts
    side: base
    blob: aaaa
    line: 30
    state: resolved
    body: |
      ここで消した bcrypt 経路、移行ガイドにリンクを足したい。
---

# Review: feature/x
```

### 例 4: kind=range + Extras (forward-compat デモ)

未知キー (`labels`, `comments[].custom_meta`) を含むサンプル。Decode →
Encode しても落ちずに温存されます。

```yaml
---
schema: 1
id: 20260501T100300-range-extras
created_at: 2026-05-01T10:03:00+09:00
branch: feature/x
base:
  ref: origin/main
  sha: 1111
head:
  ref: HEAD
  sha: 2222
labels:                       # ← 未知キー (Extras に温存される)
  - security
  - refactor
files:
  - path: src/auth.ts
    blob_base: aaaa
    blob_head: bbbb
    status: modified
comments:
  - anchor_id: 44444444-4444-4444-4444-444444444444
    kind: range
    path: src/auth.ts
    side: head
    blob: bbbb
    line_start: 10
    line_end: 14
    state: open
    body: |
      bcrypt ではなく argon2 を使ってほしい。
    custom_meta:              # ← 未知キー (Extras に温存される)
      tag: design
      reviewer: ext-bot
---

# Review: feature/x
```

`labels` と `custom_meta` は `sitatame` 本体の struct には無いキーです
が、`Extras` 経由で round-trip 保存されます。`internal/review/codec_test.go`
の `TestEncode_PreservesUnknownTopKey` /
`TestEncode_PreservesUnknownFileAndCommentKey` がこの契約を担保して
います。

## 10. 将来の schema v2 bump 時のガイドライン

スキーマバージョンを上げる (`schema: 2`) のは次のような場合のみです:

- 既存フィールドの **意味を変える**
- 既存フィールドの **型を変える**
- 既存フィールドを **削除する**

単なるフィールド追加は v1 のまま行ない、旧 reader は Extras で温存
すれば壊れません。

### v1 ファイルとの互換性

- v2 を実装するときは、本体 reader が `schema` を見て v1 / v2 を
  自動判別できるようにする
- 既存 v1 ファイルを保存し直す場合、ユーザー操作ではバージョンを
  勝手に上げない (誤って古い reader が読めなくなるのを避ける)
- v1 → v2 の migration コマンドを別途用意し、明示実行で v2 に
  上書きする

### 責任分担

- **sitatame 本体**: `schema: 1` の reader を当面残す。`schema: 2` を
  扱うときは codec を切り替える。migration コマンド (もしあれば) を
  提供する
- **外部 AI tool / Web UI / MCP server**: 自分が知っている schema 以外
  は touch しないか、Extras 機構を持って互換性を維持する
- **手動編集**: 末尾の本文 Markdown は触ってよい。フロントマターは
  該当行だけ書き換える運用 (`.claude/skills/sitatame-review-apply/SKILL.md`
  参照)

### Docs の更新

- v2 を投入するときは本ドキュメントを丸ごとリネーム (例:
  `docs/review-schema-v1.md`) して履歴を残し、新しい `docs/review-schema.md`
  を v2 として作る
- README からのリンクは「現行版を `docs/review-schema.md` が指す」
  運用を維持する

## 関連

- 真の情報源: `internal/review/types.go` (struct 定義)
- Codec 実装と round-trip 契約: `internal/review/codec.go`
- Stale 判定: `internal/review/validate.go`
- Side 決定: `internal/tui/modal.go` (`openCommentModal` / `lineSideForRow`
  / `fileScopeSide`)
- AI agent 向け処理手順: `.claude/skills/sitatame-review-apply/SKILL.md`
