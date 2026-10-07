package research

import (
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)

func node(id, parent string, minutes int) *Node {
	return &Node{ID: id, Parent: parent, Title: id, Status: StatusUntested, CreatedAt: t0.Add(time.Duration(minutes) * time.Minute)}
}

func TestResolve(t *testing.T) {
	nodes := []*Node{node("a3f9c2e1", "", 0), node("a3f00000", "", 1), node("7b2d04aa", "", 2)}
	tests := []struct {
		name    string
		prefix  string
		want    string
		wantErr string
	}{
		{"一意に定まる前方一致でノードを解決する", "7b", "7b2d04aa", ""},
		{"前方一致が複数あるときは候補を挙げてエラーにする", "a3f", "", "ambiguous"},
		{"一致するノードがなければエラーにする", "ff", "", "no hypothesis"},
		{"空のIDはエラーにする", "", "", "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(nodes, tt.prefix)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ID != tt.want {
				t.Errorf("got %s, want %s", got.ID, tt.want)
			}
		})
	}
}

func TestRootLabel(t *testing.T) {
	tests := []struct {
		name string
		i    int
		want string
	}{
		{"最初のルートはA", 0, "A"},
		{"26番目のルートはZ", 25, "Z"},
		{"27番目のルートはAAに繰り上がる", 26, "AA"},
		{"52番目のルートはAZ", 51, "AZ"},
		{"53番目のルートはBA", 52, "BA"},
		{"702番目のルートはZZ", 701, "ZZ"},
		{"703番目のルートはAAAに繰り上がる", 702, "AAA"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rootLabel(tt.i); got != tt.want {
				t.Errorf("rootLabel(%d) = %s, want %s", tt.i, got, tt.want)
			}
		})
	}
}

func TestBuildTree(t *testing.T) {
	t.Run("兄弟を作成日時順に並べて番号を振る", func(t *testing.T) {
		tree := BuildTree([]*Node{
			node("r2", "", 1), node("r1", "", 0),
			node("c2", "r1", 5), node("c1", "r1", 3), node("g1", "c2", 6),
		})
		want := map[string]string{"r1": "A", "r2": "B", "c1": "A-1", "c2": "A-2", "g1": "A-2-1"}
		for id, label := range want {
			if got := tree.Label(id); got != label {
				t.Errorf("Label(%s) = %s, want %s", id, got, label)
			}
		}
	})
	t.Run("作成日時が同じ兄弟はID順に並べる", func(t *testing.T) {
		tree := BuildTree([]*Node{node("r", "", 0), node("b", "r", 1), node("a", "r", 1)})
		if tree.Label("a") != "A-1" || tree.Label("b") != "A-2" {
			t.Errorf("a=%s b=%s", tree.Label("a"), tree.Label("b"))
		}
	})
	t.Run("親が見つからないノードはルートとして表示する", func(t *testing.T) {
		tree := BuildTree([]*Node{node("r", "", 0), node("orphan", "missing", 1)})
		if got := tree.Label("orphan"); got != "B" {
			t.Errorf("Label(orphan) = %q, want B", got)
		}
	})
}

func ptr(s string) *string { return &s }

