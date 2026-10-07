package research

import (
	"strings"
	"testing"
	"time"
)

// 三値の真理値。期待値は実装ではなく、Kleene の三値論理の真理値表から手で書く。
type tv int

const (
	tvT tv = iota
	tvF
	tvU
)

// 末端の status を三値に対応づける。supported と rejected 以外は未確定。
var statusTV = map[Status]tv{
	StatusSupported: tvT, StatusRejected: tvF,
	StatusUntested: tvU, StatusTesting: tvU, StatusOnHold: tvU,
}

var andTable = map[[2]tv]tv{
	{tvT, tvT}: tvT, {tvT, tvF}: tvF, {tvT, tvU}: tvU,
	{tvF, tvT}: tvF, {tvF, tvF}: tvF, {tvF, tvU}: tvF,
	{tvU, tvT}: tvU, {tvU, tvF}: tvF, {tvU, tvU}: tvU,
}

var orTable = map[[2]tv]tv{
	{tvT, tvT}: tvT, {tvT, tvF}: tvT, {tvT, tvU}: tvT,
	{tvF, tvT}: tvT, {tvF, tvF}: tvF, {tvF, tvU}: tvU,
	{tvU, tvT}: tvT, {tvU, tvF}: tvU, {tvU, tvU}: tvU,
}

var tvConclusion = map[tv]Conclusion{tvT: ConclusionSupported, tvF: ConclusionRejected, tvU: ConclusionUndetermined}

// build はノードを作成順に並べてツリーにする。
func build(nodes ...*Node) *Tree {
	for i, n := range nodes {
		n.CreatedAt = t0.Add(time.Duration(i) * time.Minute)
	}
	return BuildTree(nodes)
}

func leaf(id, parent string, s Status) *Node {
	return &Node{ID: id, Parent: parent, Title: id, Status: s}
}

func inner(id, parent string, c Combine) *Node {
	return &Node{ID: id, Parent: parent, Title: id, Combine: c}
}

func pendingIDs(r *Result) string {
	ids := make([]string, len(r.Pending))
	for i, n := range r.Pending {
		ids[i] = n.ID
	}
	return strings.Join(ids, ",")
}

func TestConcludeLeaf(t *testing.T) {
	for _, s := range Statuses {
		t.Run("末端がstatus "+string(s)+"のとき、結論はその三値に対応する", func(t *testing.T) {
			n := leaf("n", "", s)
			r := Conclude(build(n), n)
			if got := r.Conclusion(); got != tvConclusion[statusTV[s]] {
				t.Errorf("conclusion = %s", got)
			}
			if wantPending := statusTV[s] == tvU; (len(r.Pending) == 1) != wantPending {
				t.Errorf("pending = %q", pendingIDs(r))
			}
		})
	}
}

func TestConcludeAndOr(t *testing.T) {
	for _, c := range []struct {
		combine Combine
		table   map[[2]tv]tv
	}{{CombineAnd, andTable}, {CombineOr, orTable}} {
		// 2 つの子の status の組み合わせ 25 通りをすべて回す。
		for _, s1 := range Statuses {
			for _, s2 := range Statuses {
				name := string(c.combine) + "の子が" + string(s1) + "と" + string(s2) + "のとき、結論は三値論理の真理値表に従う"
				t.Run(name, func(t *testing.T) {
					p := inner("p", "", c.combine)
					a, b := leaf("a", "p", s1), leaf("b", "p", s2)
					r := Conclude(build(p, a, b), p)
					want := c.table[[2]tv{statusTV[s1], statusTV[s2]}]
					if got := r.Conclusion(); got != tvConclusion[want] {
						t.Errorf("conclusion = %s, want %s", got, tvConclusion[want])
					}
					// 結論が確定したら未確定の末端は挙げず、確定しなければ未確定の末端をすべて挙げる。
					var wantPending []string
					if want == tvU {
						for _, n := range []*Node{a, b} {
							if statusTV[n.Status] == tvU {
								wantPending = append(wantPending, n.ID)
							}
						}
					}
					if got := pendingIDs(r); got != strings.Join(wantPending, ",") {
						t.Errorf("pending = %q, want %q", got, strings.Join(wantPending, ","))
					}
				})
			}
		}
	}
}

