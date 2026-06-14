# sitatame Claude Code Subagents

このディレクトリは `.claude/agents/*.md` 形式の Claude Code カスタム subagent 定義群。

## 配置と発見

- Claude Code は `.claude/agents/<name>.md` を自動的に subagent として読み込む。
- 呼び出し側で `Agent` ツール / typeahead で参照できる。
- 公式仕様: https://code.claude.com/docs/en/sub-agents

## 命名と役割の俯瞰

「tools 種別」列の意味:
- **codebase write**: コードベース・docs に Edit / Write してよい
- **analysis + tmp write**: コードベースは触らない。Write は **成果物パス `tmp/agents/<name>/` 配下のみ** (body で制約)

| name | model | effort | tools 種別 | 用途 |
|---|---|---|---|---|
| `issue-perspective-analyst` | opus | xhigh | analysis + tmp write | UX / QA いずれかの視点で issue 群を分析（呼び出し時に `mode: ux` / `mode: qa` を指定） |
| `product-roadmap-analyst` | opus | xhigh | analysis + tmp write | PDM 視点で 価値判断・優先度・既存資産活用・廃止判定・既実装かの照合まで担う |
| `architecture-advisor` | opus | high | analysis + tmp write + WebFetch | 統合方式・技術選定・配布形態を複数案比較 |
| `adversarial-pr-reviewer` | opus | xhigh | analysis + tmp write | 罠・セキュリティ・Threading・API 誤用を敵対的に洗う |
| `pr-fix-applier` | sonnet | high | codebase write + Bash | PR review の指摘を worktree 内で実装し push（最頻） |
| `issue-implementer` | sonnet | high | codebase write + Bash | Issue を worktree 内で新規実装 |
| `branch-conflict-resolver` | sonnet | medium | codebase write + Bash | main との conflict を merge ベースで解消 |
| `docs-updater` | sonnet | medium | codebase write + Bash(read) | docs / README / parity table を実コード照合付きで更新 |
| `pr-description-writer` | sonnet | medium | analysis + tmp write + gh | PR description / release note を起こす |
| `ci-failure-analyzer` | sonnet | high | analysis + tmp write + gh | CI 失敗ログを分析して根本原因と最小修正案を返す |

これに加えて、グローバル（`~/.claude/agents/`）の `silent-failure-hunter` / `pr-test-analyzer` / `comment-analyzer` を併用する。

## 設計原則

1. **頻度に応じてエージェントを切る**。1 セッションで 50+ 回呼ぶ作業ほど agent body に定型を寄せる価値が大きい。
2. **読み書きの権限を `tools` で必ず絞る**。`analysis + tmp write` の agent では body 内で「Write は `tmp/agents/<name>/` 配下のみ」と明記する (frontmatter `tools` では path 制限できないため、行動規約として書く)。
3. **共通 Output Contract**（後述）を各 agent 末尾の報告フォーマットとして強制する。
4. **固有情報を含めない**: ユーザー名・絶対パス・GitHub アカウント名を agent 定義に書かない。worktree 運用や `GOCACHE` 規約のような **リポジトリ流儀** は OK。
5. **手動 worktree 運用が前提**: sitatame の実装系作業は `.claude/worktrees/<branch>/` 配下で行う。`isolation: worktree` は使わず、呼び出し元が事前に worktree を用意する。
6. **`effort` フィールド**は公式仕様（`low/medium/high/xhigh/max`、モデル依存）。本ディレクトリでは
   - 思考重要（設計・PDM・敵対レビュー）→ `opus` + `xhigh`
   - 機械的な実装・整形 → `sonnet` + `medium`〜`high`

## 思考の深さ vs 出力の長さ — 原則

サブエージェントは独立 context で動く。**思考の深さ・調査範囲は事前に絞らない**。「最大 N 件」「N 字以内」のような cap は、それを **探索段階** にかけると思考そのものを cap に合わせて縮める副作用がある。整理段階で読みやすさのために絞るのは可。

文言ルール:
- 「**探索段階の cap**」(`候補を 3-4 件にまとめる` など) は書かない。代わりに `深く検討し、最終整理段階で N 件に集約する` のように、探索フェーズと整理フェーズを分けて書く
- 「**最終整理段階の cap**」(`Top 3 を提示` `bullet 1-3 個` など) は OK。ただし「残りは tmp 成果物参照」と必ずセットで明記する

## 成果物書き出しプロトコル

### パス正規形

```
tmp/agents/<agent-name>/<YYYYMMDDTHHMMSS>-<topic-slug>.md
```

- `<agent-name>`: agent の `name` (例: `architecture-advisor`)
- `<YYYYMMDDTHHMMSS>`: UTC 秒精度 (例: `20260614T093015`)
- `<topic-slug>`: 半角英数 + ハイフン (例: `web-ui-integration`)、特定の対象がある場合は `pr80` `issue42` を含める
- 同一秒衝突対策として、衝突を検知したら `-<3 文字 random>` を末尾に追加

例: `tmp/agents/adversarial-pr-reviewer/20260614T093015-pr80-threading.md`

### 適用範囲と書き出し対象

`analysis + tmp write` の 6 agent (architecture-advisor / product-roadmap-analyst / issue-perspective-analyst / adversarial-pr-reviewer / ci-failure-analyzer / pr-description-writer) は **必ず** 成果物を書く。

書き出すべき内容:
- 複数案の詳細比較表 / フルマトリクス / 横断分析の全件
- 根拠 grep の代表ヒット (`path:line` を全件)
- WebFetch / Bash 出力の必要部分
- 検出 P0-P3 のフルリスト (整理してメインに返すのは P0/P1 のみ + P2/P3 の代表)
- 採用しなかった代替案とその理由 (「思考した跡」を残す)

