package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TadahiroYamamura/hyp/internal/research"
)

func newEvidenceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "evidence",
		Short: "Manage evidence for hypotheses",
	}
	cmd.AddCommand(
		newEvidenceAddCmd(),
		newEvidenceLinkCmd(),
		newEvidenceUnlinkCmd(),
		newEvidenceApproveCmd(),
		newEvidenceRmCmd(),
		newEvidenceShowCmd(),
	)
	return cmd
}

func registerStanceFlag(cmd *cobra.Command, stance *string) {
	cmd.Flags().StringVar(stance, "stance", "", "whether the evidence supports, refutes or is inconclusive for the hypothesis (required)")
	_ = cmd.MarkFlagRequired("stance")
	_ = cmd.RegisterFlagCompletionFunc("stance", fixedCompletion(research.Stances))
}

func newEvidenceAddCmd() *cobra.Command {
	var summary, source, foundBy, stance string
	var files []string
	cmd := &cobra.Command{
		Use:               "add <hypothesis> --summary <text> --stance <stance> [--file <path>]... [--source <url>]",
		Short:             "Create evidence, copy attached files into it, and link it to a hypothesis",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeIDs(nodeID),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := research.ParseStance(stance)
			if err != nil {
				return err
			}
			if summary == "" {
				return fmt.Errorf("summary must not be empty")
			}
			s, err := openSession()
			if err != nil {
				return err
			}
			n, err := s.resolve(args[0])
			if err != nil {
				return err
			}
			if foundBy == "" {
				if foundBy, err = s.actor(); err != nil {
					return err
				}
			}
			ev := &research.Evidence{Summary: summary, Source: source, FoundBy: foundBy, CreatedAt: now()}
			paths, err := s.repo.CreateEvidence(ev, files)
			if err != nil {
				return err
			}
			if err := research.Link(n, ev.ID, st); err != nil {
				return err
			}
			nodePaths, err := s.save(n)
			if err != nil {
				return err
			}
			if err := s.commit(fmt.Sprintf("evidence add %s to %s: %s", ev.ID, n.ID, ev.Summary), append(paths, nodePaths...)...); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), ev.ID)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&summary, "summary", "", "what the evidence shows (required)")
	_ = cmd.MarkFlagRequired("summary")
	f.StringVar(&source, "source", "", "URL of the source")
	f.StringArrayVar(&files, "file", nil, "file to attach (copied into the evidence directory; repeatable)")
	f.StringVar(&foundBy, "found-by", "", "who found the evidence, as ai:<model> or human:<name> (default: the current actor)")
	registerStanceFlag(cmd, &stance)
	return cmd
}

func newEvidenceLinkCmd() *cobra.Command {
	var stance string
	cmd := &cobra.Command{
		Use:               "link <hypothesis> <evidence> --stance <stance>",
		Short:             "Link existing evidence to a hypothesis, or change the stance of an existing link",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeIDs(nodeID, evidenceID),
		RunE: func(_ *cobra.Command, args []string) error {
			st, err := research.ParseStance(stance)
			if err != nil {
				return err
			}
			s, err := openSession()
			if err != nil {
				return err
			}
			n, err := s.resolve(args[0])
			if err != nil {
				return err
			}
			ev, err := research.ResolveEvidence(s.evidences, args[1])
			if err != nil {
				return err
			}
			if err := research.Link(n, ev.ID, st); err != nil {
				return err
			}
			paths, err := s.save(n)
			if err != nil {
				return err
			}
			return s.commit(fmt.Sprintf("evidence link %s to %s as %s", ev.ID, n.ID, st), paths...)
		},
	}
	registerStanceFlag(cmd, &stance)
	return cmd
}

func newEvidenceApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "approve <evidence>",
		Short:             "Approve evidence after checking it (humans only)",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeIDs(evidenceID),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			ev, err := research.ResolveEvidence(s.evidences, args[0])
			if err != nil {
				return err
			}
			actor, err := s.actor()
			if err != nil {
				return err
			}
			files, err := s.repo.HashEvidenceFiles(ev)
			if err != nil {
				return err
			}
			if err := research.Approve(ev, actor, files, now()); err != nil {
				return err
			}
			path, err := s.repo.SaveEvidence(ev)
			if err != nil {
				return err
			}
			return s.commit(fmt.Sprintf("evidence approve %s: %s", ev.ID, ev.Summary), path)
		},
	}
}

func newEvidenceUnlinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "unlink <hypothesis> <evidence>",
		Short:             "Unlink evidence from a hypothesis (the evidence itself is kept)",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeIDs(nodeID, evidenceID),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			n, err := s.resolve(args[0])
			if err != nil {
				return err
			}
			ev, err := research.ResolveEvidence(s.evidences, args[1])
			if err != nil {
				return err
			}
			stance, err := research.Unlink(n, ev.ID)
			if err != nil {
				return err
			}
			paths, err := s.save(n)
			if err != nil {
				return err
			}
			// 紐づけ直すときに必要になるので、外した stance をコミットメッセージに残す。
			return s.commit(fmt.Sprintf("evidence unlink %s (was %s) from %s", ev.ID, stance, n.ID), paths...)
		},
	}
}

func newEvidenceRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "rm <evidence>",
		Short:             "Remove evidence and its attached files (only when no hypothesis links to it)",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeIDs(evidenceID),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			ev, err := research.ResolveEvidence(s.evidences, args[0])
			if err != nil {
				return err
			}
			if linked := research.LinkedFrom(s.nodes, ev.ID); len(linked) > 0 {
				ids := make([]string, len(linked))
				for i, n := range linked {
					ids[i] = n.ID
				}
				return fmt.Errorf("evidence %s is linked from %s; unlink it first", ev.ID, strings.Join(ids, ", "))
			}
			path, err := s.repo.RemoveEvidence(ev)
			if err != nil {
				return err
			}
			return s.commit(fmt.Sprintf("evidence rm %s: %s", ev.ID, ev.Summary), path)
		},
	}
}

type jsonLinkedFrom struct {
	Label  string          `json:"label"`
	ID     string          `json:"id"`
	Title  string          `json:"title"`
	Stance research.Stance `json:"stance"`
}

type jsonEvidence struct {
	*research.Evidence
	Approval   research.ApprovalState `json:"approval"`
	LinkedFrom []jsonLinkedFrom       `json:"linked_from"`
}

func newEvidenceShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:               "show <evidence>",
		Short:             "Show evidence, its approval state and the hypotheses linking to it",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeIDs(evidenceID),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			ev, err := research.ResolveEvidence(s.evidences, args[0])
			if err != nil {
				return err
			}
			j := jsonEvidence{Evidence: ev, LinkedFrom: []jsonLinkedFrom{}}
			if files, err := s.repo.HashEvidenceFiles(ev); err == nil {
				j.Approval = research.Approval(ev, files)
			} else {
				// 添付が読めなくても根拠の中身は確認できるよう、表示は続ける。
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: cannot read attached files:", err)
				j.Approval = research.ApprovalNone
				if ev.ApprovedFingerprint != "" {
					j.Approval = research.ApprovalOutdated
				}
			}
			for _, n := range research.LinkedFrom(s.nodes, ev.ID) {
				for _, l := range n.Evidence {
					if l.ID == ev.ID {
						j.LinkedFrom = append(j.LinkedFrom, jsonLinkedFrom{Label: s.tree.Label(n.ID), ID: n.ID, Title: n.Title, Stance: l.Stance})
					}
				}
			}
			out := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(out, j)
			}
			fmt.Fprintf(out, "%s  %s\n", ev.ID, ev.Summary)
			if ev.Source != "" {
				fmt.Fprintf(out, "source:   %s\n", ev.Source)
			}
			fmt.Fprintf(out, "found by: %s\n", ev.FoundBy)
			switch j.Approval {
			case research.ApprovalNone:
				fmt.Fprintln(out, "approval: not approved")
			case research.ApprovalCurrent:
				fmt.Fprintf(out, "approval: approved by %s at %s\n", ev.ApprovedBy, ev.ApprovedAt.Format("2006-01-02 15:04"))
			case research.ApprovalOutdated:
				fmt.Fprintf(out, "approval: CHANGED since approved by %s at %s\n", ev.ApprovedBy, ev.ApprovedAt.Format("2006-01-02 15:04"))
			}
			if len(ev.Files) > 0 {
				fmt.Fprintln(out, "files:")
				for _, f := range ev.Files {
					fmt.Fprintf(out, "  %s\n", s.repo.EvidenceFilePath(ev.ID, f.Path))
				}
			}
			fmt.Fprintln(out, "linked from:")
			for _, l := range j.LinkedFrom {
				fmt.Fprintf(out, "  %s  %s  [%s] %s\n", l.Label, l.ID, l.Stance, l.Title)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}
