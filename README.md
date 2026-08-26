# gw

Git worktree の状態を確認し、不要になった worktree の整理を支援する CLI です。

各 worktree の Git の状態に加えて、GitHub の pull request とコーディングエージェントのセッション状態をまとめて確認できます。

## インストール

Go が使える環境では、次のコマンドでインストールできます。

```console
$ go install github.com/kawaken/gw/cmd/gw@latest
```

## 使い方

Git リポジトリ内で実行します。

```console
$ gw list
PATH                         BRANCH       GIT    AGENT          CLEANUP
/path/to/repository          main         clean  unknown        keep
/path/to/worktree            feature/foo  clean  claude:ended   recommended
```

個別の worktree を詳しく確認するには、`inspect` を使います。引数を省略するとメイン worktree を表示します。

```console
$ gw inspect /path/to/worktree
$ gw inspect feature/foo
```

削除候補は、まず dry-run で確認できます。

```console
$ gw clean --dry-run
$ gw clean
```

`clean` が対象にするのは、pull request がマージ済みまたはクローズ済みで、worktree が clean、かつ実行中のエージェントセッションがないものです。メイン worktree、ロックされた worktree、状態を確認できないものは削除しません。削除されるのは worktree で、ブランチは残ります。

他のツールから利用する場合は、`--json` を付けると構造化された結果を取得できます。

```console
$ gw list --json
$ gw inspect feature/foo --json
$ gw clean --dry-run --json
```

## エージェント連携

Claude Code または Codex の hook からセッションの開始・終了を `gw` に通知できます。設定例は次のコマンドで表示されます。

```console
$ gw guide agent-hook claude
$ gw guide agent-hook codex
```

表示された設定を、利用しているエージェントの hook 設定に追加してください。`gw` は既存の設定を変更しません。

GitHub の pull request 情報は `gh` コマンドが利用できる場合に取得します。取得できない情報は推測せず、`unknown` として扱います。

## 状態の保存場所

エージェントセッションの状態は、XDG Base Directory に従って次の場所に保存されます。

```text
${XDG_STATE_HOME:-~/.local/state}/gw/sessions.json
```
