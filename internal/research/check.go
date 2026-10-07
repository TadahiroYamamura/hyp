package research

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue は check が見つけた問題。Path は研究ディレクトリからの相対パス。
type Issue struct {
	Severity Severity `json:"severity"`
	Path     string   `json:"path"`
	Message  string   `json:"message"`
}

func (i Issue) Error() string { return i.Path + ": " + i.Message }

func errorIssue(path, msg string) Issue   { return Issue{SeverityError, path, msg} }
func warningIssue(path, msg string) Issue { return Issue{SeverityWarning, path, msg} }

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}$`)

// Check は研究ディレクトリのファイルの整合性を調べる。git の状態は調べない（呼び出し元が調べる）。
//
// error はデータが壊れているか、承認後に内容が変わったもの。warning は導出や運用に支障が出うるが、
// データとしては成立しているもの。
func (r *Repo) Check() []Issue {
	nodes, issues := r.loadNodes()
	evs, evIssues := r.loadEvidence()
	issues = append(issues, evIssues...)

	tree := BuildTree(nodes)
	evByID := map[string]*Evidence{}
	for _, ev := range evs {
		evByID[ev.ID] = ev
	}
	linked := map[string]bool{}
	for _, n := range nodes {
		issues = append(issues, checkNode(r, tree, n, evByID)...)
		for _, l := range n.Evidence {
			linked[l.ID] = true
		}
	}
	for _, ev := range evs {
		issues = append(issues, checkEvidence(r, ev)...)
		if !linked[ev.ID] {
			issues = append(issues, warningIssue(filepath.Join(evidenceDir, ev.ID), "not linked from any hypothesis"))
		}
	}
	return issues
}

func checkNode(r *Repo, t *Tree, n *Node, evByID map[string]*Evidence) []Issue {
	path := r.NodePath(n.ID)
	var issues []Issue
	add := func(sev Severity, format string, args ...any) {
		issues = append(issues, Issue{sev, path, fmt.Sprintf(format, args...)})
	}
	if !idPattern.MatchString(n.ID) {
		add(SeverityError, "invalid id %q", n.ID)
	}
	if n.Title == "" {
		add(SeverityError, "title is empty")
	}
	if n.CreatedAt.IsZero() {
		add(SeverityError, "created_at is missing")
	}
	if n.Parent != "" {
		if _, ok := t.Node(n.Parent); !ok {
			add(SeverityError, "parent %s not found", n.Parent)
		} else if t.Label(n.ID) == "" {
			add(SeverityError, "parent chain forms a cycle")
		}
	}
	if t.HasChildren(n.ID) {
		if n.Status != "" {
			add(SeverityError, "status %q is set on a hypothesis with children", n.Status)
		}
		if n.Combine == "" {
			add(SeverityWarning, "combine is not set, so the conclusion cannot be derived")
		} else if _, err := ParseCombine(string(n.Combine)); err != nil {
			add(SeverityError, "%v", err)
		}
	} else if _, err := ParseStatus(string(n.Status)); err != nil {
		add(SeverityError, "%v", err)
	}
	if p, ok := t.Node(n.Parent); ok && p.Combine == CombineCases && n.Condition == "" {
		add(SeverityWarning, "case has no condition")
	} else if n.Condition != "" && (!ok || p.Combine != CombineCases) {
		add(SeverityWarning, "condition is ignored because the parent's combine is not cases")
	}
	if n.Weight != nil {
		if !(*n.Weight > 0) || math.IsInf(*n.Weight, 0) {
			add(SeverityError, "weight must be a positive number")
		}
		if n.Parent == "" {
			add(SeverityWarning, "weight is ignored on a root")
		}
	}
	if n.Axis != "" && !t.HasChildren(n.ID) {
		add(SeverityWarning, "axis is set on a hypothesis without children")
	}
	if cs := t.Children(n.ID); len(cs) > 0 {
		set := 0
		for _, c := range cs {
			if c.Weight != nil {
				set++
			}
		}
		if set > 0 && set < len(cs) {
			add(SeverityWarning, "only %d of %d children have a weight; the others are treated as the average", set, len(cs))
		}
	}
	seen := map[string]bool{}
	for _, l := range n.Evidence {
		if _, ok := evByID[l.ID]; !ok {
			add(SeverityError, "linked evidence %s not found", l.ID)
		}
		if seen[l.ID] {
			add(SeverityError, "evidence %s is linked more than once", l.ID)
		}
		seen[l.ID] = true
		if _, err := ParseStance(string(l.Stance)); err != nil {
			add(SeverityError, "evidence %s: %v", l.ID, err)
		}
	}
	return issues
}

func checkEvidence(r *Repo, ev *Evidence) []Issue {
	dir := filepath.Join(evidenceDir, ev.ID)
	path := r.EvidencePath(ev.ID)
	var issues []Issue
	add := func(sev Severity, format string, args ...any) {
		issues = append(issues, Issue{sev, path, fmt.Sprintf(format, args...)})
	}
	if !idPattern.MatchString(ev.ID) {
		add(SeverityError, "invalid id %q", ev.ID)
	}
	if ev.Summary == "" {
		add(SeverityError, "summary is empty")
	}
	if ev.FoundBy == "" {
		add(SeverityError, "found_by is empty")
	}
	if ev.CreatedAt.IsZero() {
		add(SeverityError, "created_at is missing")
	}

	known := map[string]bool{evidenceFile: true}
	current := make([]EvidenceFile, 0, len(ev.Files))
	readable := true
	for _, f := range ev.Files {
		known[f.Path] = true
		h, err := HashFile(filepath.Join(r.Root, dir, f.Path))
		if err != nil {
			add(SeverityError, "attached file %s cannot be read: %v", f.Path, err)
			readable = false
			continue
		}
		current = append(current, EvidenceFile{Path: f.Path, SHA256: h})
		// 承認済みなら下の fingerprint の判定で報告されるので、未承認のときだけ報告する。
		if h != f.SHA256 && ev.ApprovedFingerprint == "" {
			add(SeverityWarning, "attached file %s has changed since it was recorded", f.Path)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(r.Root, dir)); err == nil {
		for _, e := range entries {
			if !known[e.Name()] {
				issues = append(issues, warningIssue(filepath.Join(dir, e.Name()), "file is not recorded in the evidence"))
			}
		}
	}

	switch {
	case ev.ApprovedFingerprint == "" && ev.ApprovedBy != "":
		add(SeverityError, "approved_by is set without approved_fingerprint")
	case ev.ApprovedFingerprint == "":
	case ev.ApprovedBy == "" || IsAI(ev.ApprovedBy):
		add(SeverityError, "approved by %q, but evidence can only be approved by a human", ev.ApprovedBy)
	case !readable || Approval(ev, current) == ApprovalOutdated:
		add(SeverityError, "changed since approved by %s; review and approve it again", ev.ApprovedBy)
	}
	return issues
}
