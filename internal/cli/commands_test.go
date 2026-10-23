package cli

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/TadahiroYamamura/hyp/internal/gitutil"
)

func git(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func hyp(args ...string) (string, error) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return strings.TrimSpace(out.String()), err
}

// newResearchDir は仮説を 1 つ持つ研究ディレクトリをカレントディレクトリに作り、その仮説の ID を返す。
func newResearchDir(t *testing.T) string {
	t.Helper()
	t.Chdir(t.TempDir())
	git(t, "init", "-q")
	git(t, "config", "user.name", "tester")
	git(t, "config", "user.email", "tester@example.com")
	t.Setenv(gitutil.ActorEnv, "")
	if _, err := hyp("init"); err != nil {
		t.Fatal(err)
	}
	id, err := hyp("add", "全体仮説")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestReason(t *testing.T) {
	t.Run("主体がAIで理由がなければエラーにし、ファイルを変更しない", func(t *testing.T) {
		id := newResearchDir(t)
		t.Setenv(gitutil.ActorEnv, "ai:claude")
		for _, reason := range []string{"", " \n"} {
			_, err := hyp("set", id, "--title", "変更後", "--reason", reason)
			if err == nil || !strings.Contains(err.Error(), "--reason") {
				t.Errorf("reason %q: err = %v", reason, err)
			}
		}
		if status := git(t, "status", "--porcelain"); status != "" {
			t.Errorf("uncommitted changes: %q", status)
		}
		if subject := git(t, "log", "-1", "--format=%s"); !strings.HasPrefix(subject, "add ") {
			t.Errorf("last commit = %q", subject)
		}
	})
	t.Run("主体がAIでも理由があれば変更でき、理由をコミットの本文に残す", func(t *testing.T) {
		id := newResearchDir(t)
		t.Setenv(gitutil.ActorEnv, "ai:claude")
		if _, err := hyp("set", id, "--title", "変更後", "--reason", "タイトルが曖昧だと指摘された"); err != nil {
			t.Fatal(err)
		}
		if body := git(t, "log", "-1", "--format=%b"); !strings.Contains(body, "タイトルが曖昧だと指摘された") {
			t.Errorf("body = %q", body)
		}
	})
	t.Run("人は理由を省略して変更できる", func(t *testing.T) {
		id := newResearchDir(t)
		if _, err := hyp("set", id, "--title", "変更後"); err != nil {
			t.Fatal(err)
		}
		if subject := git(t, "log", "-1", "--format=%s"); !strings.HasPrefix(subject, "set ") {
			t.Errorf("last commit = %q", subject)
		}
	})
	t.Run("研究の中身を変えるコマンドはすべて--reasonを受け取る", func(t *testing.T) {
		for _, path := range [][]string{
			{"add"}, {"set"}, {"mv"}, {"rm"}, {"undo"},
			{"evidence", "add"}, {"evidence", "link"}, {"evidence", "unlink"}, {"evidence", "approve"}, {"evidence", "rm"},
		} {
			cmd, _, err := NewRootCmd().Find(path)
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Flags().Lookup("reason") == nil {
				t.Errorf("%v has no --reason", path)
			}
		}
	})
}

func TestSet(t *testing.T) {
	t.Run("今と同じ値を設定すると、変更がないことをエラーで返しコミットを作らない", func(t *testing.T) {
		id := newResearchDir(t)
		before := git(t, "rev-parse", "HEAD")
		_, err := hyp("set", id, "--title", "全体仮説")
		if !errors.Is(err, gitutil.ErrNoChanges) {
			t.Errorf("err = %v", err)
		}
		if after := git(t, "rev-parse", "HEAD"); after != before {
			t.Error("HEAD should not move")
		}
	})
}
