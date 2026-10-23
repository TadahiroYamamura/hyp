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
| `axis` | 子を持つノードのみ。子を切る基準（分解の軸）。1 階層に 1 つ |
| `weight` | ルート以外。親にとっての重み（インパクト）。兄弟の中での相対値 |
| `evidence` | 根拠への紐づけのリスト。各要素は `id` と `stance`（`supports` / `refutes` / `inconclusive`） |
| `created_at` | 作成日時。兄弟の並び順（表示用の番号）に使う |

`stance` を根拠側ではなく紐づけ側に持つのは、同じ根拠がある仮説を支持し別の仮説を反証することがあるため。

`weight` は読むときに兄弟の合計で割って正規化する。合計を 1 に保つ調整を人や AI にさせると、子を足すたびに兄弟全員を書き換えることになるため。`weight` のない兄弟は、`weight` のある兄弟の平均として扱う（全員がなければ均等）。`cases` の下では、その場合が全体に占める割合を表す。

確信度は持たない。値を読んで使う場面がなく、条件付き支持も表せないため。必要になったら追加する。

## 結論の導出

人間がステータスを付けるのは末端ノードのみ。子を持つノードの結論は子から計算する。

| combine | supported | rejected | それ以外 |
|---|---|---|---|
| `and` | 子がすべて supported | 子のいずれかが rejected | undetermined |
| `or` | 子のいずれかが supported | 子がすべて rejected | undetermined |
| `cases` | supported の子の条件のもとで supported | rejected の子の条件のもとで rejected | undetermined |

- 条件は上位へ伝播する。`and` の下では「かつ」、`or` の下では「または」で結合する
- 導出結果は「支持される条件」と「棄却される条件」の 2 つの論理式で持ち、結論は `supported`（支持の条件が恒真）/ `rejected`（棄却の条件が恒真）/ `conditional`（どちらかが条件付き）/ `undetermined`（どちらも偽）の 4 つになる
- 未確定の末端は Kleene の三値論理と同じく扱う。`and` の子に rejected が一つあれば、ほかが未確定でも rejected に確定する
- 結論が確定したら、残りの未確定の末端は結論に影響しないので pending に挙げない
- `cases` の場合分けは網羅的である前提に立ち、すべての場合で支持（棄却）されたときに限り条件なしの支持（棄却）とする
- 論理式の簡約は、定数の吸収・入れ子の平坦化・重複の除去だけ。条件の意味を解釈しないため
- `combine` 未設定のノード、条件のない場合、`cases` 以外の親の下の `condition` は、未確定（または条件を無視）として扱い警告する
- 進捗（progress）は、結論のうち判定済みの割合を `weight` で加重したもの。結論が確定した部分木は 1 とする。仮説が正しい確率ではない。`and` の下では、判定済みが 8 割でも残りが棄却されれば親は棄却されるため。確率を出すには末端ごとの確率と独立性の仮定が要り、扱わない
- `cases` のノードでは、支持・棄却された場合が占める割合を `weight` で出す。「どの範囲に展開すれば根拠があるか」の判断に使う
- 未確定の末端（pending）は、対象のノードにとっての重み（経路上の正規化した `weight` の積）が大きい順に並べる。各末端には、その末端だけが支持・棄却されたときの結論を添える。掘る枝や検証の順番を選ぶ材料にするため
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
| `created_at` | 作成日時 |
| `approved_by` | 承認者。未承認なら空 |
| `approved_at` | 承認日時 |
| `approved_fingerprint` | 承認時点の fingerprint |

- 添付ファイルは `evidence/<id>/` にコピーする。研究ディレクトリだけで完結させ、承認後の変更を検知するため
- ファイルのバージョン管理は git に任せる
- fingerprint は `summary`・`source`・各添付ファイルの sha256 から計算する。添付だけでなく本文の書き換えも検知するため（Doorstop の suspect link と同じ考え方）
- 承認後に fingerprint が変わると `hyp check` が警告する
- 承認時は、記録済みのハッシュではなく添付ファイルの現在の内容から sha256 と fingerprint を計算し直す。承認者が確認したのは現在の内容のため
- 主体が `ai:` で始まる場合、`approve` はエラーにする。権限設定（deny）を補う二重の防御で、AI が `HYP_ACTOR` を名乗っている限り誤って承認できない
- 添付ファイルはファイル名だけで `evidence/<id>/` の直下に置く。同じ名前の添付と `evidence.yaml` という名前は拒否する
- `found_by` を省略したら、操作の主体（`HYP_ACTOR` または git の利用者）を入れる
- 紐づけ時の `stance` は必須。既定値を置くと、向きを考えずに紐づけた根拠が支持として扱われるため
- 紐づけ済みの根拠を別の `stance` で `link` すると、`stance` だけを更新する
- `unlink` は外した `stance` をコミットメッセージに残す。紐づけ直すときに必要になるため
- `rm` は、根拠のディレクトリに記録にないファイルがあれば何も消さずにエラーにする。git で追跡されていない可能性があり、消すと戻せないため
- 承認状態は `unapproved` / `approved` / `changed-since-approval` の 3 つ。添付と本文から fingerprint を計算し直して判定する

## git 連携

