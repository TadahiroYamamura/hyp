package research

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Formula は条件の論理式。条件は不透明な文字列として扱い、意味（排他・矛盾）は解釈しない。
type Formula struct {
	op   string // "true" / "false" / "atom" / "and" / "or"
	atom string
	args []*Formula
}

var (
	True  = &Formula{op: "true"}
	False = &Formula{op: "false"}
)

func Atom(s string) *Formula { return &Formula{op: "atom", atom: s} }

func (f *Formula) IsTrue() bool  { return f.op == "true" }
func (f *Formula) IsFalse() bool { return f.op == "false" }

// And と Or は、定数の吸収・入れ子の平坦化・重複の除去だけを行う。
// 条件の意味を解釈しないので、それ以上の簡約（矛盾の検出など）はしない。
func And(fs ...*Formula) *Formula { return combine("and", True, False, fs) }
func Or(fs ...*Formula) *Formula  { return combine("or", False, True, fs) }

func combine(op string, identity, absorbing *Formula, fs []*Formula) *Formula {
	var args []*Formula
	seen := map[string]bool{}
	var add func(f *Formula)
	add = func(f *Formula) {
		if f.op == op {
			for _, a := range f.args {
				add(a)
			}
			return
		}
		if key := f.String(); !seen[key] {
			seen[key] = true
			args = append(args, f)
		}
	}
	for _, f := range fs {
		if f.op == absorbing.op {
			return absorbing
		}
		if f.op != identity.op {
			add(f)
		}
	}
	switch len(args) {
	case 0:
		return identity
	case 1:
		return args[0]
	}
	return &Formula{op: op, args: args}
}

func (f *Formula) String() string {
	switch f.op {
	case "atom":
		return strconv.Quote(f.atom)
	case "and", "or":
		parts := make([]string, len(f.args))
		for i, a := range f.args {
			parts[i] = a.String()
			if a.op == "and" || a.op == "or" {
				parts[i] = "(" + parts[i] + ")"
			}
		}
		return strings.Join(parts, " "+f.op+" ")
	}
	return f.op
}

type Conclusion string

const (
	ConclusionSupported    Conclusion = "supported"
	ConclusionRejected     Conclusion = "rejected"
	ConclusionConditional  Conclusion = "conditional"
	ConclusionUndetermined Conclusion = "undetermined"
)

// Result は導出結果。SupportedWhen / RejectedWhen は、それぞれ仮説が支持・棄却される条件。
// Pending は結論を確定させるために判定が必要な末端、Warnings は導出できなかった箇所。
//
// Progress は結論のうち判定済みの割合で、子の重みで加重する。結論が確定した部分木は 1 とする。
// 仮説が正しい確率ではない（and の下で判定済みが 8 割でも、残りが棄却されれば全体が棄却される）。
//
// SupportedShare / RejectedShare は combine が cases のときだけ値を持ち、
// 支持・棄却された場合が占める割合を、場合の重みで表す。
type Result struct {
	SupportedWhen  *Formula
	RejectedWhen   *Formula
	Pending        []Pending
	Warnings       []string
	Progress       float64
	SupportedShare *float64
	RejectedShare  *float64
}

// Pending は未確定の末端。Weight は導出の対象にとっての重み（経路上の正規化した重みの積）。
// IfSupported / IfRejected は、その末端だけがそう判定されたときの対象の結論。
type Pending struct {
	Node        *Node
	Weight      float64
	IfSupported Conclusion
	IfRejected  Conclusion
}

func (r *Result) Conclusion() Conclusion {
	switch {
	case r.SupportedWhen.IsTrue():
		return ConclusionSupported
	case r.RejectedWhen.IsTrue():
		return ConclusionRejected
	case r.SupportedWhen.IsFalse() && r.RejectedWhen.IsFalse():
		return ConclusionUndetermined
	}
	return ConclusionConditional
}

