package research

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// checkFixture は問題のない研究ディレクトリを作る。
//
//	P [and]
//	├─ C1 supported（承認済みの根拠 E を supports で紐づけ）
//	└─ C2 untested
type checkFixture struct {
	repo       *Repo
	p, c1, c2  *Node
	ev         *Evidence
	attachment string
}

func newCheckFixture(t *testing.T) *checkFixture {
	t.Helper()
	r := &Repo{Root: t.TempDir()}
	src := filepath.Join(t.TempDir(), "data.csv")
	os.WriteFile(src, []byte("hello"), 0o644)
	ev := &Evidence{Summary: "s", FoundBy: "ai:claude", CreatedAt: t0}
	if _, err := r.CreateEvidence(ev, []string{src}); err != nil {
		t.Fatal(err)
	}
	files, err := r.HashEvidenceFiles(ev)
	if err != nil {
		t.Fatal(err)
	}
	if err := Approve(ev, "human:tester", files, t0); err != nil {
		t.Fatal(err)
	}
	f := &checkFixture{
		repo:       r,
		p:          &Node{ID: "aaaaaaa0", Title: "P", Combine: CombineAnd, CreatedAt: t0},
		c1:         &Node{ID: "aaaaaaa1", Parent: "aaaaaaa0", Title: "C1", Status: StatusSupported, CreatedAt: t0, Evidence: []EvidenceLink{{ID: ev.ID, Stance: StanceSupports}}},
		c2:         &Node{ID: "aaaaaaa2", Parent: "aaaaaaa0", Title: "C2", Status: StatusUntested, CreatedAt: t0},
		ev:         ev,
		attachment: filepath.Join(r.Root, "evidence", ev.ID, "data.csv"),
	}
	f.save(t)
	return f
}

