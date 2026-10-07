package research

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	MarkerFile    = "hyp.yaml"
	hypothesesDir = "hypotheses"
	evidenceDir   = "evidence"
	evidenceFile  = "evidence.yaml"
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

func (r *Repo) EvidencePath(id string) string {
	return filepath.Join(evidenceDir, id, evidenceFile)
}

// EvidenceFilePath は根拠に添付したファイルの相対パスを返す。
func (r *Repo) EvidenceFilePath(id, name string) string {
	return filepath.Join(evidenceDir, id, name)
}

func (r *Repo) LoadEvidence() ([]*Evidence, error) {
	entries, err := os.ReadDir(filepath.Join(r.Root, evidenceDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var evs []*Evidence
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(r.Root, r.EvidencePath(e.Name()))
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var ev Evidence
		if err := yaml.Unmarshal(b, &ev); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		evs = append(evs, &ev)
	}
	return evs, nil
}

// SaveEvidence は書き込んだファイルの相対パスを返す。
func (r *Repo) SaveEvidence(ev *Evidence) (string, error) {
	b, err := yaml.Marshal(ev)
	if err != nil {
		return "", err
	}
	rel := r.EvidencePath(ev.ID)
	if err := os.MkdirAll(filepath.Join(r.Root, evidenceDir, ev.ID), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(r.Root, rel), b, 0o644); err != nil {
		return "", err
	}
	return rel, nil
}

// CreateEvidence は ev に新しい ID を振り、srcs を根拠のディレクトリにコピーして保存する。
// 途中で失敗したら、コピー済みのファイルを残さないようディレクトリごと消す。
// 書き込んだファイルの相対パスを返す。
func (r *Repo) CreateEvidence(ev *Evidence, srcs []string) (paths []string, err error) {
	seen := map[string]bool{}
	for _, src := range srcs {
		name := filepath.Base(src)
		if name == evidenceFile {
			return nil, fmt.Errorf("cannot attach a file named %s", evidenceFile)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate attachment name %s", name)
		}
		seen[name] = true
		if info, err := os.Stat(src); err != nil {
			return nil, err
		} else if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file", src)
		}
	}
	if err := os.MkdirAll(filepath.Join(r.Root, evidenceDir), 0o755); err != nil {
		return nil, err
	}
	var dir string
	for {
		if ev.ID, err = NewID(); err != nil {
			return nil, err
		}
		dir = filepath.Join(r.Root, evidenceDir, ev.ID)
		if err = os.Mkdir(dir, 0o755); err == nil {
			break
		} else if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	defer func() {
		if err != nil {
			os.RemoveAll(dir)
		}
	}()
	for _, src := range srcs {
		rel := r.EvidenceFilePath(ev.ID, filepath.Base(src))
		if err = copyFile(src, filepath.Join(r.Root, rel)); err != nil {
			return nil, err
		}
		h, err := HashFile(filepath.Join(r.Root, rel))
		if err != nil {
			return nil, err
		}
		ev.Files = append(ev.Files, EvidenceFile{Path: filepath.Base(src), SHA256: h})
		paths = append(paths, rel)
	}
	rel, err := r.SaveEvidence(ev)
	if err != nil {
		return nil, err
	}
	return append(paths, rel), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// HashEvidenceFiles は添付ファイルの現在の sha256 を計算する。記録済みの値は使わない。
func (r *Repo) HashEvidenceFiles(ev *Evidence) ([]EvidenceFile, error) {
	files := make([]EvidenceFile, len(ev.Files))
	for i, f := range ev.Files {
		h, err := HashFile(filepath.Join(r.Root, r.EvidenceFilePath(ev.ID, f.Path)))
		if err != nil {
			return nil, err
		}
		files[i] = EvidenceFile{Path: f.Path, SHA256: h}
	}
	return files, nil
}

// RemoveEvidence は根拠のディレクトリを消し、その相対パスを返す。
// 記録済みの添付が既に消えていても削除をコミットできるよう、個々のファイルではなくディレクトリを返す。
// 記録にないファイルが根拠のディレクトリにあれば、何も消さずにエラーにする。
// それらは git で追跡されていない可能性があり、消すと戻せないため。
func (r *Repo) RemoveEvidence(ev *Evidence) (string, error) {
	dir := filepath.Join(r.Root, evidenceDir, ev.ID)
	known := map[string]bool{evidenceFile: true}
	for _, f := range ev.Files {
		known[f.Path] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var unknown []string
	for _, e := range entries {
		if !known[e.Name()] {
			unknown = append(unknown, e.Name())
		}
	}
	if len(unknown) > 0 {
		return "", fmt.Errorf("%s contains files not recorded in the evidence: %s", filepath.Join(evidenceDir, ev.ID), strings.Join(unknown, ", "))
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	return filepath.Join(evidenceDir, ev.ID), nil
}
