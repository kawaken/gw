# gw 観測・状態管理ツール計画

## 1. 方針

`gw` は、Git worktree の作成・削除を所有するツールではなく、既存の worktree と、その作業状態を観測・管理するツールにする。

当初は `gw` が worktree のライフサイクル全体を管理し、その中でコーディングエージェントを起動する想定だった。しかし現在は Claude Code や Codex などのエージェントが、それぞれの作法・配置・セッション管理で worktree を扱う。

そのため、worktree 単体の作成・削除はエージェントに任せ、`gw` は以下を担当する。

- Git worktree の状態把握
- GitHub PR との関連付け
- Claude Code / Codex セッション状態の観測
- 放置された worktree の cleanup 候補判定
- 安全性の高い cleanup の実行

## 2. 設計原則

- Go の単体バイナリとして動作する
- シェル関数による `cd` ラッパーは持たない
- 単一リポジトリを対象にする
- `git worktree list --porcelain` で発見できる worktree を対象にする
- `.git` や worktree 内に独自メタデータを書き込まない
- 人間向けの通常 CLI と、エージェント向けの `--json` 出力を提供する
- GitHub やエージェント情報が取得できない場合も、Git の情報だけで動作する
- MCP は実装しない
- 取得できない情報は推測せず `unknown` とする

## 3. コマンド体系

### 通常コマンド

```text
gw list
gw list --json
gw inspect <worktree>
gw inspect <worktree> --json
gw refresh
gw clean --dry-run
gw clean
```

- `list`: worktree ごとの Git・GitHub・エージェント状態を一覧表示する
- `inspect`: 1つの worktree の状態と判定理由を詳細表示する
- `refresh`: GitHub など外部状態を再取得し、状態を更新する
- `clean --dry-run`: cleanup 対象を表示するが、削除しない
- `clean`: cleanup 推奨対象だけを削除する

`clean` は worktree 本体だけを削除し、ブランチは初期版では残す。worktree の削除には `git worktree remove` を使う。dirty な worktree、現在の worktree、エージェントが active な worktree は削除しない。

`git worktree prune` は、すでに存在しない worktree に対応する Git 管理情報の掃除に使えるが、実際の worktree ディレクトリを削除する cleanup 本体とは区別する。

### `guide` コマンド

`guide` を AI 向けの公式な説明入口にする。

```text
gw guide
gw guide list
gw guide inspect
gw guide clean
gw guide json
gw guide agent-hook claude
gw guide agent-hook codex
```

`gw help` のコマンド一覧には、次のような説明を出す。

```text
guide  AI向けに各サブコマンドの使い方や連携方法を説明
```

`guide` はファイルを変更せず、現在のバイナリが対応している使い方・JSON仕様・安全条件・hook設定例を表示する。

特に `gw guide agent-hook claude` / `gw guide agent-hook codex` は、既存の設定を自動編集せず、ユーザーが追加すべき設定と確認方法を説明する。

### 内部コマンド

```text
gw agent-event --provider claude
gw agent-event --provider codex
```

`agent-event` は Claude Code / Codex の hook から JSON を stdin で受け取る内部用コマンド。通常の help 一覧には表示せず、`gw guide agent-hook ...` から説明する。

## 4. JSON 出力

主要な状態取得コマンドは `--json` をサポートする。人間向け出力も JSON と同じ内部データを表示する。

トップレベルの構造は次を基本とする。

```json
{
  "schema_version": 1,
  "repository": {},
  "worktrees": [],
  "sources": {},
  "errors": []
}
```

worktree には少なくとも次を含める。

```json
{
  "path": "/path/to/worktree",
  "branch": "feature/example",
  "head": "abc1234",
  "git": {
    "clean": true,
    "ahead": 0,
    "behind": 0
  },
  "github": {
    "pr": null
  },
  "agent": {
    "provider": null,
    "session_id": null,
    "lifecycle": "unknown",
    "activity": "unknown"
  },
  "cleanup": {
    "recommendation": "review",
    "reasons": []
  }
}
```

JSON の方針:

- `schema_version` を必ず含める
- 取得不能な値は空文字ではなく `null` または `unknown`
- GitHub や agent のエラーで Git の結果を失わない
- cleanup 判定の根拠を安定したコードで返す
- 時刻は ISO 8601 形式にする

## 5. Git 状態

Git から以下を取得する。

- worktree のパス
- ブランチ名、detached HEAD
- HEAD
- clean / dirty
- ahead / behind
- 最終コミット時刻
- worktree 管理情報の異常

独立した clone や `git worktree list` から発見できないディレクトリは、初期版の管理対象外とする。

## 6. GitHub 状態

GitHub 情報は `gh` CLI を利用できる場合のみ取得する。`gh` が存在しない、未ログイン、通信エラーなどの場合は `unknown` とする。