// Conclude は末端の status から、子を持つノードの結論を導く。
// 未確定の末端は、対象にとっての重みが大きい順に並べ、それぞれが決まったときの結論を添える。
func Conclude(t *Tree, n *Node) *Result {
	res := conclude(t, n, nil)
	for i := range res.Pending {
		p := &res.Pending[i]
		p.IfSupported = conclude(t, n, map[string]Status{p.Node.ID: StatusSupported}).Conclusion()
		p.IfRejected = conclude(t, n, map[string]Status{p.Node.ID: StatusRejected}).Conclusion()
	}
	sort.SliceStable(res.Pending, func(i, j int) bool { return res.Pending[i].Weight > res.Pending[j].Weight })
	return res
}

// conclude は override に挙げた末端の status を置き換えて導出する。
// 未確定（untested / testing / on-hold）の末端の扱いは Kleene の三値論理と同じで、
// and の子に rejected が一つあれば、ほかの子が未確定でも rejected に確定する。
func conclude(t *Tree, n *Node, override map[string]Status) *Result {
	children := t.Children(n.ID)
	if len(children) == 0 {
		status := n.Status
		if s, ok := override[n.ID]; ok {
			status = s
		}
		switch status {
		case StatusSupported:
			return &Result{SupportedWhen: True, RejectedWhen: False, Progress: 1}
		case StatusRejected:
			return &Result{SupportedWhen: False, RejectedWhen: True, Progress: 1}
		}
		return &Result{SupportedWhen: False, RejectedWhen: False, Pending: []Pending{{Node: n, Weight: 1}}}
	}

	res := &Result{}
	subs := make([]*Result, len(children))
	for i, c := range children {
		subs[i] = conclude(t, c, override)
		res.Warnings = append(res.Warnings, subs[i].Warnings...)
		if c.Condition != "" && n.Combine != CombineCases {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s %s: condition is ignored because the parent's combine is not cases", t.Label(c.ID), c.ID))
		}
	}

	var sup, rej []*Formula
	switch n.Combine {
	case CombineAnd:
		for _, s := range subs {
			sup, rej = append(sup, s.SupportedWhen), append(rej, s.RejectedWhen)
		}
		res.SupportedWhen, res.RejectedWhen = And(sup...), Or(rej...)
	case CombineOr:
		for _, s := range subs {
			sup, rej = append(sup, s.SupportedWhen), append(rej, s.RejectedWhen)
		}
		res.SupportedWhen, res.RejectedWhen = Or(sup...), And(rej...)
	case CombineCases:
		// 場合分けは網羅的である（分解が正しい）前提に立つ。条件の意味は解釈できないので、
		// すべての場合で支持（棄却）されたときに限り、条件なしの支持（棄却）とみなす。
		allSup, allRej := true, true
		for i, c := range children {
			s := subs[i]
			allSup = allSup && s.SupportedWhen.IsTrue()
			allRej = allRej && s.RejectedWhen.IsTrue()
			if c.Condition == "" {
				res.Warnings = append(res.Warnings, fmt.Sprintf("%s %s: case has no condition and is treated as undetermined", t.Label(c.ID), c.ID))
				allSup, allRej = false, false
				continue
			}
			k := Atom(c.Condition)
			sup, rej = append(sup, And(k, s.SupportedWhen)), append(rej, And(k, s.RejectedWhen))
		}
		res.SupportedWhen, res.RejectedWhen = Or(sup...), Or(rej...)
		var supShare, rejShare float64
		for i, c := range children {
			if subs[i].SupportedWhen.IsTrue() {
				supShare += t.Share(c.ID)
			}
			if subs[i].RejectedWhen.IsTrue() {
				rejShare += t.Share(c.ID)
			}
		}
		res.SupportedShare, res.RejectedShare = &supShare, &rejShare
		if allSup {
			res.SupportedWhen = True
		}
		if allRej {
			res.RejectedWhen = True
		}
	default:
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s %s: combine is not set, so the conclusion cannot be derived", t.Label(n.ID), n.ID))
		res.SupportedWhen, res.RejectedWhen = False, False
	}

	// 結論が確定していれば、残りの未確定の末端は結論に影響しないので挙げない。
	if res.SupportedWhen.IsTrue() || res.RejectedWhen.IsTrue() {
		res.Progress = 1
		return res
	}
	for i, c := range children {
		share := t.Share(c.ID)
		res.Progress += share * subs[i].Progress
		for _, p := range subs[i].Pending {
			p.Weight *= share
			res.Pending = append(res.Pending, p)
		}
	}
	return res
}
