package bootstrap

import "testing"

func TestSelectedAgentNameDefaultsToMultiAgent(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"":             "multi_agent",
		"unknown":      "multi_agent",
		"multi":        "multi_agent",
		"multi-agent":  "multi_agent",
		"multi_agent":  "multi_agent",
		"tool":         "tool_calling_agent",
		"tool-calling": "tool_calling_agent",
		"tool_calling": "tool_calling_agent",
		"default":      "default_agent",
		"fixed":        "default_agent",
		"rule":         "default_agent",
	}

	for mode, want := range cases {
		mode := mode
		want := want
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			if got := selectedAgentName(mode); got != want {
				t.Fatalf("selectedAgentName(%q) = %q, want %q", mode, got, want)
			}
		})
	}
}
