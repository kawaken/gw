# gw

Git worktree の状態を観測・管理する Go CLI です。

worktree の作成や通常の開発セッション管理は Claude Code / Codex などのコーディングエージェントに任せ、`gw` は Git、GitHub PR、エージェントセッションの状態を集約して、放置された worktree の cleanup 候補を判定します。

## Install

チェックアウトしたリポジトリからインストールします。

```bash
go install ./cmd/gw
```

または、バイナリをビルドします。

```bash
go build -o ./gw ./cmd/gw
```

## Commands

```bash
gw list                  # worktreeの状態を一覧表示
gw list --json           # エージェント向けJSON
gw inspect <worktree>    # 詳細とcleanup判定理由
gw refresh               # GitHubなどの状態を再取得
gw clean --dry-run       # cleanup候補を確認
gw clean                 # 推奨候補だけを削除
gw guide                 # AI向けの使い方・仕様
```

`gw clean` は、PRがマージ済みまたはクローズ済みで、worktreeがcleanかつactiveなエージェントセッションがないものだけを推奨対象にします。dirtyなworktree、現在のworktree、状態が不明なworktreeは削除しません。初期版ではブランチを残します。

## Agent hooks

Claude Code / Codex のセッション状態を記録するhook設定は、次のコマンドで確認できます。

```bash
gw guide agent-hook claude
gw guide agent-hook codex
```

これらのコマンドは既存設定を変更せず、`SessionStart` / `SessionEnd` に追加する設定例を表示します。

## Data locations

設定・状態・キャッシュはXDGディレクトリに保存します。

```text
${XDG_CONFIG_HOME:-~/.config}/gw/
${XDG_STATE_HOME:-~/.local/state}/gw/
${XDG_CACHE_HOME:-~/.cache}/gw/
```

GitHub情報を取得できない場合や、エージェントhookが設定されていない場合は、該当する状態を `unknown` として扱います。
