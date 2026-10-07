package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/TadahiroYamamura/hyp/internal/research"
)

type jsonPending struct {
	Label       string              `json:"label"`
	ID          string              `json:"id"`
	Title       string              `json:"title"`
	Status      research.Status     `json:"status"`
	Weight      float64             `json:"weight"`
	IfSupported research.Conclusion `json:"if_supported"`
	IfRejected  research.Conclusion `json:"if_rejected"`
}

type jsonConclusion struct {
	Label         string              `json:"label"`
	ID            string              `json:"id"`
	Title         string              `json:"title"`
	Conclusion    research.Conclusion `json:"conclusion"`
	SupportedWhen string              `json:"supported_when"`
	RejectedWhen  string              `json:"rejected_when"`
	Progress      float64             `json:"progress"`
	// cases のときだけ出力する。
	SupportedShare *float64      `json:"supported_share,omitempty"`
	RejectedShare  *float64      `json:"rejected_share,omitempty"`
	Pending        []jsonPending `json:"pending"`
	Warnings       []string      `json:"warnings"`
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
				Label:          s.tree.Label(n.ID),
				ID:             n.ID,
				Title:          n.Title,
				Conclusion:     r.Conclusion(),
				SupportedWhen:  r.SupportedWhen.String(),
				RejectedWhen:   r.RejectedWhen.String(),
				Progress:       r.Progress,
				SupportedShare: r.SupportedShare,
				RejectedShare:  r.RejectedShare,
				Pending:        []jsonPending{},
				Warnings:       append([]string{}, r.Warnings...),
			}
			for _, p := range r.Pending {
				j.Pending = append(j.Pending, jsonPending{
					Label: s.tree.Label(p.Node.ID), ID: p.Node.ID, Title: p.Node.Title, Status: p.Node.Status,
					Weight: p.Weight, IfSupported: p.IfSupported, IfRejected: p.IfRejected,
				})
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
			fmt.Fprintf(out, "progress: %s judged (not the probability that the hypothesis is true)\n", percent(j.Progress))
			if j.SupportedShare != nil {
				fmt.Fprintf(out, "cases: supported in %s, rejected in %s\n", percent(*j.SupportedShare), percent(*j.RejectedShare))
			}
			if len(j.Pending) > 0 {
				fmt.Fprintln(out, "pending (by weight; conclusion if only this one is judged):")
				for _, p := range j.Pending {
					fmt.Fprintf(out, "  %s  %s  [%s] %s  weight %s  if supported: %s, if rejected: %s\n",
						p.Label, p.ID, p.Status, p.Title, percent(p.Weight), p.IfSupported, p.IfRejected)
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

func percent(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }
