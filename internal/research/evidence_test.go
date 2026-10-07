package research

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sha256("hello") を外部の値として使う（`printf hello | sha256sum` の出力）。
const helloSHA256 = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

func TestParseStance(t *testing.T) {
	for _, s := range []string{"supports", "refutes", "inconclusive"} {
		t.Run(s+"を受け付ける", func(t *testing.T) {
			if got, err := ParseStance(s); err != nil || string(got) != s {
				t.Errorf("got %q, %v", got, err)
			}
		})
	}
	t.Run("定義にないstanceは拒否する", func(t *testing.T) {
		if _, err := ParseStance("support"); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestFingerprint(t *testing.T) {
	files := []EvidenceFile{{Path: "a.csv", SHA256: "aa"}, {Path: "b.py", SHA256: "bb"}}
	base := Fingerprint("summary", "https://example.com", files)
	tests := []struct {
		name    string
		summary string
		source  string
		files   []EvidenceFile
	}{
		{"summaryが変わればfingerprintも変わる", "summary2", "https://example.com", files},
		{"sourceが変わればfingerprintも変わる", "summary", "https://example.org", files},
		{"添付ファイルの中身が変わればfingerprintも変わる", "summary", "https://example.com", []EvidenceFile{{Path: "a.csv", SHA256: "xx"}, {Path: "b.py", SHA256: "bb"}}},
		{"添付ファイルが減ればfingerprintも変わる", "summary", "https://example.com", files[:1]},
		{"添付ファイル名が変わればfingerprintも変わる", "summary", "https://example.com", []EvidenceFile{{Path: "c.csv", SHA256: "aa"}, {Path: "b.py", SHA256: "bb"}}},
		{"値の境界をずらしても別のfingerprintになる", "summaryhttps://example.com", "", files},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Fingerprint(tt.summary, tt.source, tt.files); got == base {
				t.Errorf("fingerprint should differ from base")
			}
		})
	}
	t.Run("添付ファイルの並び順が違っても同じfingerprintになる", func(t *testing.T) {
		reversed := []EvidenceFile{files[1], files[0]}
		if got := Fingerprint("summary", "https://example.com", reversed); got != base {
			t.Errorf("got %s, want %s", got, base)
		}
	})
}

func TestLink(t *testing.T) {
	t.Run("未紐づけの根拠を紐づける", func(t *testing.T) {
		n := node("n", "", 0)
		if err := Link(n, "e1", StanceSupports); err != nil {
			t.Fatal(err)
		}
		if len(n.Evidence) != 1 || n.Evidence[0] != (EvidenceLink{ID: "e1", Stance: StanceSupports}) {
			t.Errorf("evidence = %+v", n.Evidence)
		}
	})
	t.Run("紐づけ済みの根拠を別のstanceで紐づけるとstanceだけを更新する", func(t *testing.T) {
		n := node("n", "", 0)
		n.Evidence = []EvidenceLink{{ID: "e0", Stance: StanceSupports}, {ID: "e1", Stance: StanceSupports}}
		if err := Link(n, "e1", StanceRefutes); err != nil {
			t.Fatal(err)
		}
		if len(n.Evidence) != 2 || n.Evidence[1].Stance != StanceRefutes || n.Evidence[0].Stance != StanceSupports {
			t.Errorf("evidence = %+v", n.Evidence)
		}
	})
	t.Run("同じstanceで紐づけ直すとエラーにする", func(t *testing.T) {
		n := node("n", "", 0)
		n.Evidence = []EvidenceLink{{ID: "e1", Stance: StanceSupports}}
		if err := Link(n, "e1", StanceSupports); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestApprove(t *testing.T) {
	files := []EvidenceFile{{Path: "a.csv", SHA256: "new"}}
	t.Run("人間の承認では承認者と承認時点のfingerprintを記録する", func(t *testing.T) {
		ev := &Evidence{Summary: "s", Source: "u", Files: []EvidenceFile{{Path: "a.csv", SHA256: "old"}}}
		if err := Approve(ev, "human:tester", files, t0); err != nil {
			t.Fatal(err)
		}
		if ev.ApprovedBy != "human:tester" || ev.ApprovedAt == nil || !ev.ApprovedAt.Equal(t0) {
			t.Errorf("approved = %q %v", ev.ApprovedBy, ev.ApprovedAt)
		}
		if ev.ApprovedFingerprint != Fingerprint("s", "u", files) || ev.Files[0].SHA256 != "new" {
			t.Errorf("fingerprint/files not taken from the current files: %+v", ev)
		}
	})
	t.Run("AIは承認できない", func(t *testing.T) {
		ev := &Evidence{Summary: "s"}
		if err := Approve(ev, "ai:claude", files, t0); err == nil {
			t.Fatal("want error")
		}
		if ev.ApprovedBy != "" {
			t.Errorf("approved_by = %q", ev.ApprovedBy)
		}
	})
}

func TestCreateEvidence(t *testing.T) {
	setup := func(t *testing.T) (*Repo, string) {
		root, src := t.TempDir(), t.TempDir()
		for name, content := range map[string]string{"data.csv": "hello", "evidence.yaml": "x"} {
			os.WriteFile(filepath.Join(src, name), []byte(content), 0o644)
		}
		os.MkdirAll(filepath.Join(src, "sub"), 0o755)
		os.WriteFile(filepath.Join(src, "sub", "data.csv"), []byte("other"), 0o644)
		return &Repo{Root: root}, src
	}
	evidenceDirs := func(r *Repo) []os.DirEntry {
		es, _ := os.ReadDir(filepath.Join(r.Root, evidenceDir))
		return es
	}
	t.Run("添付ファイルをコピーし、そのsha256を記録する", func(t *testing.T) {
		r, src := setup(t)
		ev := &Evidence{Summary: "s"}
		paths, err := r.CreateEvidence(ev, []string{filepath.Join(src, "data.csv")})
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(filepath.Join(r.Root, "evidence", ev.ID, "data.csv")); string(got) != "hello" {
			t.Errorf("copied content = %q", got)
		}
		if len(ev.Files) != 1 || ev.Files[0] != (EvidenceFile{Path: "data.csv", SHA256: helloSHA256}) {
			t.Errorf("files = %+v", ev.Files)
		}
		want := []string{filepath.Join("evidence", ev.ID, "data.csv"), filepath.Join("evidence", ev.ID, "evidence.yaml")}
		if strings.Join(paths, ",") != strings.Join(want, ",") {
			t.Errorf("paths = %v, want %v", paths, want)
		}
		evs, err := r.LoadEvidence()
		if err != nil || len(evs) != 1 || evs[0].ID != ev.ID {
			t.Errorf("loaded = %+v, %v", evs, err)
		}
	})
	t.Run("添付ファイルなしでも作成できる", func(t *testing.T) {
		r, _ := setup(t)
		if _, err := r.CreateEvidence(&Evidence{Summary: "s", Source: "https://example.com"}, nil); err != nil {
			t.Fatal(err)
		}
	})
	failures := []struct {
		name    string
		srcs    func(src string) []string
		wantErr string
	}{
		{"同じファイル名の添付は拒否する", func(src string) []string {
			return []string{filepath.Join(src, "data.csv"), filepath.Join(src, "sub", "data.csv")}
		}, "duplicate"},
		{"evidence.yamlという名前の添付は拒否する", func(src string) []string { return []string{filepath.Join(src, "evidence.yaml")} }, "cannot attach"},
		{"存在しないファイルの添付は拒否する", func(src string) []string { return []string{filepath.Join(src, "missing.csv")} }, "no such file"},
		{"ディレクトリの添付は拒否する", func(src string) []string { return []string{filepath.Join(src, "sub")} }, "not a regular file"},
	}
	for _, tt := range failures {
		t.Run(tt.name+"し、根拠のディレクトリを残さない", func(t *testing.T) {
			r, src := setup(t)
			if _, err := r.CreateEvidence(&Evidence{Summary: "s"}, tt.srcs(src)); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
			if es := evidenceDirs(r); len(es) != 0 {
				t.Errorf("evidence dirs left: %d", len(es))
			}
		})
	}
	t.Run("コピーの途中で失敗したら、根拠のディレクトリごと消す", func(t *testing.T) {
		r, src := setup(t)
		unreadable := filepath.Join(src, "secret.csv")
		os.WriteFile(unreadable, []byte("x"), 0o000)
		if _, err := os.ReadFile(unreadable); err == nil {
			t.Skip("running as a user that can read mode 000 files")
		}
		if _, err := r.CreateEvidence(&Evidence{Summary: "s"}, []string{filepath.Join(src, "data.csv"), unreadable}); err == nil {
			t.Fatal("want error")
		}
		if es := evidenceDirs(r); len(es) != 0 {
			t.Errorf("evidence dirs left: %d", len(es))
		}
	})
}

func TestUnlink(t *testing.T) {
	t.Run("紐づけを外し、外したstanceを返す", func(t *testing.T) {
		n := node("n", "", 0)
		n.Evidence = []EvidenceLink{{ID: "e0", Stance: StanceSupports}, {ID: "e1", Stance: StanceRefutes}, {ID: "e2", Stance: StanceSupports}}
		got, err := Unlink(n, "e1")
		if err != nil {
			t.Fatal(err)
		}
		if got != StanceRefutes {
			t.Errorf("stance = %q", got)
		}
		if len(n.Evidence) != 2 || n.Evidence[0].ID != "e0" || n.Evidence[1].ID != "e2" {
			t.Errorf("evidence = %+v", n.Evidence)
		}
	})
	t.Run("紐づいていない根拠を外そうとするとエラーにする", func(t *testing.T) {
		n := node("n", "", 0)
		if _, err := Unlink(n, "e1"); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestLinkedFrom(t *testing.T) {
	a, b, c := node("a", "", 0), node("b", "", 1), node("c", "", 2)
	a.Evidence = []EvidenceLink{{ID: "e1", Stance: StanceSupports}}
	b.Evidence = []EvidenceLink{{ID: "e2", Stance: StanceSupports}}
	c.Evidence = []EvidenceLink{{ID: "e2", Stance: StanceRefutes}, {ID: "e1", Stance: StanceRefutes}}
	t.Run("根拠を紐づけている仮説だけを返す", func(t *testing.T) {
		got := LinkedFrom([]*Node{a, b, c}, "e1")
		if len(got) != 2 || got[0] != a || got[1] != c {
			t.Errorf("got %d nodes", len(got))
		}
	})
}

func TestApproval(t *testing.T) {
	files := []EvidenceFile{{Path: "a.csv", SHA256: "aa"}}
	approved := func() *Evidence {
		ev := &Evidence{Summary: "s", Source: "u"}
		Approve(ev, "human:tester", files, t0)
		return ev
	}
	t.Run("承認していなければunapproved", func(t *testing.T) {
		if got := Approval(&Evidence{Summary: "s"}, files); got != ApprovalNone {
			t.Errorf("got %s", got)
		}
	})
	t.Run("承認後に変わっていなければapproved", func(t *testing.T) {
		if got := Approval(approved(), files); got != ApprovalCurrent {
			t.Errorf("got %s", got)
		}
	})
	t.Run("承認後に添付が変わればchanged-since-approval", func(t *testing.T) {
		if got := Approval(approved(), []EvidenceFile{{Path: "a.csv", SHA256: "xx"}}); got != ApprovalOutdated {
			t.Errorf("got %s", got)
		}
	})
	t.Run("承認後に本文が変わればchanged-since-approval", func(t *testing.T) {
		ev := approved()
		ev.Summary = "edited"
		if got := Approval(ev, files); got != ApprovalOutdated {
			t.Errorf("got %s", got)
		}
	})
}

func TestRemoveEvidence(t *testing.T) {
	setup := func(t *testing.T) (*Repo, *Evidence) {
		r := &Repo{Root: t.TempDir()}
		src := filepath.Join(t.TempDir(), "data.csv")
		os.WriteFile(src, []byte("hello"), 0o644)
		ev := &Evidence{Summary: "s"}
		if _, err := r.CreateEvidence(ev, []string{src}); err != nil {
			t.Fatal(err)
		}
		return r, ev
	}
	t.Run("根拠のディレクトリを添付ごと消す", func(t *testing.T) {
		r, ev := setup(t)
		path, err := r.RemoveEvidence(ev)
		if err != nil {
			t.Fatal(err)
		}
		if path != filepath.Join("evidence", ev.ID) {
			t.Errorf("path = %s", path)
		}
		if _, err := os.Stat(filepath.Join(r.Root, path)); !os.IsNotExist(err) {
			t.Errorf("dir should be removed: %v", err)
		}
	})
	t.Run("記録にないファイルがあれば何も消さずにエラーにする", func(t *testing.T) {
		r, ev := setup(t)
		extra := filepath.Join(r.Root, "evidence", ev.ID, "note.txt")
		os.WriteFile(extra, []byte("x"), 0o644)
		if _, err := r.RemoveEvidence(ev); err == nil || !strings.Contains(err.Error(), "note.txt") {
			t.Fatalf("err = %v", err)
		}
		for _, name := range []string{"note.txt", "data.csv", "evidence.yaml"} {
			if _, err := os.Stat(filepath.Join(r.Root, "evidence", ev.ID, name)); err != nil {
				t.Errorf("%s should remain: %v", name, err)
			}
		}
	})
	t.Run("記録済みの添付が既に消えていても削除できる", func(t *testing.T) {
		r, ev := setup(t)
		os.Remove(filepath.Join(r.Root, "evidence", ev.ID, "data.csv"))
		if _, err := r.RemoveEvidence(ev); err != nil {
			t.Fatal(err)
		}
	})
}
