package research

import (
	"fmt"
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
type Result struct {
	SupportedWhen *Formula
	RejectedWhen  *Formula
	Pending       []*Node
	Warnings      []string
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
// 未確定（untested / testing / on-hold）の末端の扱いは Kleene の三値論理と同じで、
// and の子に rejected が一つあれば、ほかの子が未確定でも rejected に確定する。
func Conclude(t *Tree, n *Node) *Result {
	children := t.Children(n.ID)
	if len(children) == 0 {
		switch n.Status {
		case StatusSupported:
			return &Result{SupportedWhen: True, RejectedWhen: False}
		case StatusRejected:
			return &Result{SupportedWhen: False, RejectedWhen: True}
		}
		return &Result{SupportedWhen: False, RejectedWhen: False, Pending: []*Node{n}}
	}

	res := &Result{}
	subs := make([]*Result, len(children))
	for i, c := range children {
		subs[i] = Conclude(t, c)
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
	if !res.SupportedWhen.IsTrue() && !res.RejectedWhen.IsTrue() {
		for _, s := range subs {
			res.Pending = append(res.Pending, s.Pending...)
		}
	}
	return res
}
