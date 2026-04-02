# gw シェルスクリプト → Go 移植計画

## Context

zsh で実装されたGit Worktree管理ツール「gw」（`rc/git_gw` 約1145行 + `bin/` スクリプト5本）をGoに移植する。

**設計思想**: git の plumbing/porcelain パターンを採用。
- **`gw`（Go バイナリ）** = plumbing層。常にJSON出力
- **`gw`（シェル関数）** = porcelain層。`command gw` でバイナリを呼び、JSONを整形して表示 + cd 実行
- **`gw __fmt`（Go）** = 組み込みJSONフォーマッター。jq等の外部依存を排除

**cd制約**: 子プロセスは親シェルのカレントディレクトリを変更できない（UNIXの基本制約）。そのためシェル関数が必須。

## アーキテクチャ

### JSON出力プロトコル

`gw` バイナリは常にJSONを返す。全コマンド共通フィールド + コマンド固有フィールド。

```json
// 共通フィールド
{
  "messages": ["Created worktree for task-123"],
  "cd": "/path/to/worktree"
}

// list のコマンド固有フィールド
{
  "messages": ["task-1  (2d)  feature/auth", "task-2  (5d)"],
  "worktrees": [
    {"label": "task-1", "branch": "feature/auth", "age": "2d"},
    {"label": "task-2", "age": "5d"}
  ]
}

// エラー時（exit code は非ゼロ）
{
  "messages": ["Error: task name is required"]
}
```

**重要**: `messages` には常に整形済みテキストが入る。ラッパーは `messages` だけ見れば表示でき、コマンド固有フィールドの詳細を知らなくてよい。

### シェルラッパー（gw関数）

`gw init zsh` が生成する。`command gw` でzsh関数をバイパスしてバイナリを直接呼び出す。

```zsh
function gw() {
    local result
    result=$(command gw "$@")
    local exit_code=$?
    echo "$result" | command gw __fmt messages
    local cd_path
    cd_path=$(echo "$result" | command gw __fmt cd)
    if [[ $exit_code -eq 0 ]] && [[ -n "$cd_path" ]]; then
        export GW_PREVIOUS_WORKTREE="$(git rev-parse --show-toplevel 2>/dev/null)"
        cd "$cd_path"
    fi
    return $exit_code
}

_gw() {
    local -a subcommands
    subcommands=(
        'create:Create new worktree'
        'list:List worktrees'
        'remove:Remove worktree'
        'switch:Switch to worktree'
        'describe:Show/set metadata'
        'review:Create PR review worktree'
        'prune:Bulk remove worktrees'
        'archive:Archive worktree'
        'activate:Activate archived worktree'
        'sync:Sync files to worktree'
        'sync-edit:Edit sync config'
        'init:Generate shell setup script'
    )
    _arguments -C '1: :->command' '*: :->args'
    case $state in
        command) _describe -t commands 'gw subcommand' subcommands ;;
        args)
            case $words[2] in
                switch|sw|remove|rm|describe|desc)
                    local -a tasks
                    tasks=(${(f)"$(command gw __completion tasks)"})
                    _describe -V -t tasks 'worktree' tasks ;;
                archive)
                    local -a tasks
                    tasks=(${(f)"$(command gw __completion tasks)"})
                    _describe -V -t tasks 'worktree' tasks ;;
                activate)
                    local -a tasks
                    tasks=(${(f)"$(command gw __completion archived)"})
                    _describe -V -t tasks 'worktree' tasks ;;
            esac ;;
    esac
}
compdef _gw gw
```

### セットアップ

```zsh
# インストール
go install github.com/kawaken/gw/cmd/gw@latest

# .zshrc にこれだけ
eval "$(gw init zsh)"
```

`gw init zsh` は例外的に生のシェルスクリプトを出力する（JSON-always の唯一の例外）。gw関数 + _gw補完関数 + compdef を一括出力。

### __fmt サブコマンド

stdinからJSONを読み、指定フィールドを抽出して出力。ヘルプには非表示（`__` prefix）。

```bash
echo '{"messages":["hello"],"cd":"/path"}' | gw __fmt messages
# → hello

echo '{"messages":["hello"],"cd":"/path"}' | gw __fmt cd
# → /path
```

- `messages`: 配列の各要素を改行区切りで出力
- `cd`: 文字列をそのまま出力
- フィールドが存在しない/空の場合は何も出力しない

### TUI描画

prune, sync-edit のTUIは `/dev/tty` に直接描画（bubbletea の標準動作）。stdout がキャプチャされていても問題なし。

### __completion サブコマンド

補完候補を `label:description` 形式で出力。ヘルプ非表示（`__` prefix）。

```bash
gw __completion tasks     # 全worktree（通常 + archived以外）
gw __completion archived  # アーカイブ済みworktreeのみ
```

zsh補完関数内から `command gw __completion tasks` で呼び出す。

### .gw/config (TOML)

