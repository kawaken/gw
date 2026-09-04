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

削除時は Git の worktree 登録と実体ディレクトリを、ignored なビルド生成物も含めて強制的に削除します。削除に失敗した場合は Git のエラー理由を表示し、終了コード1を返します。

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

GitHub の pull request 情報は `gh` コマンドが利用できる場合に取得します。通常の `list` と `inspect` は、直近 30 分以内に取得した PR 情報をキャッシュから再利用します。`refresh` と `clean` はキャッシュを使わず、現在の情報を再取得します。

JSON の `worktrees[].github.status` は、worktreeごとのPR検索結果を表します。`found` はPRあり、`not_found` は問い合わせ成功・PRなし、`unknown` はブランチがなく確認できない状態、`unavailable` はGitHub連携が利用できないか取得に失敗した状態です。取得元（`gh`、`cache`など）はトップレベルの `sources.github` に分離して記録されます。取得に失敗した場合は `errors` にも詳細を記録します。

この意味変更に伴い、JSONの `schema_version` は `2` です。以前のキャッシュにある`available`は、PRの有無に応じて`found`または`not_found`へ読み替えます。

## 状態の保存場所

エージェントセッションの状態は、XDG Base Directory に従って次の場所に保存されます。

```text
${XDG_STATE_HOME:-~/.local/state}/gw/sessions.json
```

GitHub PR 情報のキャッシュは、同じ状態ディレクトリ配下の `cache/github/` にリポジトリごとに保存されます。キャッシュが期限切れまたは壊れている場合は再取得し、再取得できない場合は cleanup 対象にしません。
