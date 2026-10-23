package cli

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/TadahiroYamamura/hyp/internal/gitutil"
	"github.com/TadahiroYamamura/hyp/internal/research"
)

// 使い方の説明は研究ディレクトリに書き込まず、CLI が出力する。CLI を更新したときに説明が古いまま残らないため。
//
//go:embed prime.md
var primeText string

func newPrimeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prime",
		Short: "Print usage instructions for AI agents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), primeText)
			return err
		},
	}
}

const (
	claudeSettingsPath = ".claude/settings.json"
	primeCommand       = "hyp prime"
	agentActor         = "ai:claude"
)

// claudeDenyRules は AI エージェントに使わせない操作。研究ディレクトリからの相対パスで書く。
var claudeDenyRules = []string{
	"Bash(hyp evidence approve *)",
	// 引数なしの形と引数つきの形の両方を拒否する。
	"Bash(hyp undo --force)",
	"Bash(hyp undo --force *)",
	// Edit の規則は Write など、ファイルを書き換えるほかのツールにも効く。
	"Edit(" + research.MarkerFile + ")",
	"Edit(hypotheses/**)",
	"Edit(evidence/**)",
}

func newSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Set up integration with AI agents",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "claude",
		Short: "Write Claude Code settings (" + claudeSettingsPath + ") into the research directory",
		Long: "Adds to " + claudeSettingsPath + " a SessionStart hook running `" + primeCommand + "`, " +
			"deny rules for approving evidence, forced undo and direct edits, and HYP_ACTOR=" + agentActor + ". " +
			"Existing settings are kept.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			path := filepath.Join(s.repo.Root, claudeSettingsPath)
			settings := map[string]any{}
			b, err := os.ReadFile(path)
			if err == nil {
				if err := json.Unmarshal(b, &settings); err != nil {
					return fmt.Errorf("parse %s: %w", claudeSettingsPath, err)
				}
			} else if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if err := mergeClaudeSettings(settings); err != nil {
				return fmt.Errorf("%s: %w", claudeSettingsPath, err)
			}
			out, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return err
			}
			out = append(out, '\n')
			if string(out) == string(b) {
				fmt.Fprintf(cmd.OutOrStdout(), "%s is already set up\n", path)
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, out, 0o644); err != nil {
				return err
			}
			if err := s.commit("setup claude", "", claudeSettingsPath); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
			return nil
		},
	})
	return cmd
}

// mergeClaudeSettings は既存の設定を残したまま、hyp に必要な設定を足す。何度実行しても結果は変わらない。
func mergeClaudeSettings(settings map[string]any) error {
	env, err := object(settings, "env")
	if err != nil {
		return err
	}
	env[gitutil.ActorEnv] = agentActor

	perms, err := object(settings, "permissions")
	if err != nil {
		return err
	}
	deny, err := array(perms, "deny")
	if err != nil {
		return err
	}
	for _, rule := range claudeDenyRules {
		if !containsString(deny, rule) {
			deny = append(deny, rule)
		}
	}
	perms["deny"] = deny

	hooks, err := object(settings, "hooks")
	if err != nil {
		return err
	}
	sessionStart, err := array(hooks, "SessionStart")
	if err != nil {
		return err
	}
	if !hasCommandHook(sessionStart, primeCommand) {
		sessionStart = append(sessionStart, map[string]any{
			"hooks": []any{map[string]any{"type": "command", "command": primeCommand}},
		})
	}
	hooks["SessionStart"] = sessionStart
	return nil
}

func object(m map[string]any, key string) (map[string]any, error) {
	v, ok := m[key]
	if !ok {
		o := map[string]any{}
		m[key] = o
		return o, nil
	}
	o, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%q is not an object", key)
	}
	return o, nil
}

func array(m map[string]any, key string) ([]any, error) {
	v, ok := m[key]
	if !ok {
		return nil, nil
	}
	a, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%q is not an array", key)
	}
	return a, nil
}

func containsString(a []any, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}

func hasCommandHook(groups []any, command string) bool {
	for _, g := range groups {
		group, ok := g.(map[string]any)
		if !ok {
			continue
		}
		hs, _ := group["hooks"].([]any)
		for _, h := range hs {
			if hook, ok := h.(map[string]any); ok && hook["command"] == command {
				return true
			}
		}
	}
	return false
}