```toml
[worktree]
dir = "../myapp-wt"          # worktreeを置くベースディレクトリ（デフォルト: ../{repo}-wt）
max_name_length = 20         # task名の最大文字数（デフォルト: 20、超過時は警告 + 短縮）

[sync]
copy = [".env", ".tool-versions"]
symlink = [".envrc"]
```

- `pelletier/go-toml/v2` を使用（コメント保持のラウンドトリップ書き込みに対応）
- 旧フォーマット（`copy:`, `symlink:` プレフィックス形式）との互換性なし

### hookインターフェース

hookスクリプトは環境変数のみで情報を受け取る（引数なし）。

| 変数 | 説明 | 対象 |
|---|---|---|
| `GW_SUBCOMMAND` | 呼び出しサブコマンド | 全hook |
| `GW_TASK_NAME` | worktree名 | 全hook |
| `GW_WORKTREE_PATH` | worktreeの絶対パス | 全hook |
| `GW_MAIN_REPO` | mainリポジトリの絶対パス | 全hook |
| `GW_DESCRIPTION` | --purpose で設定した説明 | 全hook |
| `GW_PR_NUMBER` | PR番号 | review |
| `GW_PR_URL` | PR URL | review |
| `GW_PR_TITLE` | PRタイトル | review |

## ライブラリ選定

| 用途 | 選定 | 理由 |
|---|---|---|
| CLI | 標準ライブラリ（flag + switch） | サブコマンド10個・引数単純。軽量 |
| Git操作 | `os/exec` で git CLI 呼び出し | worktree操作は go-git で未サポート |
| TUI | `charmbracelet/bubbletea` | prune・sync-edit 両方に統一 |
| TOML | `pelletier/go-toml/v2` | コメント保持ラウンドトリップ |
| JSON | 標準 `encoding/json` | |

## サブコマンド一覧

| コマンド | エイリアス | cd | 概要 |
|---|---|---|---|
| create | cr | ✅ | worktree作成。既存の場合はcdのみ・hookなし |
| list | ls | - | worktree一覧 |
| remove | rm | ✅ | worktree削除（現在のwt削除時にmainへ）+ branch削除（デフォルトbranch以外） |
| switch | sw | ✅ | worktree切替（`-` で前のwtに戻る） |
| describe | desc | - | メタデータ表示/設定（main worktreeでも動作、メタデータなければ空） |
| review | pr | ✅ | PR用worktree作成。既存の場合はcdのみ・hookなし |
| prune | - | ✅ | TUIで選択して一括削除。デフォルトは全件表示、`--older N` でフィルター |
| archive | - | ✅ | アーカイブ（現在のwt時にmainへ） |
| activate | - | - | アーカイブ解除 |
| sync | - | - | ファイル同期 |
| sync-edit | - | - | TUIでconfig編集 |
| init | - | - | シェル初期化スクリプト出力（唯一の非JSON出力） |
| __fmt | - | - | JSONフィールド抽出（内部用、ヘルプ非表示） |
| __completion | - | - | 補完候補出力（内部用、ヘルプ非表示） |

エイリアスはGo側で処理。`switch -` は環境変数 `GW_PREVIOUS_WORKTREE` を読む。

### create の詳細仕様

- ブランチ名デフォルト: task名と同じ（`-b` オプションで上書き可）
- `--purpose` オプション: メタデータのdescriptionキーに保存（省略可）
- 既存worktree: cdのみ返す・hookは実行しない（冪等）
- ディレクトリ: `{base_dir}/{task_name}` （base_dirは.gw/configで設定可）

### remove の詳細仕様

- デフォルトbranchは削除しない（`git symbolic-ref refs/remotes/origin/HEAD` で取得、失敗時は `main`/`master` フォールバック）
- worktreeと同名のbranchを削除（マージ済み・未マージ問わず）
- 削除前に確認プロンプト（`--force` でスキップ）

## パッケージ構成

```text
github.com/kawaken/gw
├── cmd/gw/main.go            # エントリポイント + ディスパッチャ
├── git/git.go                # git CLI ラッパー（Runner interface）
├── worktree/
│   ├── path.go               # main_repo_path, get_worktree_path, shorten_name, make_label
│   ├── resolve.go            # worktree名解決（完全一致、PR-N、#N）
│   ├── sort.go               # commit時刻でソートした一覧
│   └── format.go             # 表示フォーマット
├── metadata/metadata.go      # .git/worktrees/{name}/gw_metadata の read/write
├── config/config.go          # .gw/config (TOML) の parse/write（pelletier/go-toml/v2）
├── sync/sync.go              # ファイル同期（copy/symlink処理）
├── hook/hook.go              # GW_* 環境変数 + hook スクリプト実行
├── ui/
│   ├── prompt.go             # 確認ダイアログ、テキスト入力（/dev/tty 経由）
│   └── tui.go                # bubbletea によるTUI（prune選択 + sync-edit 編集）
├── output/output.go          # JSON出力構造体 + ヘルパー
└── subcmd/                   # 各サブコマンド
    ├── create.go, list.go, remove.go, switch.go, describe.go
    ├── review.go, prune.go, archive.go, activate.go
    ├── sync.go, syncedit.go
    ├── init.go                # gw init zsh（シェル関数 + 補完 + compdef 生成）
    ├── fmt.go                 # __fmt サブコマンド
    └── completion.go          # __completion サブコマンド
```

