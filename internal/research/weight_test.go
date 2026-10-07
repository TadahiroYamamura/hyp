package research

import (
	"math"
	"testing"
)

func weighted(id, parent string, s Status, w float64) *Node {
	n := leaf(id, parent, s)
	n.Weight = &w
	return n
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestShare(t *testing.T) {
	t.Run("兄弟の重みの合計が1になるよう正規化する", func(t *testing.T) {
		p := inner("p", "", CombineAnd)
		tree := build(p, weighted("a", "p", StatusUntested, 3), weighted("b", "p", StatusUntested, 1))
		if !near(tree.Share("a"), 0.75) || !near(tree.Share("b"), 0.25) {
			t.Errorf("a=%v b=%v", tree.Share("a"), tree.Share("b"))
		}
	})
	t.Run("兄弟のだれも重みを持たなければ均等にする", func(t *testing.T) {
		p := inner("p", "", CombineAnd)
		tree := build(p, leaf("a", "p", StatusUntested), leaf("b", "p", StatusUntested), leaf("c", "p", StatusUntested))
		if !near(tree.Share("b"), 1.0/3) {
			t.Errorf("b=%v", tree.Share("b"))
		}
	})
	t.Run("重みのない兄弟は重みのある兄弟の平均として扱う", func(t *testing.T) {
		// a=2, c=4 の平均 3 を b に当てると合計 9。
		p := inner("p", "", CombineAnd)
		tree := build(p, weighted("a", "p", StatusUntested, 2), leaf("b", "p", StatusUntested), weighted("c", "p", StatusUntested, 4))
		if !near(tree.Share("a"), 2.0/9) || !near(tree.Share("b"), 3.0/9) || !near(tree.Share("c"), 4.0/9) {
			t.Errorf("a=%v b=%v c=%v", tree.Share("a"), tree.Share("b"), tree.Share("c"))
		}
	})
}

func TestConcludeWeights(t *testing.T) {
	t.Run("進捗は判定済みの子の重みの合計で、残りの末端にはそれが決まったときの結論を添える", func(t *testing.T) {
		p := inner("p", "", CombineAnd)
		tree := build(p, weighted("a", "p", StatusSupported, 0.8), weighted("b", "p", StatusUntested, 0.2))
		r := Conclude(tree, p)
		if !near(r.Progress, 0.8) {
			t.Errorf("progress = %v", r.Progress)
		}
		if len(r.Pending) != 1 {
			t.Fatalf("pending = %v", pendingIDs(r))
		}
		got := r.Pending[0]
		if got.Node.ID != "b" || !near(got.Weight, 0.2) || got.IfSupported != ConclusionSupported || got.IfRejected != ConclusionRejected {
			t.Errorf("pending = %+v", got)
		}
	})
	t.Run("結論が確定した部分木の進捗は1とする", func(t *testing.T) {
		p := inner("p", "", CombineAnd)
		tree := build(p, weighted("a", "p", StatusRejected, 0.1), weighted("b", "p", StatusUntested, 0.9))
		if r := Conclude(tree, p); !near(r.Progress, 1) || len(r.Pending) != 0 {
			t.Errorf("progress = %v, pending = %q", r.Progress, pendingIDs(r))
		}
	})
	t.Run("入れ子では経路上の重みを掛け合わせ、重みの大きい末端から並べる", func(t *testing.T) {
		// P [and]: X [and](A supported, B untested), C untested。重みはすべて均等。
		// X の進捗 0.5、P の進捗 0.5*0.5 + 0.5*0 = 0.25。B の重み 0.5*0.5、C の重み 0.5。
		p := inner("P", "", CombineAnd)
		x := inner("X", "P", CombineAnd)
		tree := build(p, x, leaf("A", "X", StatusSupported), leaf("B", "X", StatusUntested), leaf("C", "P", StatusUntested))
		r := Conclude(tree, p)
		if !near(r.Progress, 0.25) || pendingIDs(r) != "C,B" {
			t.Fatalf("progress = %v, pending = %q", r.Progress, pendingIDs(r))
		}
		if !near(r.Pending[0].Weight, 0.5) || !near(r.Pending[1].Weight, 0.25) {
			t.Errorf("weights = %v, %v", r.Pending[0].Weight, r.Pending[1].Weight)
		}
		// C だけが支持されても B が残るので未確定、C だけが棄却されれば and なので棄却。
		if c := r.Pending[0]; c.IfSupported != ConclusionUndetermined || c.IfRejected != ConclusionRejected {
			t.Errorf("C: %+v", c)
		}
	})
	t.Run("casesでは支持・棄却された場合が占める割合を出す", func(t *testing.T) {
		p := inner("p", "", CombineCases)
		a, b, c := weighted("a", "p", StatusSupported, 0.7), weighted("b", "p", StatusSupported, 0.2), weighted("c", "p", StatusUntested, 0.1)
		a.Condition, b.Condition, c.Condition = "スマホ", "PC", "タブレット"
		r := Conclude(build(p, a, b, c), p)
		if r.SupportedShare == nil || !near(*r.SupportedShare, 0.9) || !near(*r.RejectedShare, 0) {
			t.Fatalf("shares = %v, %v", r.SupportedShare, r.RejectedShare)
		}
		if !near(r.Progress, 0.9) {
			t.Errorf("progress = %v", r.Progress)
		}
		if got := r.Pending[0]; got.IfSupported != ConclusionSupported || got.IfRejected != ConclusionConditional {
			t.Errorf("pending = %+v", got)
		}
	})
	t.Run("cases以外では割合を出さない", func(t *testing.T) {
		p := inner("p", "", CombineAnd)
		if r := Conclude(build(p, leaf("a", "p", StatusSupported)), p); r.SupportedShare != nil || r.RejectedShare != nil {
			t.Errorf("shares should be nil")
		}
	})
}

func TestParseWeight(t *testing.T) {
	for _, s := range []string{"0", "-1", "abc", "Inf", "NaN"} {
		t.Run("正の有限の数でない重み "+s+" は拒否する", func(t *testing.T) {
			if _, err := ParseWeight(s); err == nil {
				t.Fatal("want error")
			}
		})
	}
	t.Run("空文字列は重みを外す指定として受け付ける", func(t *testing.T) {
		if w, err := ParseWeight(""); err != nil || w != nil {
			t.Errorf("got %v, %v", w, err)
		}
	})
	t.Run("正の数を受け付ける", func(t *testing.T) {
		if w, err := ParseWeight("0.5"); err != nil || *w != 0.5 {
			t.Errorf("got %v, %v", w, err)
		}
	})
}

func TestApplyEditAxisWeight(t *testing.T) {
	setup := func() (*Tree, *Node, *Node) {
		p, c := inner("p", "", CombineAnd), leaf("c", "p", StatusUntested)
		return build(p, c), p, c
	}
	t.Run("子を持つノードには軸を設定できる", func(t *testing.T) {
		tree, p, _ := setup()
		if err := ApplyEdit(tree, p, Edit{Axis: ptr("工程")}); err != nil || p.Axis != "工程" {
			t.Errorf("axis = %q, %v", p.Axis, err)
		}
	})
	t.Run("末端には軸を設定できない", func(t *testing.T) {
		tree, _, c := setup()
		if err := ApplyEdit(tree, c, Edit{Axis: ptr("工程")}); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("親を持つノードには重みを設定できる", func(t *testing.T) {
		tree, _, c := setup()
		if err := ApplyEdit(tree, c, Edit{Weight: ptr("2")}); err != nil || c.Weight == nil || *c.Weight != 2 {
			t.Errorf("weight = %v, %v", c.Weight, err)
		}
	})
	t.Run("ルートには重みを設定できない", func(t *testing.T) {
		tree, p, _ := setup()
		if err := ApplyEdit(tree, p, Edit{Weight: ptr("2")}); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("空文字列で重みを外せる", func(t *testing.T) {
		tree, _, c := setup()
		w := 2.0
		c.Weight = &w
		if err := ApplyEdit(tree, c, Edit{Weight: ptr("")}); err != nil || c.Weight != nil {
			t.Errorf("weight = %v, %v", c.Weight, err)
		}
	})
}
