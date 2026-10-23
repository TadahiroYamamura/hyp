# hyp

全体仮説をサブ仮説に分解し、検証の進捗と根拠を管理する CLI。

## インストール

Go 1.26.3 以上が必要。

```
go install github.com/TadahiroYamamura/hyp@latest
```

## はじめ方

```
mkdir my-research && cd my-research
hyp init            # 研究ディレクトリを作る。git リポジトリでなければ git init もする
hyp setup claude    # Claude Code で使うなら。使い方の読み込み・権限の設定を .claude/settings.json に入れる
hyp add "全体仮説"
```

コマンドの一覧は `hyp --help`、AI エージェント向けの使い方は `hyp prime` で表示する。

設計は [docs/dev/design.md](docs/dev/design.md) を参照。
