---
name: sitatame-review-apply
description: sitatame が ~/.sitatame/<project-slug>/<branch-slug>/review.md に書き出したレビューを読み、open のコメントごとにコードを修正して resolved にマークする
---

# sitatame review apply

sitatame は TUI でレビューを記録し、Markdown + YAML フロントマターの 1 ファイルに固めて出力する。この skill は出力された review.md を読み込み、`state: open` のコメントに対応する修正を順次適用していき、適用が終わったコメントを `state: resolved` に書き換える役割を担う。

## 1. 対象ファイルの探し方

issue #76 以降、レビューファイルは **1-branch-1-file** レイアウトに移行した。探す場所は 1 つだけ:

```
~/.sitatame/<project-slug>/<branch-slug>/review.md
```

`$SITATAME_HOME` が設定されている場合は `~/.sitatame` の代わりにその値が使われる（解決順: `$SITATAME_HOME` → `~/.sitatame` → `<tmp>/sitatame`）。

最新レビューを選ぶ手順:

1. 上記パスに `review.md` が存在するか確認する（`stat` で十分）
2. 存在すれば、それが処理対象の 1 ファイルである。選択不要
3. ユーザーから具体的なファイル名 / `SITATAME_REVIEW=<abs>` の値が提示されている場合はそれを最優先する

### 旧 layout を見つけた場合

以下のパスを検出したら **migration 漏れ** なので処理を中断し、ユーザーに案内する:

- `~/.sitatame/<project-slug>/reviews/<branch-slug>/*.md`（pre-#76 の旧 reviews/ layout）
- `~/.sitatame/<project-slug>/drafts/<branch-slug>/*.md`（pre-#76 の旧 drafts/ layout）

案内文:
> "sitatame を一度起動して migration を完了させてください（旧 `reviews/` / `drafts/` layout が残っています）。"

`<repo-root>/.sitatame/` を検出した場合も同様: PR #42 以前の legacy です。同じ案内を出してください。

### Rescue file を検出した場合

`~/.sitatame/<project-slug>/<branch-slug>/review.md.rescue.*.json` を検出したら **apply せず abort** する:

> "Rescue file detected at `<path>`. This means the previous save failed. Please open the rescue file (JSON) and manually recover your comments before re-running this skill."

`review.md` と rescue file が共存している場合も rescue を優先して abort する。

### `.legacy-<YYYYMMDDTHHMMSS>/` ディレクトリ

`~/.sitatame/<project-slug>/.legacy-<ts>/` は migration が退避させた旧データ。走査対象外とする（無視）。

## 2. ファイル構造

人間向けの完全な schema 仕様は [`docs/review-schema.md`](../../../docs/review-schema.md) にある（フィールド定義・Side 決定ルール・state 遷移・Extras 契約まで網羅）。本 skill はそれを前提に AI 向けの処理手順だけを書く。schema が将来 bump された場合は、まず `docs/review-schema.md` の差分を読むこと。

```markdown
---
schema: 1
id: 01H...
created_at: 2026-06-11T...
branch: feature/foo
base:
  ref: main
  sha: abcd...
head:
  ref: feature/foo
  sha: 0123...
files:
  - path: internal/foo/bar.go
    blob_base: ...
    blob_head: ...
review_comment: |
  全体方針への一言
comments:
  - anchor_id: a-001
    kind: line          # review | file | line | range
    path: internal/foo/bar.go
    side: head          # head | base （省略時は head 扱い）
    blob: 0123abcd      # head/base 側のファイル blob（任意）
    line: 42            # kind=line のとき
    line_start: 40      # kind=range のとき
    line_end: 45        # kind=range のとき
    state: open         # open | resolved | stale
    body: |
      ここの早期 return を消したい
---

（本文。renderer がコメント一覧を再構成しているだけなので、
 フロントマターの comments[] が単一の真実情報源）
```

要点:

- `schema: 1` を前提に動く。違うバージョンが来たら停止してユーザーに確認する
- `comments[].kind` は `review` / `file` / `line` / `range` のいずれか
- `review` は anchor を持たない全体コメント。コード修正の対象にはならない
- `file` は `path` のみ、`line` は `path` + `line`、`range` は `path` + `line_start..line_end`
- `side: base` の場合は「修正対象は HEAD 側ではなく BASE 側の行」という意味なので、現行コードを base 側の行番号で探してはいけない（後述）
- 上記サンプルは sitatame 本体の Encode 出力順（anchor_id → kind → path → side → blob → line → line_start → line_end → rename_from → rename_to → similarity → state → body）に揃えている。保存し直すと未指定キーが落ちる点と、state が anchor 群の **後** に並ぶ点に注意（手書きの古いファイルでは state が anchor_id 直下にある可能性もあるが、書き戻すときは Encode 順を尊重するのが安全）

## 3. Pruning 契約

**`state: open` のコメントのみ処理対象** とする。

- `state: resolved`: 無視する（スキップ）
- `state: stale`: 無視する（anchor が壊れているため）

この契約により、一度 resolved にしたコメントを skill が誤って再処理することはない。

