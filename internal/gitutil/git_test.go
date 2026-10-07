package gitutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.name", "tester"},
		{"config", "user.email", "tester@example.com"},
	} {
		if _, err := run(dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCommit(t *testing.T) {
	t.Run("指定したファイルだけをコミットし、ステージ済みのほかの変更は含めない", func(t *testing.T) {
		dir := newRepo(t)
		write(t, dir, "target.yaml", "a")
		write(t, dir, "other.yaml", "b")
		if _, err := run(dir, "add", "other.yaml"); err != nil {
			t.Fatal(err)
		}
		if err := Commit(dir, "msg", "human:tester", "target.yaml"); err != nil {
			t.Fatal(err)
		}
		files, _ := run(dir, "show", "--name-only", "--format=", "HEAD")
		if files != "target.yaml" {
			t.Errorf("committed files = %q", files)
		}
		status, _ := run(dir, "status", "--porcelain")
		if !strings.Contains(status, "other.yaml") {
			t.Errorf("other.yaml should remain uncommitted: %q", status)
		}
	})
	t.Run("削除したファイルの削除をコミットする", func(t *testing.T) {
		dir := newRepo(t)
		write(t, dir, "x.yaml", "a")
		if err := Commit(dir, "add", "human:tester", "x.yaml"); err != nil {
			t.Fatal(err)
		}
		os.Remove(filepath.Join(dir, "x.yaml"))
		if err := Commit(dir, "rm", "human:tester", "x.yaml"); err != nil {
			t.Fatal(err)
		}
		if files, _ := run(dir, "ls-files"); files != "" {
			t.Errorf("tracked files = %q", files)
		}
	})
	t.Run("操作の主体をHyp-Actorトレーラーとして残す", func(t *testing.T) {
		dir := newRepo(t)
		write(t, dir, "x.yaml", "a")
		if err := Commit(dir, "msg", "ai:claude", "x.yaml"); err != nil {
			t.Fatal(err)
		}
		got, _ := run(dir, "log", "-1", "--format=%(trailers:key=Hyp-Actor,valueonly)")
		if got != "ai:claude" {
			t.Errorf("trailer = %q", got)
		}
	})
}

func TestActor(t *testing.T) {
	t.Run("環境変数で名乗りがあればそれを主体とする", func(t *testing.T) {
		t.Setenv(ActorEnv, "ai:claude")
		if got, _ := Actor(newRepo(t)); got != "ai:claude" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("名乗りがなければgitの利用者本人の操作とする", func(t *testing.T) {
		t.Setenv(ActorEnv, "")
		if got, _ := Actor(newRepo(t)); got != "human:tester" {
			t.Errorf("got %q", got)
		}
	})
}

func head(t *testing.T, dir string) string {
	t.Helper()
	out, err := run(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func read(t *testing.T, dir, name string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", false
	}
	return string(b), true
}

func TestUndo(t *testing.T) {
	// base: a.yaml=v1 をコミット済みの状態から始める。
	setup := func(t *testing.T) string {
		dir := newRepo(t)
		write(t, dir, "a.yaml", "v1")
		if err := Commit(dir, "base", "human:tester", "a.yaml"); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	t.Run("hypのコミットで変更・追加・削除したファイルをすべて元に戻す", func(t *testing.T) {
		dir := setup(t)
		write(t, dir, "b.yaml", "b")
		if err := Commit(dir, "base2", "human:tester", "b.yaml"); err != nil {
			t.Fatal(err)
		}
		write(t, dir, "a.yaml", "v2")
		write(t, dir, "c.yaml", "new")
		os.Remove(filepath.Join(dir, "b.yaml"))
		if err := Commit(dir, "change", "human:tester", "a.yaml", "b.yaml", "c.yaml"); err != nil {
			t.Fatal(err)
		}
		subject, err := Undo(dir, "human:tester", false)
		if err != nil {
			t.Fatal(err)
		}
		if subject != "change" {
			t.Errorf("subject = %q", subject)
		}
		if got, _ := read(t, dir, "a.yaml"); got != "v1" {
			t.Errorf("a.yaml = %q, want v1", got)
		}
		if got, ok := read(t, dir, "b.yaml"); !ok || got != "b" {
			t.Errorf("b.yaml should be restored, got %q %v", got, ok)
		}
		if _, ok := read(t, dir, "c.yaml"); ok {
			t.Error("c.yaml should be removed")
		}
		if status, _ := run(dir, "status", "--porcelain"); status != "" {
			t.Errorf("working tree should be clean: %q", status)
		}
		if actor, _ := run(dir, "log", "-1", "--format=%(trailers:key=Hyp-Actor,valueonly)"); actor != "human:tester" {
			t.Errorf("undo commit actor = %q", actor)
		}
	})
	t.Run("hyp以外のコミットはforceなしでは取り消さない", func(t *testing.T) {
		dir := setup(t)
		write(t, dir, "a.yaml", "manual")
		run(dir, "commit", "-q", "-am", "manual")
		before := head(t, dir)
		if _, err := Undo(dir, "human:tester", false); err == nil || !strings.Contains(err.Error(), "--force") {
			t.Fatalf("err = %v", err)
		}
		if head(t, dir) != before {
			t.Error("HEAD should not move")
		}
	})
	t.Run("forceならhyp以外のコミットも取り消す", func(t *testing.T) {
		dir := setup(t)
		write(t, dir, "a.yaml", "manual")
		run(dir, "commit", "-q", "-am", "manual")
		if _, err := Undo(dir, "human:tester", true); err != nil {
			t.Fatal(err)
		}
		if got, _ := read(t, dir, "a.yaml"); got != "v1" {
			t.Errorf("a.yaml = %q", got)
		}
	})
	t.Run("対象のファイルに未コミットの変更があれば上書きせずに中止する", func(t *testing.T) {
		dir := setup(t)
		write(t, dir, "a.yaml", "v2")
		Commit(dir, "change", "human:tester", "a.yaml")
		write(t, dir, "a.yaml", "edited directly")
		before := head(t, dir)
		if _, err := Undo(dir, "human:tester", false); err == nil {
			t.Fatal("want error")
		}
		if got, _ := read(t, dir, "a.yaml"); got != "edited directly" || head(t, dir) != before {
			t.Errorf("a.yaml = %q", got)
		}
	})
	t.Run("ほかにステージ済みの変更があっても取り消しのコミットに含めない", func(t *testing.T) {
		dir := setup(t)
		write(t, dir, "a.yaml", "v2")
		Commit(dir, "change", "human:tester", "a.yaml")
		write(t, dir, "other.yaml", "x")
		run(dir, "add", "other.yaml")
		if _, err := Undo(dir, "human:tester", false); err != nil {
			t.Fatal(err)
		}
		if files, _ := run(dir, "show", "--name-only", "--format=", "HEAD"); files != "a.yaml" {
			t.Errorf("committed files = %q", files)
		}
	})
	t.Run("最初のコミットは取り消せない", func(t *testing.T) {
		dir := setup(t)
		if _, err := Undo(dir, "human:tester", true); err == nil || !strings.Contains(err.Error(), "nothing to undo") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestUndoInSubdirectory(t *testing.T) {
	t.Run("研究ディレクトリがリポジトリのサブディレクトリでも取り消せる", func(t *testing.T) {
		dir := newRepo(t)
		sub := filepath.Join(dir, "research")
		os.MkdirAll(sub, 0o755)
		write(t, sub, "a.yaml", "v1")
		if err := Commit(sub, "base", "human:tester", "a.yaml"); err != nil {
			t.Fatal(err)
		}
		write(t, sub, "a.yaml", "v2")
		write(t, sub, "b.yaml", "new")
		if err := Commit(sub, "change", "human:tester", "a.yaml", "b.yaml"); err != nil {
			t.Fatal(err)
		}
		if _, err := Undo(sub, "human:tester", false); err != nil {
			t.Fatal(err)
		}
		if got, _ := read(t, sub, "a.yaml"); got != "v1" {
			t.Errorf("a.yaml = %q", got)
		}
		if _, ok := read(t, sub, "b.yaml"); ok {
			t.Error("b.yaml should be removed")
		}
	})
}
