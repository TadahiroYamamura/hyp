package gitutil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const ActorEnv = "HYP_ACTOR"

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func IsInsideWorkTree(dir string) bool {
	out, err := run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

func Init(dir string) error {
	_, err := run(dir, "init", "-q")
	return err
}

// Actor は操作の主体を返す。AI エージェントには環境変数で自身を名乗らせ、
// 名乗りがなければ git の利用者本人の操作とみなす。
func Actor(dir string) (string, error) {
	if a := os.Getenv(ActorEnv); a != "" {
		return a, nil
	}
	name, err := run(dir, "config", "user.name")
	if err != nil || name == "" {
		return "", fmt.Errorf("cannot determine actor: set %s or git user.name", ActorEnv)
	}
	return "human:" + name, nil
}

// Commit は paths だけをコミットする（git commit にパスを渡すと、ほかにステージ済みの変更があっても含めない）。
// CLI を経由しない変更を未コミットのまま残し、検知できるようにするため。
func Commit(dir, message, actor string, paths ...string) error {
	addArgs := append([]string{"add", "-A", "--"}, paths...)
	if _, err := run(dir, addArgs...); err != nil {
		return err
	}
	commitArgs := append([]string{"commit", "-q", "-m", message, "--trailer", "Hyp-Actor: " + actor, "--"}, paths...)
	_, err := run(dir, commitArgs...)
	return err
}

// Undo は直前のコミットを打ち消すコミットを作る。
// git revert を使わないのは、取り消しのコミットにも対象のファイルだけを含め、
// Hyp-Actor を付けるため（revert の途中ではパスを指定したコミットができない）。
// force が false なら、hyp が作ったコミット（Hyp-Actor を持つもの）だけを取り消す。
func Undo(dir, actor string, force bool) (string, error) {
	if _, err := run(dir, "rev-parse", "--verify", "-q", "HEAD~1"); err != nil {
		return "", fmt.Errorf("nothing to undo: HEAD has no parent")
	}
	if !force {
		hypActor, err := run(dir, "log", "-1", "--format=%(trailers:key=Hyp-Actor,valueonly)")
		if err != nil {
			return "", err
		}
		if hypActor == "" {
			return "", fmt.Errorf("the last commit was not made by hyp; use --force to undo it anyway")
		}
	}
	// 研究ディレクトリが git リポジトリの一部でもパスがずれないよう、dir からの相対パスで扱う。
	out, err := run(dir, "diff-tree", "--relative", "--no-commit-id", "--name-only", "-r", "-z", "HEAD")
	if err != nil {
		return "", err
	}
	paths := strings.Split(strings.TrimRight(out, "\x00"), "\x00")
	// 未コミットの変更を上書きで消さないよう、対象のファイルが HEAD と一致しているときだけ進める。
	statusArgs := append([]string{"status", "--porcelain", "--"}, paths...)
	if dirty, err := run(dir, statusArgs...); err != nil {
		return "", err
	} else if dirty != "" {
		return "", fmt.Errorf("files touched by the last commit have uncommitted changes:\n%s", dirty)
	}
	for _, p := range paths {
		if _, err := run(dir, "cat-file", "-e", "HEAD~1:./"+p); err == nil {
			_, err = run(dir, "checkout", "HEAD~1", "--", p)
			if err != nil {
				return "", err
			}
		} else if err := os.Remove(filepath.Join(dir, p)); err != nil {
			// git rm で索引からも消すと、Commit の git add がパスを見つけられず失敗するため、作業ツリーだけ消す。
			return "", err
		}
	}
	subject, err := run(dir, "log", "-1", "--format=%s")
	if err != nil {
		return "", err
	}
	if err := Commit(dir, "undo: "+subject, actor, paths...); err != nil {
		return "", err
	}
	return subject, nil
}
