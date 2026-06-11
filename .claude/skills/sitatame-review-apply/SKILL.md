---
name: sitatame-review-apply
description: sitatame が ~/.sitatame/<project-slug>/reviews/ に書き出したレビューを読み、open のコメントごとにコードを修正して resolved にマークする
---

# sitatame review apply

sitatame は TUI でレビューを記録し、Markdown + YAML フロントマターの 1 ファイルに固めて出力する。この skill は出力された最新レビューを読み込み、`state: open` のコメントに対応する修正を順次適用していき、適用が終わったコメントを `state: resolved` に書き換える役割を担う。

## 1. 対象ファイルの探し方

レビューファイルは 2 種類のレイアウトのどちらかに置かれている可能性がある。同一スキーマなのでどちらでも同じ手順で扱える。

- 新レイアウト（推奨）: `~/.sitatame/<project-slug>/reviews/<branch-slug>/<id>.md`
- 旧レイアウト: `<repo-root>/.sitatame/reviews/<branch-slug>/<id>.md`

両方が存在する場合は、ユーザーに「どちらを処理するか」を確認する。指定が無ければ更新時刻が新しい方を採用する。

最新レビュー 1 件を選ぶ手順:

1. 上記 2 つのパスを両方探索する（`ls` などで存在チェック）
2. 候補ファイルの mtime を比較し、最新の 1 件を採用する
3. ユーザーから具体的なファイル名 / `SITATAME_REVIEW=<abs>` の値が提示されている場合はそれを優先する

## 2. ファイル構造

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

## 3. 適用の手順

各 `state: open` のコメントについて、以下を順に実行する:

1. `kind` と `anchor` を読み、対象範囲を特定する
   - `kind: line` → `path` の `line` 行をピンポイントで読む
   - `kind: range` → `path` の `line_start..line_end` を範囲として読む
   - `kind: file` → `path` 全体を対象にする
   - `kind: review` → コード修正の対象外。スキップ
2. `side` を確認する
   - `side: head`（既定）: その行番号は HEAD（現在のコード）の行番号として解釈してよい
   - `side: base`: BASE（修正前）の行番号。現在のコードでは行が動いている可能性があるため、`body` と周辺コンテキストから対応箇所を推定する。確信が持てないなら resolve しない
3. `body` に従って最小限の修正を組み立てる
4. **必ず diff の形でユーザーに提示し、承認を得てから** 適用する。勝手にコミットや push はしない
5. 適用後、当該コメントの YAML を `state: open` → `state: resolved` に書き換える（**フロントマター部分だけ**いじり、本文 Markdown は触らない）
6. 1 ファイル 1 コミットを目安に区切る（複数コメントが同一ファイルにあるならまとめて 1 コミットでよい）

## 4. resolve しない判断

以下のケースでは `state` を **触らない** か、別途 review_comment にメモを足す:

- anchor の行番号が見つからない（既にリファクタ済み等で対応行を特定できない）
- `body` に「これは意図的」「ignore」「無視」「あえてこうしている」等、修正不要を示すニュアンスがある
- 修正案に確信が持てない / テストが書けない
- `state: stale` のコメントは既に anchor が壊れているので resolve しない（オリジナルの reviewer が手当てするまで待つ）

resolve をスキップした場合は、ユーザーに「なぜスキップしたか」を 1 行で伝える。

## 5. YAML 書き換えの注意

フロントマターは `---` で囲まれた YAML ブロック。書き換える時は次を守る:

- `state: open` → `state: resolved` 以外の差分を入れない（インデント、コメント順、キー順を保つ）
- `comments` 全体を YAML 経由で再生成しない（`Extras` フィールドや未知のキーが落ちる可能性がある）。**該当行だけ書き換える**のが安全
- 末尾の Markdown 本文には触らない

具体的には、対象コメントブロックの中で `    state: open` を `    state: resolved` に置換する。インデントは 4 スペースが基本だが、ファイルに合わせて確認する。

## 6. ガードレール（再掲）

- 修正は **diff としてプレビュー** → ユーザー承認 → 適用、の順序を崩さない
- 行番号や anchor を見失ったら resolve しない
- 「意図的」と読める body は resolve しない
- `kind: review` / `state: stale` は触らない
- 1 ファイル単位でコミットを刻む（メッセージは「resolve sitatame:<anchor_id>」程度で十分）
- 修正後の検証として、関連するテストがあれば走らせる

## 7. 出力

最後に以下をまとめてユーザーへ報告する:

- 処理対象としたレビューファイル（絶対パス）
- resolved に書き換えたコメントの `anchor_id` 一覧
- スキップしたコメントの `anchor_id` と理由
- 走らせたテスト / lint コマンドと結果
