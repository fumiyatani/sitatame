# sitatame – Claude Code working notes

## Git identity

すべてのコミットは、リポジトリオーナーの GitHub アカウントで行うこと。

- author / committer ともに以下を使用する:
    - 名前: `Fumiya Tani`
    - メール: `36175109+fumiyatani@users.noreply.github.com`
- `git config` は触らない。コミットごとに `--author=...` と
  `GIT_COMMITTER_NAME` / `GIT_COMMITTER_EMAIL` を指定する形で対応する。
- 既に Claude 名義でコミットしてしまった場合は、`--amend` で書き換えて
  `--force-with-lease` で push し直す。
