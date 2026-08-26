# gw 観測・状態管理ツール

## 1. 目的

`gw` は、Git worktree とその作業状態を観測し、不要になった worktree の整理を支援する CLI です。

担当するのは次の範囲です。

- Git worktree の状態把握
- GitHub pull request との関連付け
- Claude Code / Codex のセッション状態の観測
- cleanup 候補の判定
- 安全条件を満たす worktree の削除

worktree の作成や通常の開発セッション管理は、それぞれの開発ツールに任せます。

## 2. 設計

- Go の単体バイナリとして動作する
- 1回の実行では、現在の Git リポジトリに紐づく worktree を対象にする
- `git worktree list --porcelain` で発見できる worktree を対象にする
- `.git` や worktree 内に独自メタデータを書き込まない
- 人間向けの通常出力と、`--json` による構造化出力を提供する
- GitHub やエージェント情報が取得できない場合も、Git の情報を返す
- 取得できない情報は推測せず、`unknown` として扱う

## 3. CLI

### 状態確認

```text
gw list [--json]
gw inspect [<worktree>] [--json]
gw refresh [--json]
```

`list` は worktree の状態を一覧表示し、`inspect` は指定した worktree の詳細と cleanup 判定理由を表示します。`inspect` の引数を省略するとメイン worktree を対象にします。

`refresh` は Git と利用可能な連携先から現在の状態を再取得します。結果を永続化する更新処理ではありません。

### cleanup

```text
gw clean --dry-run [--json]
gw clean [--json]
```

`--dry-run` は削除候補を表示するだけで、worktree を削除しません。通常の `clean` は `recommended` と判定された worktree に対して `git worktree remove` を実行します。

### ガイドと内部コマンド

```text
gw guide
gw guide list|inspect|clean|json
gw guide agent-hook claude|codex
gw agent-event --provider claude|codex
```

`guide` は現在のバイナリが対応している使い方、JSON出力、安全条件、agent hook の設定例を表示します。`agent-event` は Claude Code / Codex の hook から呼び出す内部コマンドです。

## 4. JSON 出力

`list`、`inspect`、`refresh` は `--json` に対応しています。`clean` は dry-run と削除実行の結果を、それぞれ `CleanupReport` として返します。

状態取得結果のトップレベルは次の構造です。

```json
{
  "schema_version": 1,
  "repository": {},
  "worktrees": [],
  "sources": {},
  "errors": []
}
```

worktree には `path`、`branch`、`head`、`detached`、`locked` と、次の状態を含みます。

- `git`: clean / dirty、upstreamとの差分、最終コミット時刻
- `github`: pull request と取得状態
- `agent`: provider、session ID、lifecycle、activity、観測時刻
- `cleanup`: `recommended` / `review` / `keep` と判定理由

値を取得できない場合は、空文字ではなく `null` または `unknown` を使います。GitHub や agent のエラーがあっても、Git の結果は可能な限り返します。

`clean --json` の結果には `schema_version`、`repository`、`mode`、`candidates`、`removed`、`errors` が含まれます。

## 5. GitHub 連携

GitHub の情報取得には `gh` CLI を使用します。ブランチをもとに pull request を検索し、番号、タイトル、状態、URL、merge 時刻などを取得します。

`gh` がない、認証できない、通信に失敗したなどの場合は、GitHub の状態を `unknown` または `unavailable` として扱います。GitHub の結果はキャッシュせず、各状態取得時に再取得します。

## 6. エージェントセッション

Claude Code と Codex の `SessionStart` / `SessionEnd` hook を利用します。hook の標準入力から provider、session ID、cwd、イベント名、終了理由、観測時刻などを受け取り、セッション状態を更新します。

同じ provider と session ID の記録は上書きする snapshot 方式です。transcript の内容は解析しません。

現在の lifecycle は `active`、`ended`、`unknown`、activity は `unknown` です。hook が設定されていない場合や状態を取得できない場合も `unknown` として扱います。

## 7. cleanup 判定

判定は次の3段階です。

### recommended

pull request が `MERGED` または `CLOSED` で、worktree が clean、active なエージェントセッションがない場合です。

### review

dirty な worktree、pull request がないもの、open の pull request、GitHub の状態が不明なものなどです。自動削除しません。

### keep

メイン worktree、ロックされた worktree、active なエージェントセッションがあるものです。

`gw clean` が削除するのは `recommended` の worktree だけです。worktree を削除しても、ブランチは削除しません。

## 8. 状態保存

エージェントセッションの状態だけを XDG Base Directory に保存します。

```text
${XDG_STATE_HOME:-~/.local/state}/gw/sessions.json
```

設定ファイル、GitHub キャッシュ、観測結果の永続保存は現在使用していません。

## 9. 実装状況

- CLI の基本コマンド、help、guide: 実装済み
- Git worktree と Git 状態の観測: 実装済み
- JSON 出力とエラー情報: 実装済み
- GitHub pull request 連携: 実装済み
- Claude Code / Codex の agent hook 連携: 実装済み
- cleanup 候補の判定と `git worktree remove`: 実装済み
- TUI、複数リポジトリ横断、自動 daemon、MCP: 対象外

## 10. 今後の検討事項

現時点で仕様を決め切っていないものは、必要になった時点で検討します。

- JSON の全フィールドと列挙値を、外部向けの正式な仕様として固定するか
- リポジトリごとの設定や ignore が必要か。その場合の設定ファイル形式
- GitHub の問い合わせが増えた場合のキャッシュと有効期限
- cleanup の一部失敗時に終了コードを非ゼロにするか
- マージ済みブランチを削除する明示的なオプションが必要か

現在の実装は、設定やキャッシュを持たず、取得時に GitHub を再照会する単純な構成です。
