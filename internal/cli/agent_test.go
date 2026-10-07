package cli

import (
	"encoding/json"
	"testing"
)

func mergeJSON(t *testing.T, in string) (string, error) {
	t.Helper()
	settings := map[string]any{}
	if err := json.Unmarshal([]byte(in), &settings); err != nil {
		t.Fatal(err)
	}
	if err := mergeClaudeSettings(settings); err != nil {
		return "", err
	}
	b, _ := json.Marshal(settings)
	return string(b), nil
}

func normalize(t *testing.T, s string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

const hypDeny = `"Bash(hyp evidence approve *)","Bash(hyp undo --force)","Bash(hyp undo --force *)","Edit(hyp.yaml)","Edit(hypotheses/**)","Edit(evidence/**)"`

const primeHook = `{"hooks":[{"type":"command","command":"hyp prime"}]}`

func TestMergeClaudeSettings(t *testing.T) {
	t.Run("設定がなければhypに必要な設定だけを作る", func(t *testing.T) {
		got, err := mergeJSON(t, `{}`)
		if err != nil {
			t.Fatal(err)
		}
		want := normalize(t, `{
			"env": {"HYP_ACTOR": "ai:claude"},
			"permissions": {"deny": [`+hypDeny+`]},
			"hooks": {"SessionStart": [`+primeHook+`]}
		}`)
		if got != want {
			t.Errorf("got  %s\nwant %s", got, want)
		}
	})
	t.Run("既存の設定を残したまま足す", func(t *testing.T) {
		got, err := mergeJSON(t, `{
			"model": "opus",
			"env": {"FOO": "1", "HYP_ACTOR": "human:x"},
			"permissions": {"allow": ["Bash(ls *)"], "deny": ["Bash(rm *)", "Edit(evidence/**)"]},
			"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "echo hi"}]}], "Stop": []}
		}`)
		if err != nil {
			t.Fatal(err)
		}
		want := normalize(t, `{
			"model": "opus",
			"env": {"FOO": "1", "HYP_ACTOR": "ai:claude"},
			"permissions": {"allow": ["Bash(ls *)"], "deny": ["Bash(rm *)", "Edit(evidence/**)",
				"Bash(hyp evidence approve *)","Bash(hyp undo --force)","Bash(hyp undo --force *)","Edit(hyp.yaml)","Edit(hypotheses/**)"]},
			"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "echo hi"}]}, `+primeHook+`], "Stop": []}
		}`)
		if got != want {
			t.Errorf("got  %s\nwant %s", got, want)
		}
	})
	t.Run("二度実行しても設定は重複しない", func(t *testing.T) {
		once, _ := mergeJSON(t, `{}`)
		twice, err := mergeJSON(t, once)
		if err != nil {
			t.Fatal(err)
		}
		if once != twice {
			t.Errorf("once  %s\ntwice %s", once, twice)
		}
	})
	for _, in := range []string{`{"env": []}`, `{"permissions": {"deny": "x"}}`, `{"hooks": {"SessionStart": {}}}`} {
		t.Run("想定と違う型の設定はエラーにして書き換えない: "+in, func(t *testing.T) {
			if _, err := mergeJSON(t, in); err == nil {
				t.Fatal("want error")
			}
		})
	}
}
