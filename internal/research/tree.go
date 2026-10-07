package research

import (
	"sort"
	"strconv"
)

type Tree struct {
	byID     map[string]*Node
	children map[string][]*Node
	roots    []*Node
	labels   map[string]string
}

// BuildTree は兄弟を作成日時順に並べ、表示用の番号（A, A-1, A-1-2）を振る。
// 親が見つからないノードは、表示から消えないようルートとして扱う。
func BuildTree(nodes []*Node) *Tree {
	t := &Tree{
		byID:     map[string]*Node{},
		children: map[string][]*Node{},
		labels:   map[string]string{},
	}
	for _, n := range nodes {
		t.byID[n.ID] = n
	}
	for _, n := range nodes {
		if _, ok := t.byID[n.Parent]; n.Parent != "" && ok {
			t.children[n.Parent] = append(t.children[n.Parent], n)
		} else {
			t.roots = append(t.roots, n)
		}
	}
	sortByCreated(t.roots)
	for _, cs := range t.children {
		sortByCreated(cs)
	}
	for i, r := range t.roots {
		t.assignLabels(r, rootLabel(i))
	}
	return t
}

func sortByCreated(ns []*Node) {
	// 作成日時が同じ場合でも表示順が揺れないよう ID で決める。
	sort.Slice(ns, func(i, j int) bool {
		if !ns[i].CreatedAt.Equal(ns[j].CreatedAt) {
			return ns[i].CreatedAt.Before(ns[j].CreatedAt)
		}
		return ns[i].ID < ns[j].ID
	})
}

func (t *Tree) assignLabels(n *Node, label string) {
	t.labels[n.ID] = label
	for i, c := range t.children[n.ID] {
		t.assignLabels(c, label+"-"+strconv.Itoa(i+1))
	}
}

// rootLabel は 0→A, 25→Z, 26→AA のようにスプレッドシートの列名と同じ規則で振る。
func rootLabel(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('A'+(i-1)%26)) + s
	}
	return s
}

func (t *Tree) Roots() []*Node               { return t.roots }
func (t *Tree) Children(id string) []*Node   { return t.children[id] }
func (t *Tree) Label(id string) string       { return t.labels[id] }
func (t *Tree) Node(id string) (*Node, bool) { n, ok := t.byID[id]; return n, ok }
func (t *Tree) HasChildren(id string) bool   { return len(t.children[id]) > 0 }

// IsDescendant は id が ancestor 自身またはその子孫かを返す。
func (t *Tree) IsDescendant(id, ancestor string) bool {
	if id == ancestor {
		return true
	}
	for _, c := range t.children[ancestor] {
		if t.IsDescendant(id, c.ID) {
			return true
		}
	}
	return false
}
