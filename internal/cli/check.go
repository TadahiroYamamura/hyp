package cli

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/TadahiroYamamura/hyp/internal/gitutil"
	"github.com/TadahiroYamamura/hyp/internal/research"
)

// silentExit は、出力済みの結果を終了コードだけで伝えるときに返す。
type silentExit int

func (e silentExit) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

func newCheckCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check the integrity of the research directory (exits with 1 if errors are found)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			// 壊れたファイルがあっても一覧を出せるよう、openSession を使わずに調べる。
			repo, err := research.Find(wd)
			if err != nil {
				return err
			}
			issues := repo.Check()
			files, err := gitutil.Uncommitted(repo.Root, research.ManagedPaths...)
			if err != nil {
				return err
			}
			for _, f := range files {
				issues = append(issues, research.Issue{
					Severity: research.SeverityError,
					Path:     f,
					Message:  "uncommitted change (possibly edited without hyp)",
				})
			}
			sort.SliceStable(issues, func(i, j int) bool { return issues[i].Path < issues[j].Path })

			errs := 0
			for _, i := range issues {
				if i.Severity == research.SeverityError {
					errs++
				}
			}
			out := cmd.OutOrStdout()
			if asJSON {
				if issues == nil {
					issues = []research.Issue{}
				}
				if err := writeJSON(out, issues); err != nil {
					return err
				}
			} else {
				for _, i := range issues {
					fmt.Fprintf(out, "%-7s %s: %s\n", i.Severity, i.Path, i.Message)
				}
				fmt.Fprintf(out, "%d error(s), %d warning(s)\n", errs, len(issues)-errs)
			}
			if errs > 0 {
				return silentExit(1)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}