## 実装フェーズ

### Phase 1: 基盤 + JSON出力 + list / describe / init

**目標**: `gw list`, `gw describe`, `eval "$(gw init zsh)"` が動く

1. `go mod init github.com/kawaken/gw` + 依存追加
2. `output/output.go` - JSON出力構造体（Result{Messages, CD, ...}）
3. `git/git.go` - git CLI ラッパー（Runner interface）
4. `worktree/path.go` - main_repo_path, get_worktree_path, shorten_name, make_label
5. `metadata/metadata.go` - key=value 形式の read/write
6. `worktree/resolve.go` - worktree名解決
7. `worktree/sort.go` - sorted worktrees
8. `worktree/format.go` - 表示フォーマット
9. `cmd/gw/main.go` - ディスパッチャ（エイリアス解決含む）
10. `subcmd/fmt.go` - __fmt サブコマンド（stdin JSON → フィールド抽出）
11. `subcmd/list.go` - gw list (--path, -v, -a)
12. `subcmd/completion.go` - __completion tasks|archived
13. `subcmd/describe.go` - gw describe / --purpose（main worktreeでも動作）
14. `subcmd/init.go` - gw init zsh（gw()関数 + _gw()補完 + compdef を生text出力）

### Phase 2: cd 系 + sync + hook

**目標**: create, switch, remove, review, archive/activate, sync が動く

15. `ui/prompt.go` - 確認ダイアログ・テキスト入力（/dev/tty 経由）
16. `config/config.go` - .gw/config TOML parse/write（pelletier/go-toml/v2）
17. `sync/sync.go` - copy/symlink 処理
18. `hook/hook.go` - hook 実行（GW_* 環境変数セット）
19. `subcmd/switch.go` - resolve + cd出力（GW_PREVIOUS_WORKTREE対応）
20. `subcmd/archive.go` + `subcmd/activate.go`
21. `subcmd/sync.go`
22. `subcmd/create.go` - 既存確認 → main更新 → worktree作成 → sync → metadata → hook → cd
23. `subcmd/remove.go` - 確認プロンプト（--force でスキップ） → hook → git worktree remove → branch削除（デフォルトbranch以外） → cd
24. `subcmd/review.go` - gh連携 → fetch → worktree作成 → setup → hook → cd（既存なら cd のみ）

### Phase 3: TUI

**目標**: prune, sync-edit が動く

25. `ui/tui.go` - bubbletea による選択UI（prune用 multi-select）
26. `subcmd/prune.go` - 一覧 → TUI選択 → 一括削除（--older N でフィルター）
27. bubbletea で .gw/config 編集UI（sync-edit用）を `ui/tui.go` に追加
28. `subcmd/syncedit.go`

## Go化しないもの（そのまま残す）

- `bin/gw-claude-start` - claude CLI 起動 + cd（hookから呼ばれる）
- `bin/gw-claude-trust` - ~/.claude.json 編集（hookから呼ばれる）
- `bin/gw-post-review` - claude CLI 起動（hookから呼ばれる）
- `bin/gw-migrate` - 一度きりのマイグレーション

## テスト戦略

- `git/git.go`: `Runner` interface でモック可能
- `worktree/path.go`: 純粋関数のユニットテスト
- `metadata/`, `config/`, `sync/`: `t.TempDir()` でファイルI/Oテスト
- `ui/prompt.go`: `Prompter` interface でモック可能
- `output/`: JSON出力の構造体テスト
- サブコマンド: テスト用 git repo を TempDir に作ってインテグレーションテスト

## 検証方法

各フェーズ完了時に `make all` を実行する（ビルド + lint + テスト + フォーマット確認を一括）。

**Makefile ターゲット:**
- `make build` — `./bin/gw` をビルド
- `make test` — `-race -count=1` でテスト
- `make lint` — golangci-lint
- `make fmt-check` — gofmt + goimports フォーマット確認
- `make check` — fmt-check + lint + test（CI 用）
- `make all` — build + check（フェーズ完了確認用）

## 移植元ファイル

- `rc/git_gw` - 全ロジック（1145行）。サブコマンド + ヘルパー関数 + 補完関数
- `bin/gw-sync-edit` - TUI化が必要な最も複雑なスクリプト
- `bin/gw-claude-trust` - JSON操作（Go化対象外）
- `bin/gw-claude-start` - Claude起動（Go化対象外）
- `bin/gw-post-review` - Claude起動（Go化対象外）
- `bin/gw-migrate` - マイグレーション（Go化対象外）
