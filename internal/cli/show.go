package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/TadahiroYamamura/hyp/internal/research"
)

type jsonNode struct {
	Label string `json:"label"`
	*research.Node
	Children []*jsonNode `json:"children,omitempty"`
}

func newShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:               "show [id]",
		Short:             "List root hypotheses, or show the tree under a hypothesis",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeIDs(nodeID),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(args) == 0 {
				if asJSON {
					roots := make([]*jsonNode, 0, len(s.tree.Roots()))
					for _, r := range s.tree.Roots() {
						roots = append(roots, &jsonNode{Label: s.tree.Label(r.ID), Node: r})
					}
					return writeJSON(out, roots)
				}
				for _, r := range s.tree.Roots() {
					fmt.Fprintf(out, "%s  %s  %s\n", s.tree.Label(r.ID), r.ID, r.Title)
				}
				return nil
			}
			n, err := s.resolve(args[0])
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(out, toJSON(s.tree, n))
			}
			fmt.Fprintln(out, describe(s.tree, n))
			printChildren(out, s.tree, n.ID, "")
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

func toJSON(t *research.Tree, n *research.Node) *jsonNode {
	j := &jsonNode{Label: t.Label(n.ID), Node: n}
	for _, c := range t.Children(n.ID) {
		j.Children = append(j.Children, toJSON(t, c))
	}
	return j
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printChildren(w io.Writer, t *research.Tree, id, indent string) {
	children := t.Children(id)
	for i, c := range children {
		branch, next := "├─ ", "│  "
		if i == len(children)-1 {
			branch, next = "└─ ", "   "
		}
		fmt.Fprintln(w, indent+branch+describe(t, c))
		printChildren(w, t, c.ID, indent+next)
	}
}

// describe は子を持つノードには combine を、末端には status を表示する。
// 子を持つノードの結論は子から導出するもので、status は判定に使わないため。
func describe(t *research.Tree, n *research.Node) string {
	tag := string(n.Status)
	if t.HasChildren(n.ID) {
		tag = string(n.Combine)
		if tag == "" {
			tag = "combine unset"
		}
	}
	if n.Axis != "" {
		tag += " by " + n.Axis
	}
	s := fmt.Sprintf("%s  %s  [%s] %s", t.Label(n.ID), n.ID, tag, n.Title)
	if n.Parent != "" {
		s += fmt.Sprintf("  (weight %s)", percent(t.Share(n.ID)))
	}
	if n.Condition != "" {
		s += fmt.Sprintf("  (if: %s)", n.Condition)
	}
	return s
}