func TestApplyEdit(t *testing.T) {
	newTree := func() (*Tree, map[string]*Node) {
		ns := []*Node{node("p", "", 0), node("c", "p", 1)}
		ns[0].Status = ""
		m := map[string]*Node{}
		for _, n := range ns {
			m[n.ID] = n
		}
		return BuildTree(ns), m
	}
	tests := []struct {
		name    string
		target  string
		parent  Combine
		edit    Edit
		wantErr string
	}{
		{"末端のノードにはstatusを設定できる", "c", "", Edit{Status: ptr("supported")}, ""},
		{"子を持つノードにはstatusを設定できない", "p", "", Edit{Status: ptr("supported")}, "leaf"},
		{"定義にないstatusは拒否する", "c", "", Edit{Status: ptr("done")}, "invalid status"},
		{"子を持つノードにはcombineを設定できる", "p", "", Edit{Combine: ptr("or")}, ""},
		{"末端のノードにはcombineを設定できない", "c", "", Edit{Combine: ptr("and")}, "children"},
		{"末端のノードでもcombineを外す指定は受け付ける", "c", "", Edit{Combine: ptr("")}, ""},
		{"定義にないcombineは拒否する", "p", "", Edit{Combine: ptr("xor")}, "invalid combine"},
		{"親のcombineがcasesならconditionを設定できる", "c", CombineCases, Edit{Condition: ptr("スマホ登録")}, ""},
		{"親のcombineがcases以外ならconditionを設定できない", "c", CombineAnd, Edit{Condition: ptr("スマホ登録")}, "cases"},
		{"ルートにはconditionを設定できない", "p", "", Edit{Condition: ptr("スマホ登録")}, "cases"},
		{"conditionを消す指定は親のcombineによらず受け付ける", "c", CombineAnd, Edit{Condition: ptr("")}, ""},
		{"タイトルを空にはできない", "c", "", Edit{Title: ptr("")}, "title"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, m := newTree()
			m["p"].Combine = tt.parent
			err := ApplyEdit(tree, m[tt.target], tt.edit)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestNewNode(t *testing.T) {
	// 親の status が何であっても消すことを確かめるため、全 status を回す。
	for _, st := range Statuses {
		t.Run("親のstatusが"+string(st)+"でも、子を持ったらstatusを消す", func(t *testing.T) {
			p := node("p", "", 0)
			p.Status = st
			n, changed, err := NewNode(BuildTree([]*Node{p}), "p", "child", t0)
			if err != nil {
				t.Fatal(err)
			}
			if n.Status != StatusUntested || n.Parent != "p" {
				t.Errorf("new node = %+v", n)
			}
			if p.Status != "" || len(changed) != 1 {
				t.Errorf("parent status = %q, changed = %d; want \"\", 1", p.Status, len(changed))
			}
		})
	}
	t.Run("既に子を持つ親に追加しても親は変更しない", func(t *testing.T) {
		p, c := node("p", "", 0), node("c", "p", 1)
		p.Status = ""
		_, changed, err := NewNode(BuildTree([]*Node{p, c}), "p", "child", t0)
		if err != nil {
			t.Fatal(err)
		}
		if len(changed) != 0 {
			t.Errorf("changed = %d, want 0", len(changed))
		}
	})
	t.Run("存在しない親は拒否する", func(t *testing.T) {
		if _, _, err := NewNode(BuildTree(nil), "missing", "child", t0); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestMove(t *testing.T) {
	setup := func() (*Tree, map[string]*Node) {
		// a ─ a1 ─ a11
		//   └ a2
		// b （末端）
		ns := []*Node{node("a", "", 0), node("a1", "a", 1), node("a11", "a1", 2), node("a2", "a", 3), node("b", "", 4)}
		ns[0].Status, ns[1].Status = "", ""
		m := map[string]*Node{}
		for _, n := range ns {
			m[n.ID] = n
		}
		return BuildTree(ns), m
	}
	t.Run("自分の子孫の下には移動できない", func(t *testing.T) {
		tree, m := setup()
		if _, err := Move(tree, m["a"], "a11"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("自分自身の下には移動できない", func(t *testing.T) {
		tree, m := setup()
		if _, err := Move(tree, m["a"], "a"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("最後の子が抜けた親はuntestedの末端に戻り、移動先の未判定の末端はstatusを失う", func(t *testing.T) {
		tree, m := setup()
		changed, err := Move(tree, m["a11"], "b")
		if err != nil {
			t.Fatal(err)
		}
		if m["a1"].Status != StatusUntested || m["b"].Status != "" || m["a11"].Parent != "b" {
			t.Errorf("a1=%q b=%q a11.parent=%q", m["a1"].Status, m["b"].Status, m["a11"].Parent)
		}
		if len(changed) != 3 {
			t.Errorf("changed = %d, want 3 (a1, b, a11)", len(changed))
		}
	})
	t.Run("ルートへ移動でき、最後の子が抜けた親はuntestedの末端に戻る", func(t *testing.T) {
		tree, m := setup()
		changed, err := Move(tree, m["a11"], "")
		if err != nil {
			t.Fatal(err)
		}
		if m["a11"].Parent != "" || m["a1"].Status != StatusUntested || len(changed) != 2 {
			t.Errorf("a11.parent=%q a1=%q changed=%d", m["a11"].Parent, m["a1"].Status, len(changed))
		}
	})
	t.Run("ルートをルートへは移動できない", func(t *testing.T) {
		tree, m := setup()
		if _, err := Move(tree, m["a"], ""); err == nil || !strings.Contains(err.Error(), "already a root") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("今の親と同じ親へは移動できない", func(t *testing.T) {
		tree, m := setup()
		if _, err := Move(tree, m["a2"], "a"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("ほかに子が残る親のstatusは変えない", func(t *testing.T) {
		tree, m := setup()
		if _, err := Move(tree, m["a2"], "b"); err != nil {
			t.Fatal(err)
		}
		if m["a"].Status != "" {
			t.Errorf("a.Status = %q", m["a"].Status)
		}
	})
}

func TestRemove(t *testing.T) {
	t.Run("子を持つノードは削除できない", func(t *testing.T) {
		p, c := node("p", "", 0), node("c", "p", 1)
		if _, err := Remove(BuildTree([]*Node{p, c}), p); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("最後の子を削除した親はuntestedの末端に戻る", func(t *testing.T) {
		p, c := node("p", "", 0), node("c", "p", 1)
		p.Status = ""
		changed, err := Remove(BuildTree([]*Node{p, c}), c)
		if err != nil {
			t.Fatal(err)
		}
		if p.Status != StatusUntested || len(changed) != 1 {
			t.Errorf("p.Status = %q, changed = %d", p.Status, len(changed))
		}
	})
}
