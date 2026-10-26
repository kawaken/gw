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

新しい worktree は `new` で作成できます。`origin/main` を fetch してから、名前と同じ新規ブランチで作成し、作成したパスだけを出力します。

```console
$ gw new fix-login
/home/user/.local/share/gw/worktrees/repository/fix-login
$ gw new fix-login --base origin/develop --branch feature/login --json
```

配置先は `<root>/<repo>/<名前>` です。`<root>` は `--root`、環境変数 `GW_WORKTREE_ROOT`、`${XDG_DATA_HOME:-~/.local/share}/gw/worktrees` の順に決まります。同じ名前の worktree が既にあれば、作り直さずにそのパスを返します。既存のブランチ名を指定した場合や、配置先が別のものと衝突する場合は失敗します。失敗の種類ごとの終了コードは `gw guide new` で確認できます。

削除候補は、まず dry-run で確認できます。

```console
$ gw clean --dry-run
$ gw clean
```

引数なしの `clean` が対象にするのは、pull request がマージ済みまたはクローズ済みで、worktree が clean、push していないコミットがなく、かつ実行中のエージェントがないものです。メイン worktree、ロックされた worktree、状態を確認できないものは削除しません。

worktree を指定すると、そのworktreeだけを対象にします。指定はパス、ブランチ名、ディレクトリ名のどれでもかまいません。判定に合わないものは削除せず、理由を表示します。`--force` を付けると判定を無視して削除します。実行中のエージェントやロックがある worktree は、`--force` を 2 回付けたときだけ削除します。

```console
$ gw clean fix-login
$ gw clean fix-login --force --dry-run
$ gw clean fix-login --force
$ gw clean fix-login --force --force
```

既定では、worktree と一緒にローカルブランチも削除します。ブランチを残したい場合は `--save-branch` を付けます。削除したブランチの先頭コミット ID を表示するので、`git branch <ブランチ名> <コミットID>` で戻せます。`--dry-run` では、削除で失われるもの（未コミットのファイル数、push していないコミット数、PR の状態）を確認できます。

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

以前のキャッシュにある`available`は、PRの有無に応じて`found`または`not_found`へ読み替えます。

JSONの `schema_version` は `3` です。`2` からの主な変更は、`gw lock` の状態（`lock`）、未コミットのファイル数と push していないコミット数（`git.uncommitted`、`git.unpushed`）、PR の先頭コミット（`github.pr.head_oid`）の追加と、`clean --json` の `removed` をパスの配列からオブジェクトの配列に変えたことです。詳細は `gw guide json` と `gw guide clean` で確認できます。

### 外部ツールから起動する場合

外部ツールがエージェントの CLI を起動する場合は、hook ではなく `gw lock` で実行中を宣言します。CLI を起動したら `gw lock <worktree> --pid <PID>`、終了したら `gw unlock <worktree>` を実行します。生きているプロセスの lock がある worktree は `clean` の対象になりません。gw は PID の生存を確認するので、プロセスが終了した lock は古いものとして扱われ、保護しません。

このとき、環境変数 `GW_MANAGED=1` を付けて CLI を起動してください。hook はセッションと worktree の紐付けを記録しますが、lifecycle を `managed` にして、実行中の判定には使いません。詳細は `gw guide lock` で確認できます。

## 状態の保存場所

エージェントセッションの状態は、XDG Base Directory に従って次の場所に保存されます。`gw lock` の lock は、同じディレクトリの `locks/` に worktree ごとに保存されます。

```text
${XDG_STATE_HOME:-~/.local/state}/gw/sessions.json
${XDG_STATE_HOME:-~/.local/state}/gw/locks/
```

GitHub PR 情報のキャッシュは、同じ状態ディレクトリ配下の `cache/github/` にリポジトリごとに保存されます。キャッシュが期限切れまたは壊れている場合は再取得し、再取得できない場合は cleanup 対象にしません。