func TestConcludeCases(t *testing.T) {
	t.Run("場合分けの結果は、条件をかつ・またはで結んだ条件付きの結論になる", func(t *testing.T) {
		// A [and]
		// ├─ A-1 supported
		// ├─ A-2 [cases]: スマホ登録 → supported, PC登録 → rejected
		// └─ A-3 [cases]: 動画あり → supported, 動画なし → testing
		a := inner("A", "", CombineAnd)
		a1 := leaf("A1", "A", StatusSupported)
		a2 := inner("A2", "A", CombineCases)
		a21, a22 := leaf("A21", "A2", StatusSupported), leaf("A22", "A2", StatusRejected)
		a21.Condition, a22.Condition = "スマホ登録", "PC登録"
		a3 := inner("A3", "A", CombineCases)
		a31, a32 := leaf("A31", "A3", StatusSupported), leaf("A32", "A3", StatusTesting)
		a31.Condition, a32.Condition = "動画あり", "動画なし"
		r := Conclude(build(a, a1, a2, a21, a22, a3, a31, a32), a)
		if r.Conclusion() != ConclusionConditional {
			t.Errorf("conclusion = %s", r.Conclusion())
		}
		if got := r.SupportedWhen.String(); got != `"スマホ登録" and "動画あり"` {
			t.Errorf("supported when %s", got)
		}
		if got := r.RejectedWhen.String(); got != `"PC登録"` {
			t.Errorf("rejected when %s", got)
		}
		if got := pendingIDs(r); got != "A32" {
			t.Errorf("pending = %q", got)
		}
		if len(r.Warnings) != 0 {
			t.Errorf("warnings = %v", r.Warnings)
		}
	})
	t.Run("すべての場合で支持されたら条件なしの支持になる", func(t *testing.T) {
		p := inner("p", "", CombineCases)
		a, b := leaf("a", "p", StatusSupported), leaf("b", "p", StatusSupported)
		a.Condition, b.Condition = "x", "y"
		if got := Conclude(build(p, a, b), p).Conclusion(); got != ConclusionSupported {
			t.Errorf("conclusion = %s", got)
		}
	})
	t.Run("すべての場合で棄却されたら条件なしの棄却になる", func(t *testing.T) {
		p := inner("p", "", CombineCases)
		a, b := leaf("a", "p", StatusRejected), leaf("b", "p", StatusRejected)
		a.Condition, b.Condition = "x", "y"
		if got := Conclude(build(p, a, b), p).Conclusion(); got != ConclusionRejected {
			t.Errorf("conclusion = %s", got)
		}
	})
	t.Run("棄却される条件だけが分かっていても条件付きの結論になる", func(t *testing.T) {
		p := inner("p", "", CombineCases)
		a, b := leaf("a", "p", StatusRejected), leaf("b", "p", StatusTesting)
		a.Condition, b.Condition = "x", "y"
		r := Conclude(build(p, a, b), p)
		if r.Conclusion() != ConclusionConditional || r.SupportedWhen.String() != "false" || r.RejectedWhen.String() != `"x"` {
			t.Errorf("conclusion = %s, supported when %s, rejected when %s", r.Conclusion(), r.SupportedWhen, r.RejectedWhen)
		}
	})
	t.Run("条件のない場合は未確定として扱い警告する", func(t *testing.T) {
		p := inner("p", "", CombineCases)
		a, b := leaf("a", "p", StatusSupported), leaf("b", "p", StatusSupported)
		a.Condition = "x"
		r := Conclude(build(p, a, b), p)
		if r.Conclusion() != ConclusionConditional || r.SupportedWhen.String() != `"x"` {
			t.Errorf("conclusion = %s, supported when %s", r.Conclusion(), r.SupportedWhen)
		}
		if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "no condition") {
			t.Errorf("warnings = %v", r.Warnings)
		}
	})
}

