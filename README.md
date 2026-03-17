# gw ヘルパースクリプト・Hooks

## Hooks

`gw` コマンドは各リポジトリの `.gw/hooks/` に実行可能ファイルを置くことで、ライフサイクルの特定タイミングに処理を挟める。

命名規則: `{event}-{subcommand}`

| Hook | タイミング | 引数 |
|---|---|---|
| `post-create` | `gw create` でworktree作成後 | `$1`: worktree_path |
| `post-review` | `gw review` でworktree新規作成後 | `$1`: worktree_path, `$2`: pr_number, `$3`: pr_url |
| `pre-remove` | `gw remove` でworktree削除前 | `$1`: worktree_path |

### 注意事項

- 既存worktreeへの移動時（`gw review <PR番号>` で既にworktreeが存在する場合）は hook は呼ばれない
- hookが非ゼロで終了した場合、`post-create` / `pre-remove` はエラー扱いになる
- `post-review` のエラーは worktree 作成後なので強制はしない

### hook の配置例

```zsh
# .gw/hooks/post-review
#!/usr/bin/env zsh
~/dotfiles/gw/gw-post-review "$@"
```

## ヘルパースクリプト

### `gw-post-review`

Claude Code をレビュー専用（plan モード）で起動するヘルパー。`.gw/hooks/post-review` から呼び出す。

**引数**: `<worktree_path> <pr_number> <pr_url>`

- `claude` コマンドが存在しない場合はスキップ
- `--permission-mode plan` で起動（コード編集不可）
- `--append-system-prompt` でレビュー専用の指示をシステムプロンプトに追加
- 初期プロンプトとして PR番号・URLをClaude Codeに渡す

### `gw-claude-trust`

ディレクトリを Claude Code の信頼済みとして `~/.claude.json` に登録する。

**引数**: `<directory>`

### `gw-sync-edit`

`.gw/config` をfzfで対話的に編集する。

**引数**: なし（カレントディレクトリのリポジトリを対象）