- 各コマンドは自分が変更したファイルだけをステージしてコミットする（`git add -A` は使わない）
  - 理由: CLI を経由しない直接編集が未コミットの差分として残り、検知できる
- コミットに `Hyp-Actor: <actor>` トレーラーを付ける。値は環境変数 `HYP_ACTOR`、未設定なら `human:<git user.name>`
- 研究の中身を変えるコマンド（`add` `set` `mv` `rm` `undo` と `evidence` の `add` `link` `unlink` `approve` `rm`）は `--reason` を受け取り、コミットメッセージの本文に書く。件名は操作を表す定型文のまま
  - 理由をノードの YAML に持たせない理由: 変更のきっかけ（人の質問・指示、AI の提案、調査の結果など）はその時点の事情で、ノードの今の状態ではなく変更の履歴に属するため。`git log` や `git blame` から辿れる
  - 主体が `ai:` で始まるときは必須にし、ファイルを変更する前にエラーにする。AI の変更にはきっかけが常にあり、残さないと経緯を辿れないため
  - 人には必須にしない。人が直接操作するのは主に承認で、変更は AI が行う運用を想定するため
  - `init` と `setup claude` は研究の中身を変えないので受け取らない
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

研究の中身を変えるコマンドは、表に書いた引数に加えて `--reason` を受け取る（「git 連携」を参照）。

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
| `hyp evidence unlink <node> <evidence>` | 紐づけを外す。根拠そのものは残す |
| `hyp evidence approve <evidence>` | 承認（人間のみ） |
| `hyp evidence rm <evidence>` | 根拠を添付ごと削除。どの仮説にも紐づいていないときのみ |
| `hyp evidence show <evidence>` | 根拠の中身、承認状態、紐づけている仮説を表示 |
| `hyp check` | 整合性検査 |
| `hyp prime` | AI エージェント向けの使い方を出力する |
| `hyp setup claude` | 研究ディレクトリの `.claude/settings.json` に、SessionStart フックで `hyp prime` を実行する設定、deny 設定、`HYP_ACTOR=ai:claude` を入れる |

- エージェント向けの使い方は、研究ディレクトリのファイルに書き込まず `hyp prime` が出力する。CLI の更新と使い方の説明がずれないため（Beads と同じ方式）
- `hyp prime` は研究の現在の状態を出力しない。状態はセッション中に変わるため、必要なときに `show` / `check` で取得させる
- `hyp undo` は直前のコミットだけを取り消す。古いコミットまで遡ると後の操作と衝突しうるため。hyp のコミット（`Hyp-Actor` を持つもの）以外は `--force` がなければ取り消さない。`--force` は CLI を経由しない変更まで消せるため、AI には使わせない
- `hyp undo` は git revert を使わず、直前のコミットで変わったファイルを一つ前の状態に戻してコミットする。取り消しのコミットにも対象のファイルだけを含め、`Hyp-Actor` を付けるため
- `HYP_ACTOR` は `ai:claude` に固定する。モデル名まで入れると、会話の途中でモデルを切り替えたときに記録と実際がずれるため
- deny 設定は `hyp evidence approve`、`hyp undo --force`、`hyp.yaml`・`hypotheses/`・`evidence/` への Edit。Edit の規則は Write など、ほかのファイル書き換えツールにも効く。hyp が管理するもの以外（`.claude/settings.json` 自体など）は対象にしない
- `hyp evidence approve` と `hyp undo --force` は、deny 設定に加えて CLI でも主体が `ai:` なら拒否する
- `hyp setup claude` は既存の設定を残して足すだけで、何度実行しても結果は変わらない
- `hyp init` はエージェントの設定に触れず、`hyp setup claude` を明示的に実行したときだけ入れる。権限設定まで変更するため、`init` のついでに行われると気づきにくい

## 整合性の検査 `hyp check`

| 重大度 | 検出するもの |
|---|---|
| error | 読めない・定義にないフィールドを含む YAML、正でない `weight`、ファイル名（ディレクトリ名）と ID の食い違い、形式に合わない ID、必須項目の欠落、存在しない親・循環、子を持つノードの status、定義にない値、存在しない根拠への紐づけ・重複した紐づけ、読めない添付ファイル、承認後の変更、人間以外による承認、`hyp.yaml`・`hypotheses/`・`evidence/` の未コミットの変更 |
| warning | `combine` 未設定、ルートの `weight`、一部の兄弟だけが持つ `weight`、末端の `axis`、条件のない場合、`cases` 以外の親の下の `condition`、どこにも紐づいていない根拠、記録にないファイル、未承認の根拠の記録後の変更 |

- error が 1 件でもあれば終了コード 1 で終わる
- 通常のコマンドは読み込みで最初の問題に当たると止まる。`check` は壊れたファイルがあっても残りを読んで、すべてを報告する
- YAML は定義にないフィールドをエラーにする。直接編集したときの綴り間違いを見逃さないため
- 研究ディレクトリが git リポジトリのサブディレクトリでも動くよう、git から得るパスは研究ディレクトリからの相対パスに直して扱う

出力は英語。ID 引数は Cobra の動的補完で Tab 補完できるようにする。
`show`・`conclude`・`evidence show` は `--json` を持つ。AI がツリーを読むとき、人間向けの表示を解析させないため。

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
