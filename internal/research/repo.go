package research

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	MarkerFile    = "hyp.yaml"
	hypothesesDir = "hypotheses"
)

// Repo は研究ディレクトリ。パスはすべて Root からの相対パスで返す（git に渡すため）。
type Repo struct {
	Root string
}

// Find は git が .git を探すのと同じく、dir から上に辿って研究ディレクトリを探す。
func Find(dir string) (*Repo, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, MarkerFile)); err == nil {
			return &Repo{Root: dir}, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("not a hyp research directory (or any of the parent directories): %s not found", MarkerFile)
		}
		dir = parent
	}
}

// Init は dir に目印ファイルを作り、作成したファイルの相対パスを返す。
func Init(dir string) (*Repo, string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, "", err
	}
	path := filepath.Join(dir, MarkerFile)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, "", fmt.Errorf("already initialized: %s exists", path)
		}
		return nil, "", err
	}
	defer f.Close()
	if _, err := f.WriteString("{}\n"); err != nil {
		return nil, "", err
	}
	return &Repo{Root: dir}, MarkerFile, nil
}

func (r *Repo) NodePath(id string) string {
	return filepath.Join(hypothesesDir, id+".yaml")
}

func (r *Repo) LoadNodes() ([]*Node, error) {
	entries, err := os.ReadDir(filepath.Join(r.Root, hypothesesDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var nodes []*Node
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(r.Root, hypothesesDir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var n Node
		if err := yaml.Unmarshal(b, &n); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		nodes = append(nodes, &n)
	}
	return nodes, nil
}

// SaveNode は書き込んだファイルの相対パスを返す。
func (r *Repo) SaveNode(n *Node) (string, error) {
	b, err := yaml.Marshal(n)
	if err != nil {
		return "", err
	}
	rel := r.NodePath(n.ID)
	if err := os.MkdirAll(filepath.Join(r.Root, hypothesesDir), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(r.Root, rel), b, 0o644); err != nil {
		return "", err
	}
	return rel, nil
}

// RemoveNode は削除したファイルの相対パスを返す。
func (r *Repo) RemoveNode(id string) (string, error) {
	rel := r.NodePath(id)
	if err := os.Remove(filepath.Join(r.Root, rel)); err != nil {
		return "", err
	}
	return rel, nil
}
