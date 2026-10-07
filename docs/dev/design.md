# 設計

開発者向けの設計資料。利用者向けのドキュメントは別に置く。

## 目的

全体仮説を論点ごとにサブ仮説へ分解し、検証の進捗と根拠を管理する CLI `hyp`。
AI エージェント（Claude Code 等）が CLI を操作して仮説の分解を手伝い、根拠の事実確認と承認は人間が行う。

## 役割分担

- CLI は決定的な操作のみを提供し、LLM を呼ばない。分解はエージェントが CLI を呼んで行う
  - 理由: CLI を単純に保ち、API キー・課金・プロンプト管理を CLI に持ち込まないため。人間も同じ CLI を使う
- 根拠を探すのは AI でも人間でもよいが、承認は人間のみ

## 研究ディレクトリ

```
my-research/              # git リポジトリ
  hyp.yaml                # 研究ディレクトリの目印。CLI はカレントから上に辿って探す
  hypotheses/<id>.yaml    # 1 ノード 1 ファイル
  evidence/<id>/          # 根拠 1 件ごとのディレクトリ
    evidence.yaml
    <添付ファイル>
```

- 1 リポジトリに複数のルート仮説を置ける（親を持たないノードがルート）
- ノードごとにファイルを分ける理由: 添付ファイルの置き場所を作れる、AI が 1 ノードだけを書き換えやすい

## ID

- 16 進 8 桁のランダム値。CLI では一意に定まる前方一致で指定できる
- 内容ハッシュにしない理由: 仮説は後から書き換えるため、内容から計算すると ID が変わる
- 連番にしない理由: 移動・削除で番号が変わると、根拠やメモからの参照が壊れる
- `A-1-2` のような番号は表示時に毎回計算する

## 仮説ノード `hypotheses/<id>.yaml`

| フィールド | 内容 |
|---|---|
| `id` | ID |
| `parent` | 親の ID。ルートは空 |
| `title` | タイトル |
| `description` | 説明文 |
| `method` | 検証方法 |
| `status` | 末端ノードのみ。`untested` / `testing` / `supported` / `rejected` / `on-hold` |
| `combine` | 子を持つノードのみ。`and` / `or` / `cases` |
| `condition` | 親の `combine` が `cases` のときのみ。条件を表す文字列 |
| `evidence` | 根拠への紐づけのリスト。各要素は `id` と `stance`（`supports` / `refutes` / `inconclusive`） |
| `created_at` | 作成日時。兄弟の並び順（表示用の番号）に使う |

`stance` を根拠側ではなく紐づけ側に持つのは、同じ根拠がある仮説を支持し別の仮説を反証することがあるため。

確信度は持たない。値を読んで使う場面がなく、条件付き支持も表せないため。必要になったら追加する。

## 結論の導出

人間がステータスを付けるのは末端ノードのみ。子を持つノードの結論は子から計算する。

| combine | supported | rejected | それ以外 |
|---|---|---|---|
| `and` | 子がすべて supported | 子のいずれかが rejected | undetermined |
| `or` | 子のいずれかが supported | 子がすべて rejected | undetermined |
| `cases` | supported の子の条件のもとで supported | rejected の子の条件のもとで rejected | undetermined |

- 条件は上位へ伝播する。`and` の下では「かつ」、`or` の下では「または」で結合する
- `on-hold` は undetermined として扱う
- `(A and B) or C` のような関係は中間ノードで表す（`or` の下に `and` のノードを置く）。式を書く方式は、ノード削除で式が壊れること、ツリーと論理構造がずれることから採らない
- 条件は不透明な文字列として扱い、意味（排他・矛盾）は解釈しない
- 末端が子を持った時点で、その status は判定済みでも消す。判定済みのノードにサブ仮説を足すのはその判定を疑い直したことを意味し、後で子を除いても元の判定に戻すのは妥当でないため。以前の判定は git の履歴に残る
- 最後の子を失って末端に戻ったノードは `untested` になる
- 子を持つノードに根拠を紐づけることはできるが、判定には使わない
- 導出が保証するのは「分解が正しいならこう言える」まで。分解の妥当性は人間と AI の判断による

## 根拠 `evidence/<id>/evidence.yaml`

根拠は独立したエンティティとし、複数の仮説から ID で参照できる。

| フィールド | 内容 |
|---|---|
| `id` | ID |
| `summary` | 要約 |
| `source` | 出典 URL（任意） |
| `files` | 添付ファイルのパスと sha256 |
| `found_by` | 根拠を見つけた主体。`ai:<モデル名>` / `human:<名前>` |
| `approved_by` | 承認者。未承認なら空 |
| `approved_at` | 承認日時 |
| `approved_fingerprint` | 承認時点の fingerprint |