func (f *checkFixture) save(t *testing.T) {
	t.Helper()
	for _, n := range []*Node{f.p, f.c1, f.c2} {
		if _, err := f.repo.SaveNode(n); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.repo.SaveEvidence(f.ev); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheck(t *testing.T) {
	t.Run("問題のない研究ディレクトリでは何も報告しない", func(t *testing.T) {
		f := newCheckFixture(t)
		if issues := f.repo.Check(); len(issues) != 0 {
			t.Errorf("issues = %v", issues)
		}
	})

	tests := []struct {
		name     string
		mutate   func(t *testing.T, f *checkFixture)
		severity Severity
		want     string
	}{
		// 仮説ファイルの読み込み
		{"定義にないフィールドはerror", func(t *testing.T, f *checkFixture) {
			writeFile(t, filepath.Join(f.repo.Root, "hypotheses", "aaaaaaa2.yaml"), "id: aaaaaaa2\nparent: aaaaaaa0\ntitle: C2\nstauts: untested\ncreated_at: 2026-10-07T00:00:00Z\n")
		}, SeverityError, "stauts"},
		{"ファイル名とIDの食い違いはerror", func(t *testing.T, f *checkFixture) {
			os.Rename(filepath.Join(f.repo.Root, "hypotheses", "aaaaaaa2.yaml"), filepath.Join(f.repo.Root, "hypotheses", "bbbbbbbb.yaml"))
		}, SeverityError, "does not match the file name"},
		{"仮説ディレクトリのyaml以外のファイルはwarning", func(t *testing.T, f *checkFixture) {
			writeFile(t, filepath.Join(f.repo.Root, "hypotheses", "memo.txt"), "x")
		}, SeverityWarning, "not a hypothesis file"},
		// 仮説の内容
		{"形式に合わないIDはerror", func(t *testing.T, f *checkFixture) {
			f.c2.ID = "C2"
			os.Remove(filepath.Join(f.repo.Root, "hypotheses", "aaaaaaa2.yaml"))
			f.save(t)
		}, SeverityError, "invalid id"},
		{"空のタイトルはerror", func(t *testing.T, f *checkFixture) { f.c2.Title = ""; f.save(t) }, SeverityError, "title is empty"},
		{"作成日時の欠落はerror", func(t *testing.T, f *checkFixture) { f.c2.CreatedAt = time.Time{}; f.save(t) }, SeverityError, "created_at"},
		{"存在しない親はerror", func(t *testing.T, f *checkFixture) { f.c2.Parent = "ffffffff"; f.save(t) }, SeverityError, "parent ffffffff not found"},
		{"親子関係の循環はerror", func(t *testing.T, f *checkFixture) { f.c2.Parent = f.c2.ID; f.save(t) }, SeverityError, "cycle"}, // 循環に含まれるノードごとに報告するので、1 つで循環させる
		{"子を持つノードのstatusはerror", func(t *testing.T, f *checkFixture) { f.p.Status = StatusSupported; f.save(t) }, SeverityError, "status \"supported\" is set"},
		{"末端の定義にないstatusはerror", func(t *testing.T, f *checkFixture) { f.c2.Status = "done"; f.save(t) }, SeverityError, "invalid status"},
		{"末端のstatusの欠落はerror", func(t *testing.T, f *checkFixture) { f.c2.Status = ""; f.save(t) }, SeverityError, "invalid status"},
		{"子を持つノードのcombine未設定はwarning", func(t *testing.T, f *checkFixture) { f.p.Combine = ""; f.save(t) }, SeverityWarning, "combine is not set"},
		{"定義にないcombineはerror", func(t *testing.T, f *checkFixture) { f.p.Combine = "xor"; f.save(t) }, SeverityError, "invalid combine"},
		{"cases以外の親の下のconditionはwarning", func(t *testing.T, f *checkFixture) { f.c2.Condition = "x"; f.save(t) }, SeverityWarning, "condition is ignored"},
		{"条件のない場合はwarning", func(t *testing.T, f *checkFixture) { f.p.Combine = CombineCases; f.c1.Condition = "x"; f.save(t) }, SeverityWarning, "case has no condition"},
		{"存在しない根拠への紐づけはerror", func(t *testing.T, f *checkFixture) {
			f.c2.Evidence = []EvidenceLink{{ID: "ffffffff", Stance: StanceSupports}}
			f.save(t)
		}, SeverityError, "linked evidence ffffffff not found"},
		{"同じ根拠の重複した紐づけはerror", func(t *testing.T, f *checkFixture) {
			f.c1.Evidence = append(f.c1.Evidence, EvidenceLink{ID: f.ev.ID, Stance: StanceRefutes})
			f.save(t)
		}, SeverityError, "more than once"},
		{"定義にないstanceはerror", func(t *testing.T, f *checkFixture) { f.c1.Evidence[0].Stance = "support"; f.save(t) }, SeverityError, "invalid stance"},
		{"正でない重みはerror", func(t *testing.T, f *checkFixture) { neg, pos := -1.0, 1.0; f.c1.Weight, f.c2.Weight = &neg, &pos; f.save(t) }, SeverityError, "weight must be a positive"},
		{"ルートの重みはwarning", func(t *testing.T, f *checkFixture) { w := 1.0; f.p.Weight = &w; f.save(t) }, SeverityWarning, "ignored on a root"},
		{"一部の子だけが重みを持つとwarning", func(t *testing.T, f *checkFixture) { w := 1.0; f.c1.Weight = &w; f.save(t) }, SeverityWarning, "only 1 of 2 children"},
		{"末端の軸はwarning", func(t *testing.T, f *checkFixture) { f.c1.Axis = "x"; f.save(t) }, SeverityWarning, "axis is set"},
		// 根拠
		{"どこにも紐づいていない根拠はwarning", func(t *testing.T, f *checkFixture) { f.c1.Evidence = nil; f.save(t) }, SeverityWarning, "not linked"},
		{"根拠のディレクトリにある記録外のファイルはwarning", func(t *testing.T, f *checkFixture) {
			writeFile(t, filepath.Join(f.repo.Root, "evidence", f.ev.ID, "memo.txt"), "x")
		}, SeverityWarning, "not recorded"},
		{"根拠ディレクトリ直下のファイルはwarning", func(t *testing.T, f *checkFixture) {
			writeFile(t, filepath.Join(f.repo.Root, "evidence", "memo.txt"), "x")
		}, SeverityWarning, "not an evidence directory"},
		{"根拠のディレクトリ名とIDの食い違いはerror", func(t *testing.T, f *checkFixture) {
			os.Rename(filepath.Join(f.repo.Root, "evidence", f.ev.ID), filepath.Join(f.repo.Root, "evidence", "bbbbbbbb"))
			f.c1.Evidence = nil
			f.save(t)
			os.RemoveAll(filepath.Join(f.repo.Root, "evidence", f.ev.ID))
		}, SeverityError, "does not match the directory name"},
		{"承認後に添付ファイルが変わればerror", func(t *testing.T, f *checkFixture) { writeFile(t, f.attachment, "edited") }, SeverityError, "changed since approved"},
		{"承認後に本文が変わればerror", func(t *testing.T, f *checkFixture) { f.ev.Summary = "edited"; f.save(t) }, SeverityError, "changed since approved"},
		{"承認後に添付ファイルが消えればerror", func(t *testing.T, f *checkFixture) { os.Remove(f.attachment) }, SeverityError, "cannot be read"},
		{"AIによる承認はerror", func(t *testing.T, f *checkFixture) { f.ev.ApprovedBy = "ai:claude"; f.save(t) }, SeverityError, "only be approved by a human"},
		{"fingerprintのない承認者はerror", func(t *testing.T, f *checkFixture) { f.ev.ApprovedFingerprint = ""; f.save(t) }, SeverityError, "without approved_fingerprint"},
		{"未承認の根拠の添付ファイルが記録後に変わればwarning", func(t *testing.T, f *checkFixture) {
			f.ev.ApprovedBy, f.ev.ApprovedAt, f.ev.ApprovedFingerprint = "", nil, ""
			f.save(t)
			writeFile(t, f.attachment, "edited")
		}, SeverityWarning, "changed since it was recorded"},
		{"根拠の空の要約はerror", func(t *testing.T, f *checkFixture) {
			f.ev.Summary, f.ev.ApprovedBy, f.ev.ApprovedAt, f.ev.ApprovedFingerprint = "", "", nil, ""
			f.save(t)
		}, SeverityError, "summary is empty"},
		{"根拠の作成日時の欠落はerror", func(t *testing.T, f *checkFixture) { f.ev.CreatedAt = time.Time{}; f.save(t) }, SeverityError, "created_at"},
		{"形式に合わない根拠のIDはerror", func(t *testing.T, f *checkFixture) {
			old := f.ev.ID
			os.Rename(filepath.Join(f.repo.Root, "evidence", old), filepath.Join(f.repo.Root, "evidence", "BAD"))
			f.ev.ID, f.c1.Evidence[0].ID = "BAD", "BAD"
			f.save(t)
		}, SeverityError, "invalid id"},
		{"根拠のfound_byの欠落はerror", func(t *testing.T, f *checkFixture) { f.ev.FoundBy = ""; f.save(t) }, SeverityError, "found_by is empty"},
	}
	t.Run("承認済みの根拠の添付が変わったときは、記録後の変更としては重ねて報告しない", func(t *testing.T) {
		f := newCheckFixture(t)
		writeFile(t, f.attachment, "edited")
		for _, i := range f.repo.Check() {
			if strings.Contains(i.Message, "changed since it was recorded") {
				t.Errorf("unexpected issue: %v", i)
			}
		}
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCheckFixture(t)
			tt.mutate(t, f)
			var matched []Issue
			for _, i := range f.repo.Check() {
				if strings.Contains(i.Message, tt.want) {
					matched = append(matched, i)
				}
			}
			if len(matched) != 1 || matched[0].Severity != tt.severity {
				t.Errorf("issues matching %q = %v, want exactly one %s; all = %v", tt.want, matched, tt.severity, f.repo.Check())
			}
		})
	}
}