func TestConcludeWarnings(t *testing.T) {
	t.Run("combineが未設定なら未確定として警告する", func(t *testing.T) {
		p := inner("p", "", "")
		a := leaf("a", "p", StatusSupported)
		r := Conclude(build(p, a), p)
		if r.Conclusion() != ConclusionUndetermined || len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "combine is not set") {
			t.Errorf("conclusion = %s, warnings = %v", r.Conclusion(), r.Warnings)
		}
	})
	t.Run("親がcasesでないのにconditionを持つ子は、条件を無視して警告する", func(t *testing.T) {
		p := inner("p", "", CombineAnd)
		a := leaf("a", "p", StatusSupported)
		a.Condition = "x"
		r := Conclude(build(p, a), p)
		if r.Conclusion() != ConclusionSupported || len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "ignored") {
			t.Errorf("conclusion = %s, warnings = %v", r.Conclusion(), r.Warnings)
		}
	})
	t.Run("子孫の警告も根まで伝える", func(t *testing.T) {
		p := inner("p", "", CombineAnd)
		q := inner("q", "p", "")
		a := leaf("a", "q", StatusSupported)
		if r := Conclude(build(p, q, a), p); len(r.Warnings) != 1 {
			t.Errorf("warnings = %v", r.Warnings)
		}
	})
}

func TestConcludeNested(t *testing.T) {
	// P [or]
	// ├─ X [and]: A supported, B testing
	// └─ C (status は各ケースで変える)
	setup := func(c Status) (*Tree, *Node) {
		p := inner("P", "", CombineOr)
		x := inner("X", "P", CombineAnd)
		return build(p, x, leaf("A", "X", StatusSupported), leaf("B", "X", StatusTesting), leaf("C", "P", c)), p
	}
	t.Run("(A and B) or C で C が棄却なら、B が確定するまで未確定でありBだけが残る", func(t *testing.T) {
		tree, p := setup(StatusRejected)
		r := Conclude(tree, p)
		if r.Conclusion() != ConclusionUndetermined || pendingIDs(r) != "B" {
			t.Errorf("conclusion = %s, pending = %q", r.Conclusion(), pendingIDs(r))
		}
	})
	t.Run("(A and B) or C で C が支持なら、B によらず支持になる", func(t *testing.T) {
		tree, p := setup(StatusSupported)
		r := Conclude(tree, p)
		if r.Conclusion() != ConclusionSupported || pendingIDs(r) != "" {
			t.Errorf("conclusion = %s, pending = %q", r.Conclusion(), pendingIDs(r))
		}
	})
}

func TestFormula(t *testing.T) {
	a, b, c := Atom("a"), Atom("b"), Atom("c")
	tests := []struct {
		name string
		f    *Formula
		want string
	}{
		{"andの中のorは括弧でくくる", And(Or(a, b), c), `("a" or "b") and "c"`},
		{"orの中のandは括弧でくくる", Or(And(a, b), c), `("a" and "b") or "c"`},
		{"同じ演算子の入れ子は平坦にする", And(a, And(b, c)), `"a" and "b" and "c"`},
		{"同じ条件の重複は1つにする", Or(a, a), `"a"`},
		{"andにfalseがあればfalse", And(a, False), "false"},
		{"orにtrueがあればtrue", Or(a, True), "true"},
		{"andのtrueは取り除く", And(True, a), `"a"`},
		{"orのfalseは取り除く", Or(False, a), `"a"`},
		{"空のandはtrue", And(), "true"},
		{"空のorはfalse", Or(), "false"},
		{"引用符を含む条件はエスケープする", Atom(`x "y"`), `"x \"y\""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.String(); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}
