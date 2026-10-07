package research

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

type Stance string

const (
	StanceSupports     Stance = "supports"
	StanceRefutes      Stance = "refutes"
	StanceInconclusive Stance = "inconclusive"
)

var Stances = []Stance{StanceSupports, StanceRefutes, StanceInconclusive}

func ParseStance(s string) (Stance, error) {
	for _, v := range Stances {
		if string(v) == s {
			return v, nil
		}
	}
	return "", fmt.Errorf("invalid stance %q (want one of %s)", s, joinStrings(Stances))
}

// EvidenceLink は仮説から根拠への紐づけ。stance を根拠側に持たないのは、
// 同じ根拠がある仮説を支持し、別の仮説を反証することがあるため。
type EvidenceLink struct {
	ID     string `yaml:"id" json:"id"`
	Stance Stance `yaml:"stance" json:"stance"`
}

type EvidenceFile struct {
	Path   string `yaml:"path" json:"path"`
	SHA256 string `yaml:"sha256" json:"sha256"`
}

type Evidence struct {
	ID                  string         `yaml:"id" json:"id"`
	Summary             string         `yaml:"summary" json:"summary"`
	Source              string         `yaml:"source,omitempty" json:"source,omitempty"`
	Files               []EvidenceFile `yaml:"files,omitempty" json:"files,omitempty"`
	FoundBy             string         `yaml:"found_by" json:"found_by"`
	CreatedAt           time.Time      `yaml:"created_at" json:"created_at"`
	ApprovedBy          string         `yaml:"approved_by,omitempty" json:"approved_by,omitempty"`
	ApprovedAt          *time.Time     `yaml:"approved_at,omitempty" json:"approved_at,omitempty"`
	ApprovedFingerprint string         `yaml:"approved_fingerprint,omitempty" json:"approved_fingerprint,omitempty"`
}

// Fingerprint は承認の対象を表す値。添付だけでなく本文の書き換えも検知するため、
// summary・source・各添付ファイルの sha256 から計算する。
func Fingerprint(summary, source string, files []EvidenceFile) string {
	sorted := append([]EvidenceFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	// 区切り文字を含む値でも別の入力と衝突しないよう、JSON で符号化してからハッシュする。
	b, _ := json.Marshal(struct {
		Summary string         `json:"summary"`
		Source  string         `json:"source"`
		Files   []EvidenceFile `json:"files"`
	}{summary, source, sorted})
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// IsAI は主体が AI エージェントかを返す。主体は `ai:<モデル名>` / `human:<名前>` の形で表す。
func IsAI(actor string) bool {
	return strings.HasPrefix(actor, "ai:")
}

func ResolveEvidence(evs []*Evidence, prefix string) (*Evidence, error) {
	return resolveID(evs, func(e *Evidence) string { return e.ID }, prefix, "evidence")
}

// Link は仮説に根拠を紐づける。紐づけ済みなら stance だけを更新する。
func Link(n *Node, evidenceID string, stance Stance) error {
	for i, l := range n.Evidence {
		if l.ID != evidenceID {
			continue
		}
		if l.Stance == stance {
			return fmt.Errorf("evidence %s is already linked to %s as %s", evidenceID, n.ID, stance)
		}
		n.Evidence[i].Stance = stance
		return nil
	}
	n.Evidence = append(n.Evidence, EvidenceLink{ID: evidenceID, Stance: stance})
	return nil
}

// Approve は、承認者が確認した時点の内容として、files（添付の現在のハッシュ）から fingerprint を記録する。
// 承認は人間だけが行う。
func Approve(ev *Evidence, actor string, files []EvidenceFile, now time.Time) error {
	if IsAI(actor) {
		return fmt.Errorf("evidence can only be approved by a human (actor is %s)", actor)
	}
	ev.Files = files
	ev.ApprovedBy = actor
	ev.ApprovedAt = &now
	ev.ApprovedFingerprint = Fingerprint(ev.Summary, ev.Source, files)
	return nil
}

// Unlink は仮説から根拠への紐づけを外し、外した紐づけの stance を返す。
func Unlink(n *Node, evidenceID string) (Stance, error) {
	for i, l := range n.Evidence {
		if l.ID == evidenceID {
			n.Evidence = append(n.Evidence[:i], n.Evidence[i+1:]...)
			return l.Stance, nil
		}
	}
	return "", fmt.Errorf("evidence %s is not linked to %s", evidenceID, n.ID)
}

// LinkedFrom は根拠を紐づけている仮説を返す。
func LinkedFrom(nodes []*Node, evidenceID string) []*Node {
	var out []*Node
	for _, n := range nodes {
		for _, l := range n.Evidence {
			if l.ID == evidenceID {
				out = append(out, n)
				break
			}
		}
	}
	return out
}

type ApprovalState string

const (
	ApprovalNone     ApprovalState = "unapproved"
	ApprovalCurrent  ApprovalState = "approved"
	ApprovalOutdated ApprovalState = "changed-since-approval"
)

// Approval は、承認後に本文か添付が変わっていないかを、files（添付の現在のハッシュ）から判定する。
func Approval(ev *Evidence, files []EvidenceFile) ApprovalState {
	if ev.ApprovedFingerprint == "" {
		return ApprovalNone
	}
	if Fingerprint(ev.Summary, ev.Source, files) != ev.ApprovedFingerprint {
		return ApprovalOutdated
	}
	return ApprovalCurrent
}
