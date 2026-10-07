package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/TadahiroYamamura/hyp/internal/research"
)

type jsonPending struct {
	Label  string          `json:"label"`
	ID     string          `json:"id"`
	Title  string          `json:"title"`
	Status research.Status `json:"status"`
}

type jsonConclusion struct {
	Label         string              `json:"label"`
	ID            string              `json:"id"`
	Title         string              `json:"title"`
	Conclusion    research.Conclusion `json:"conclusion"`
	SupportedWhen string              `json:"supported_when"`
	RejectedWhen  string              `json:"rejected_when"`
	Pending       []jsonPending       `json:"pending"`
	Warnings      []string            `json:"warnings"`
}

func newConcludeCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:               "conclude <id>",
		Short:             "Derive the conclusion of a hypothesis from the statuses of its leaves",
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
			r := research.Conclude(s.tree, n)
			j := jsonConclusion{
				Label:         s.tree.Label(n.ID),
				ID:            n.ID,
				Title:         n.Title,
				Conclusion:    r.Conclusion(),
				SupportedWhen: r.SupportedWhen.String(),
				RejectedWhen:  r.RejectedWhen.String(),
				Pending:       []jsonPending{},
				Warnings:      append([]string{}, r.Warnings...),
			}
			for _, p := range r.Pending {
				j.Pending = append(j.Pending, jsonPending{Label: s.tree.Label(p.ID), ID: p.ID, Title: p.Title, Status: p.Status})
			}
			out := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(out, j)
			}
			fmt.Fprintf(out, "%s  %s  %s\n", j.Label, j.ID, j.Title)
			switch j.Conclusion {
			case research.ConclusionConditional:
				fmt.Fprintln(out, "conclusion: conditional")
				if !r.SupportedWhen.IsFalse() {
					fmt.Fprintf(out, "  supported when %s\n", r.SupportedWhen)
				}
				if !r.RejectedWhen.IsFalse() {
					fmt.Fprintf(out, "  rejected when %s\n", r.RejectedWhen)
				}
				fmt.Fprintln(out, "  undetermined otherwise")
			default:
				fmt.Fprintf(out, "conclusion: %s\n", j.Conclusion)
			}
			if len(j.Pending) > 0 {
				fmt.Fprintln(out, "pending:")
				for _, p := range j.Pending {
					fmt.Fprintf(out, "  %s  %s  [%s] %s\n", p.Label, p.ID, p.Status, p.Title)
				}
			}
			if len(j.Warnings) > 0 {
				fmt.Fprintln(out, "warnings:")
				for _, w := range j.Warnings {
					fmt.Fprintf(out, "  %s\n", w)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}
