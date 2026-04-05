# CLAUDE.md

## 仕様

設計・フェーズ計画・サブコマンド仕様は `plan/go-migration.md` を参照。

## ルール

- Makefileにあるコマンドを使用してビルドやテストなどを実行する
- Goのコードではガード的なearly returnを心がける
- Linterのエラーを //nolint で無視しない