## 4. 適用の手順

各 `state: open` のコメントについて、以下を順に実行する:

1. `kind` と `anchor` を読み、対象範囲を特定する
   - `kind: line` → `path` の `line` 行をピンポイントで読む
   - `kind: range` → `path` の `line_start..line_end` を範囲として読む
   - `kind: file` → `path` 全体を対象にする
   - `kind: review` → コード修正の対象外。スキップ
2. `side` を確認する
   - `side: head`（既定）: その行番号は HEAD（現在のコード）の行番号として解釈してよい
   - `side: base`: BASE（修正前）の行番号。**削除行 (`-` で始まる diff 行) へのコメントはこの形で記録される。削除された行は HEAD には存在しないため、`path` の現在のファイルを `line` 番号で直接開いても対応行は見つからない。** `body` と `git diff` の文脈から「何が削除されたか / なぜコメントを残したか」を把握し、HEAD 側で対応する修正箇所を推定する。確信が持てないなら resolve しない
   - `side: base` で `blob` が記録されている場合: `git show <blob>` で削除前のファイルを取得し、`line` 行を確認できる
3. `body` に従って最小限の修正を組み立てる
4. **必ず diff の形でユーザーに提示し、承認を得てから** 適用する。勝手にコミットや push はしない
5. 適用後、当該コメントの YAML を `state: open` → `state: resolved` に書き換える（**フロントマター部分だけ**いじり、本文 Markdown は触らない）
6. 1 ファイル 1 コミットを目安に区切る（複数コメントが同一ファイルにあるならまとめて 1 コミットでよい）

## 5. resolve しない判断

以下のケースでは `state` を **触らない** か、別途 review_comment にメモを足す:

- anchor の行番号が見つからない（既にリファクタ済み等で対応行を特定できない）
- `body` に「これは意図的」「ignore」「無視」「あえてこうしている」等、修正不要を示すニュアンスがある
- 修正案に確信が持てない / テストが書けない
- `state: stale` のコメントは既に anchor が壊れているので resolve しない（オリジナルの reviewer が手当てするまで待つ）

resolve をスキップした場合は、ユーザーに「なぜスキップしたか」を 1 行で伝える。

## 6. YAML 書き換えの注意

フロントマターは `---` で囲まれた YAML ブロック。書き換える時は次を守る:

- `state: open` → `state: resolved` 以外の差分を入れない（インデント、コメント順、キー順を保つ）
- `comments` 全体を YAML 経由で再生成しない（`Extras` フィールドや未知のキーが落ちる可能性がある）。**該当行だけ書き換える**のが安全
- 末尾の Markdown 本文には触らない

### state 置換アルゴリズム（anchor_id ペアリング）

同一行に複数の `state: open` が並ぶ可能性があるため、sed 風の一括置換は禁止。次のアルゴリズムを必ず守る:

1. 対象コメントの `anchor_id` をユニーク文字列として grep し、その行位置 L を得る（例: `grep -n '^  - anchor_id: a-001$' <file>`）
2. L から下方向に最初に出てくる `state: open` の行 **だけ** を `state: resolved` に書き換える
3. 走査範囲は、次のコメントブロック先頭（次の `- anchor_id:` 行）または front matter 終端 `---` を超えないこと。超える前に `state` が見つからない場合は書き換えを諦め、ユーザーに「該当 anchor の state 行が見つからない」と報告する
4. `kind: review` のように line / range キーを持たないコメントでも、`state` キー自体は存在するので 2. のステップで拾える
5. 書き換え後、`grep -c 'state: open' <file>` の差分を確認し、想定どおり 1 件だけ減っていることを検証する（複数減っていたら誤爆 → revert）

インデントは 4 スペースが基本だが、ファイルに合わせて確認する。

## 7. ガードレール（再掲）

- 修正は **diff としてプレビュー** → ユーザー承認 → 適用、の順序を崩さない
- 行番号や anchor を見失ったら resolve しない
- 「意図的」と読める body は resolve しない
- `kind: review` / `state: stale` / `state: resolved` は触らない
- 1 ファイル単位でコミットを刻む（メッセージは「resolve sitatame:<anchor_id>」程度で十分）
- 修正後の検証として、関連するテストがあれば走らせる

## 8. キーバインド（参考）

TUI 終了時の挙動（`internal/tui/model.go`）:

| キー | 定数 | 挙動 |
| ---- | ---- | ---- |
| `s` | `QuitSave` | `review.md` に直接書き込んで終了（promote 概念なし） |
| `q` | `QuitDiscard` | **保存せず破棄**。`review.md` には触らない |
| top-level `Esc` | — | 何もしない（modal cancel のみ） |

draft / promote の概念は issue #76 で廃止済み。

## 9. 出力

最後に以下をまとめてユーザーへ報告する:

- 処理対象としたレビューファイル（絶対パス）
- resolved に書き換えたコメントの `anchor_id` 一覧
- スキップしたコメントの `anchor_id` と理由
- 走らせたテスト / lint コマンドと結果