- 添付ファイルは `evidence/<id>/` にコピーする。研究ディレクトリだけで完結させ、承認後の変更を検知するため
- ファイルのバージョン管理は git に任せる
- fingerprint は `summary`・`source`・各添付ファイルの sha256 から計算する。添付だけでなく本文の書き換えも検知するため（Doorstop の suspect link と同じ考え方）
- 承認後に fingerprint が変わると `hyp check` が警告する

## git 連携

- 各コマンドは自分が変更したファイルだけをステージしてコミットする（`git add -A` は使わない）
  - 理由: CLI を経由しない直接編集が未コミットの差分として残り、検知できる
- コミットに `Hyp-Actor: <actor>` トレーラーを付ける。値は環境変数 `HYP_ACTOR`、未設定なら `human:<git user.name>`
- git は外部コマンドとして呼ぶ。利用者の git 設定・署名がそのまま効くため
- `hyp init` は必要なら `git init` まで行う

## AI による直接編集への対策

| 段階 | 方法 |
|---|---|
| 指示 | `hyp prime` の出力で `hyp` 経由で操作するよう指示する |
| 権限 | Claude Code の settings で `hypotheses/` `evidence/` への Edit/Write と `hyp evidence approve` を deny |
| 検知 | `hyp check`（スキーマ違反・ハッシュ不一致・未コミット変更） |

Bash 経由の書き込みは権限設定では防げない。承認の偽造を機械的に防ぐ必要が出たら、人間だけが持つ鍵による署名を検討する。

## コマンド

| コマンド | 内容 |
|---|---|
| `hyp init` | `hyp.yaml` 作成、必要なら `git init` |
| `hyp add [parent] <title>` | ノード追加。parent 省略でルート |
| `hyp set <id> --status/--combine/--condition/--method/--description/--title` | 属性更新 |
| `hyp mv <id> (<new-parent> \| --root)` | 移動 |
| `hyp rm <id>` | 削除。子を持つノードはエラー |
| `hyp show [id]` | 引数なしでルートの一覧、ID 指定でその下のツリーを表示 |
| `hyp undo [--force]` | 直前のコミットを打ち消すコミットを作る |
| `hyp conclude <id>` | 結論の導出。条件と未確定ノードを表示 |
| `hyp evidence add <node> --summary --file... --source --found-by` | 根拠を作成し添付をコピー |
| `hyp evidence link <node> <evidence>` | 既存の根拠を紐づけ |
| `hyp evidence approve <evidence>` | 承認（人間のみ） |
| `hyp check` | 整合性検査 |
| `hyp prime` | AI エージェント向けの使い方を出力する |
| `hyp setup claude` | Claude Code の SessionStart フックに `hyp prime` を登録し、deny 設定（`hyp evidence approve`、`hyp undo --force`、研究ディレクトリへの Edit/Write）と `HYP_ACTOR` を入れる |

- エージェント向けの使い方は、研究ディレクトリのファイルに書き込まず `hyp prime` が出力する。CLI の更新と使い方の説明がずれないため（Beads と同じ方式）
- `hyp prime` は研究の現在の状態を出力しない。状態はセッション中に変わるため、必要なときに `show` / `check` で取得させる
- `hyp undo` は直前のコミットだけを取り消す。古いコミットまで遡ると後の操作と衝突しうるため。hyp のコミット（`Hyp-Actor` を持つもの）以外は `--force` がなければ取り消さない。`--force` は CLI を経由しない変更まで消せるため、AI には使わせない
- `hyp undo` は git revert を使わず、直前のコミットで変わったファイルを一つ前の状態に戻してコミットする。取り消しのコミットにも対象のファイルだけを含め、`Hyp-Actor` を付けるため
- `hyp init` はエージェントの設定に触れず、`hyp setup claude` を明示的に実行したときだけ入れる。権限設定まで変更するため、`init` のついでに行われると気づきにくい

出力は英語。ID 引数は Cobra の動的補完で Tab 補完できるようにする。
`show` と `conclude` は `--json` を持つ。AI がツリーを読むとき、人間向けの表示を解析させないため。

## 先行研究

| 名前 | 関係 |
|---|---|
| Ding et al. "The Diagram is like Guardrails" (arXiv 2503.16791) | 木を AI との共有表現にする。木は探索履歴で論理合成はない |
| Giorgini et al. "Reasoning with Goal Models" (ER 2002) | AND/OR 分解でのラベル伝播。結論導出の先例 |
| Ali, Dalpiaz, Giorgini "Contextual Goal Models" | 条件付き分解。`cases` の先例 |
| GSN / gsn2x | 主張の分解と根拠の対応付け。YAML + CLI |
| Doorstop | 1 項目 1 YAML、fingerprint による変更検知 |
| Beads | AI が使う CLI。JSON 出力、ハッシュ型 ID |

## 実装

- Go。依存は `github.com/spf13/cobra`（サブコマンドと Tab 補完）、`gopkg.in/yaml.v3`