ブランチを基本キーとして PR を関連付け、次を取得する。

- PR 番号、タイトル、URL
- open / closed / merged
- merge 時刻
- remote branch の存在

GitHub の状態取得はローカル Git の状態と分離し、外部取得に失敗しても `gw list` 自体は成功させる。

## 7. エージェントセッション

Herdr などの外部セッションマネージャーには依存しない。

Claude Code と Codex が提供する公式 hook を利用する。hook から受け取ったイベントを `gw agent-event` が保存する。

利用するイベントは基本的に次の2つ。

- `SessionStart`
- `SessionEnd`

hook の入力から次を保存する。

- provider
- session ID
- worktree / cwd
- transcript path（参照先として必要な場合のみ）
- event name
- 終了理由
- 観測時刻

transcript の内容は解析しない。transcript はエージェントとの会話、tool 呼び出し、結果などを記録したファイルであり、形式変更や機密情報の問題があるため、cleanup 判定の情報源にはしない。

セッション状態は、Git の状態と分離して以下のように扱う。

```text
lifecycle: active | ended | unknown
activity:  working | waiting | unknown
```

正常終了時に `SessionEnd` を受け取れない場合は `ended` と断定せず `unknown` とする。セッション終了だけで worktree の削除を決定せず、Git / GitHub 状態と組み合わせる。

## 8. 状態保存

XDG の標準ディレクトリを利用する。

```text
${XDG_CONFIG_HOME:-~/.config}/gw/
  config.toml       # ユーザー設定、ignore、cleanup設定

${XDG_STATE_HOME:-~/.local/state}/gw/
  sessions.json     # agent hook から得たセッション状態
  observations.json # 必要に応じた観測状態

${XDG_CACHE_HOME:-~/.cache}/gw/
  github/           # 再取得可能な GitHub キャッシュ
```

初期版では、Git から再取得できる情報は永続保存せず、agent event やユーザーの ignore など再取得できない情報だけを保存する方針を基本とする。

## 9. cleanup 判定

cleanup 判定は `recommended` / `review` / `keep` の3段階とする。

### recommended

次のような強い終了根拠があり、worktree が clean で、active な agent session がない場合。

- PR が merged
- PR が closed
- ブランチが削除済み

### review

- 一定期間更新がない
- dirty な worktree
- GitHub 状態が `unknown`
- agent session が `unknown` または再開可能
- セッションは終了しているが、PR やブランチの状態が不明

### keep

- 現在の worktree
- Git の状態取得に失敗している
- active な agent session が存在する

`gw clean` は `recommended` のみをデフォルトで削除する。`review` は削除しない。

初期版ではブランチを削除しない。必要になった場合に、マージ済みブランチだけを対象とする明示的なオプションを追加する。

## 10. 設定ガイド

`gw guide agent-hook claude` と `gw guide agent-hook codex` は、次を表示する。

- 設定ファイルの場所
- `SessionStart` / `SessionEnd` に追加する hook の例
- `gw agent-event` の役割
- 既存の hook を壊さずに手動マージする方法
- 動作確認方法
- 設定を戻す方法

既存設定の自動上書きは行わない。

## 11. 実装フェーズ

### Phase 1: CLI基盤とローカルGit観測

- Go 単体CLIの骨格
- `help` / `guide`
- `list` / `inspect`
- `git worktree list --porcelain` の解析
- Git状態のJSON出力
- XDGパスの解決

### Phase 2: JSON仕様と説明ガイド

- `schema_version` と状態モデルの固定
- `--json` の共通出力
- `guide json`
- エラー・終了コードの整理

### Phase 3: GitHub連携

- `gh` の存在確認
- PR検索・状態取得
- `unknown` と部分的エラーの扱い
- `refresh`

### Phase 4: Agent hook連携

- `agent-event`
- Claude / Codex のイベント形式解析
- セッション状態の保存
- `guide agent-hook claude|codex`
- hook未設定時の `unknown` 表示

### Phase 5: cleanup

- `clean --dry-run`
- 推奨・要確認・維持の判定
- `git worktree remove`
- dirty / current / active session の保護

## 12. 対象外

初期版では次を実装しない。

- TUI
- 複数リポジトリ横断管理
- worktree の作成・削除を主機能にすること
- transcript の直接解析
- Claude / Codex 以外のエージェント対応
- 自動 daemon
- MCP
- 初期版でのブランチ削除
- cleanup ポリシーの細かいリポジトリ別カスタマイズ

## 13. 未決定事項

実装開始前に次を確定する。

- JSON の全フィールド名と列挙値
- `config.toml` を含む設定ファイルの具体的な形式
- agent event の保存形式（snapshot / append-only log）
- GitHub API 呼び出しのキャッシュと有効期限
- cleanup の終了コードと削除結果のJSON形式
- Goの標準ライブラリと外部ライブラリの最終選定