実装系 4 agent (pr-fix-applier / issue-implementer / branch-conflict-resolver / docs-updater) は **任意**。設計検討メモを `tmp/agents/<name>/...` に書ける。

### Write 権限の境界

`analysis + tmp write` agent の Write は **`tmp/agents/<name>/` 配下のみ**。他 path への Write は body の禁則で明示し、もし agent が逸脱したら呼び出し元側で post-check して revert する。`tools` frontmatter では path 制限できない仕様上の制約。

### 失敗時 partial artifact ルール

agent が完了に至らない (maxTurns 到達 / 前提不足で停止 / Bash 失敗が積み重なる) 場合も、**partial 成果物を書き残す**:

```
tmp/agents/<name>/<timestamp>-<topic>-PARTIAL.md
```

partial 成果物の必須セクション:
- `## 停止理由` (前提不足 / maxTurns / 外部 fail)
- `## ここまでの調査範囲`
- `## 未検証範囲`
- `## 次に着手すべき箇所` (再開時の最初の手)

メインへの最終発話には `(partial: ...)` と明示する。

## 呼び出し元責務 (= メイン Claude の動線)

agent からの返答を受けたら、メイン Claude は以下に従う:

1. **成果物パスを受けたら、必要時のみ Read する**。即時には読まない (context 圧迫回避)。
2. メイン側の最終回答に成果物の **全文を再貼付しない**。引用するなら関連箇所のみ + パス。
3. 古い成果物 (前のセッション分) を参照する時は **timestamp を見る**。複数同名 topic がある場合は最新を採る。
4. 失敗時の `PARTIAL.md` を受け取ったら、ユーザーに状況を共有してから次手を判断する。

## 共通 Output Contract（全 agent が踏襲）

最終発話を以下 4 セクションで返す。**長さは絞らない** が、メインに必要なのは「意思決定に必要な高信号情報」だけ。網羅・代替案詳細・低重大度は tmp 成果物に送る。

### メインへ残すもの vs tmp へ送るもの

| メインに残す (= 呼び出し元の判断を変えうる) | tmp へ送る (= 詳細・網羅・補足) |
|---|---|
| 結論 (推奨方式 / マージブロック判定 / 修正案推奨 など) | 網羅表 (全 issue 評価 / 全観点マトリクス) |
| 自信度 / 却下案を採るべき条件 | 引用全文 (PR body / log / 公式 docs) |
| ブロッカー (P0/P1 検出、致命的な前提崩れ) | 低重大度 (P2/P3 全件、stylistic な指摘) |
| 次手 (推奨アクション 1-3 件) | 代替案の詳細根拠 |
| 主要な path:line 根拠 (代表 1-3 件) | 全 path:line リスト |

### フォーマット

```
## 結論
何が起きたか / 推奨アクション / 自信度。圧縮するために結論を歪めない。

## 実施内容
- codebase write agent: 変更したファイルと意図
- analysis + tmp write agent: 調査した範囲（主要ファイル・URL）
成果物: tmp/agents/<name>/<timestamp>-<topic>.md（または PARTIAL）

## 検証
- codebase write agent: `command / result / 未検証理由` の三列
- analysis + tmp write agent: 「照合方法 / 根拠 path:line / 取得日 (Web 参照時)」

## 残リスク
追加で見るべき箇所、未解決の前提、後続作業候補
```

固有項目はこの 4 セクションの末尾に `## 追加` として追記する。

## 共通禁則（全 agent body に展開）

1. メインへの最終発話にファイル全文・Bash ログ全文を貼らない。長文は `tmp/agents/<name>/...` に書いてパスを返す。
2. 根拠は `path:line` を必ず添える（省略しない）。
3. 既知情報の再説明禁止。呼び出し元プロンプトに書かれていることは要約しない。
4. 関係ない探索禁止。`node_modules/`, `build/`, `dist/`, `.gocache/`, `.gomodcache/`, `.git/`, generated files への探索は明示指示がない限り行わない。
5. 推測で断定しない。特に API / platform / CI / permission / hook / isolation 系は一次情報か実測を求める。
6. 編集系 agent は「編集前に対象ファイルと変更方針を確定」してから書く。
7. **探索段階の cap は使わない**。「思考を絞らず広げる → 最終整理段階で見やすく集約」の二段階で書く。
8. 不明点は重要度順 (ブロッカー優先) に並べる。同じ判断に収束するものは束ねる。低重要度は成果物側に。
9. 失敗・前提不足で完了に至らない時は `PARTIAL.md` を書き残す (上記「失敗時 partial artifact ルール」参照)。
10. 「念のため順番に」は禁則 (`~/.claude/rules/parallel-execution.md` 参照)。独立な調査・読み込み・subagent 起動は並列で。

## 呼び出し例

```
Agent({
  subagent_type: "pr-fix-applier",
  description: "PR #80 P0 反映",
  prompt: """
  PR #80 worktree: .claude/worktrees/issue-NN-foo
  指摘:
  - [P0] internal/foo/bar.go:42 で nil チェックが抜けて panic 経路ができている。
    修正方針: errors.Is(err, fs.ErrNotExist) で early return。
  """
})
```

呼び出し元の責務は「対象 PR / worktree / 指摘内容」のみ。worktree 制約・GOCACHE・push 規約・報告フォーマットは agent body 側に固定済み。

## 追加・更新ポリシー

- 同パターンを 3 回以上呼んだら、専用 agent 化を検討する。
- 既存 agent の body 修正で済むなら、新規追加せず修正する。
- `silent-failure-hunter` / `pr-test-analyzer` / `comment-analyzer` はグローバルに置く（プロジェクト横断で同一観点なので）。
