package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/TadahiroYamamura/hyp/internal/gitutil"
	"github.com/TadahiroYamamura/hyp/internal/research"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create a research directory in the current directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			if !gitutil.IsInsideWorkTree(wd) {
				if err := gitutil.Init(wd); err != nil {
					return err
				}
			}
			repo, path, err := research.Init(wd)
			if err != nil {
				return err
			}
			s := &session{repo: repo}
			if err := s.commit("init research directory", path); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "initialized research directory in %s\n", repo.Root)
			return nil
		},
	}
}

func newAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "add [parent] <title>",
		Short:             "Add a hypothesis (as a root if parent is omitted)",
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: completeIDs(nodeID),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			parentID, title := "", args[0]
			if len(args) == 2 {
				p, err := s.resolve(args[0])
				if err != nil {
					return err
				}
				parentID, title = p.ID, args[1]
			}
			n, changed, err := research.NewNode(s.tree, parentID, title, now())
			if err != nil {
				return err
			}
			paths, err := s.save(append(changed, n)...)
			if err != nil {
				return err
			}
			if err := s.commit(fmt.Sprintf("add %s: %s", n.ID, n.Title), paths...); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), n.ID)
			return nil
		},
	}
}

func newSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "set <id>",
		Short:             "Update attributes of a hypothesis",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeIDs(nodeID),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			n, err := s.resolve(args[0])
			if err != nil {
				return err
			}
			var e research.Edit
			// 空文字列で値を消せるよう、指定の有無はフラグが渡されたかで判断する。
			flags := cmd.Flags()
			for name, dst := range map[string]**string{
				"title": &e.Title, "description": &e.Description, "method": &e.Method,
				"status": &e.Status, "combine": &e.Combine, "condition": &e.Condition,
			} {
				if flags.Changed(name) {
					v, _ := flags.GetString(name)
					*dst = &v
				}
			}
			if e == (research.Edit{}) {
				return fmt.Errorf("nothing to update")
			}
			if err := research.ApplyEdit(s.tree, n, e); err != nil {
				return err
			}
			path, err := s.repo.SaveNode(n)
			if err != nil {
				return err
			}
			return s.commit(fmt.Sprintf("set %s: %s", n.ID, n.Title), path)
		},
	}
	f := cmd.Flags()
	f.String("title", "", "title")
	f.String("description", "", "description")
	f.String("method", "", "how to test the hypothesis")
	f.String("status", "", "status of a leaf hypothesis")
	f.String("combine", "", "how children make up this hypothesis (empty to unset)")
	f.String("condition", "", "condition of this case (parent's combine must be cases)")
	_ = cmd.RegisterFlagCompletionFunc("status", fixedCompletion(research.Statuses))
	_ = cmd.RegisterFlagCompletionFunc("combine", fixedCompletion(research.Combines))
	return cmd
}

func fixedCompletion[T ~string](vs []T) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		out := make([]string, len(vs))
		for i, v := range vs {
			out[i] = string(v)
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
}

func newMvCmd() *cobra.Command {
	var toRoot bool
	cmd := &cobra.Command{
		Use:               "mv <id> (<new-parent> | --root)",
		Short:             "Move a hypothesis under another hypothesis, or to the root",
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: completeIDs(nodeID, nodeID),
		RunE: func(_ *cobra.Command, args []string) error {
			if toRoot == (len(args) == 2) {
				return fmt.Errorf("specify either <new-parent> or --root")
			}
			s, err := openSession()
			if err != nil {
				return err
			}
			n, err := s.resolve(args[0])
			if err != nil {
				return err
			}
			newParentID, dest := "", "root"
			if !toRoot {
				p, err := s.resolve(args[1])
				if err != nil {
					return err
				}
				newParentID, dest = p.ID, p.ID
			}
			changed, err := research.Move(s.tree, n, newParentID)
			if err != nil {
				return err
			}
			paths, err := s.save(changed...)
			if err != nil {
				return err
			}
			return s.commit(fmt.Sprintf("mv %s to %s", n.ID, dest), paths...)
		},
	}
	cmd.Flags().BoolVar(&toRoot, "root", false, "move to the root")
	return cmd
}

func newRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "rm <id>",
		Short:             "Remove a hypothesis without children",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeIDs(nodeID),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			n, err := s.resolve(args[0])
			if err != nil {
				return err
			}
			changed, err := research.Remove(s.tree, n)
			if err != nil {
				return err
			}
			paths, err := s.save(changed...)
			if err != nil {
				return err
			}
			removed, err := s.repo.RemoveNode(n.ID)
			if err != nil {
				return err
			}
			return s.commit(fmt.Sprintf("rm %s: %s", n.ID, n.Title), append(paths, removed)...)
		},
	}
}

func newUndoCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "undo",
		Short: "Undo the last commit by adding a commit that reverses it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			actor, err := gitutil.Actor(s.repo.Root)
			if err != nil {
				return err
			}
			subject, err := gitutil.Undo(s.repo.Root, actor, force)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "undone: %s\n", subject)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "undo the last commit even if it was not made by hyp")
	return cmd
}
