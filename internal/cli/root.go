package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/TadahiroYamamura/hyp/internal/gitutil"
	"github.com/TadahiroYamamura/hyp/internal/research"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "hyp",
		Short:         "Manage hypothesis trees",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newInitCmd(),
		newAddCmd(),
		newSetCmd(),
		newMvCmd(),
		newRmCmd(),
		newShowCmd(),
		newUndoCmd(),
		newEvidenceCmd(),
		newConcludeCmd(),
	)
	return root
}

func Execute() int {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

// session はカレントディレクトリの研究ディレクトリと、その全ノードから組んだツリー、全根拠。
type session struct {
	repo      *research.Repo
	nodes     []*research.Node
	tree      *research.Tree
	evidences []*research.Evidence
}

func openSession() (*session, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	repo, err := research.Find(wd)
	if err != nil {
		return nil, err
	}
	nodes, err := repo.LoadNodes()
	if err != nil {
		return nil, err
	}
	evs, err := repo.LoadEvidence()
	if err != nil {
		return nil, err
	}
	return &session{repo: repo, nodes: nodes, tree: research.BuildTree(nodes), evidences: evs}, nil
}

func (s *session) resolve(prefix string) (*research.Node, error) {
	return research.Resolve(s.nodes, prefix)
}

func (s *session) save(nodes ...*research.Node) ([]string, error) {
	paths := make([]string, 0, len(nodes))
	for _, n := range nodes {
		p, err := s.repo.SaveNode(n)
		if err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, nil
}

func (s *session) actor() (string, error) {
	return gitutil.Actor(s.repo.Root)
}

func (s *session) commit(message string, paths ...string) error {
	actor, err := s.actor()
	if err != nil {
		return err
	}
	return gitutil.Commit(s.repo.Root, message, actor, paths...)
}

func now() time.Time { return time.Now().UTC() }

type idKind int

const (
	nodeID idKind = iota
	evidenceID
)

// completeIDs は ID 引数の Tab 補完候補を「ID<TAB>説明」の形で返す。kinds は引数の位置ごとの ID の種類。
// zsh / fish では説明が並ぶので、ID を覚えていなくても選べる。
func completeIDs(kinds ...idKind) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) >= len(kinds) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		s, err := openSession()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var out []string
		switch kinds[len(args)] {
		case nodeID:
			for _, n := range s.nodes {
				if strings.HasPrefix(n.ID, toComplete) {
					out = append(out, n.ID+"\t"+s.tree.Label(n.ID)+" "+n.Title)
				}
			}
		case evidenceID:
			for _, e := range s.evidences {
				if strings.HasPrefix(e.ID, toComplete) {
					out = append(out, e.ID+"\t"+e.Summary)
				}
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
}
