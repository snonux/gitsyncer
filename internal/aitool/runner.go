package aitool

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runner executes a single AI CLI tool given an instructional prompt and an
// optional stdin payload, returning the tool's trimmed textual response (or
// an error describing why it couldn't produce one). Each concrete
// implementation in runners.go knows its own tool's calling convention
// (single positional argument vs. argument-plus-piped-stdin), so callers
// never need to branch on which tool they're talking to.
type Runner interface {
	Run(prompt, stdin string) (string, error)
}

// runnerFactory builds a Runner rooted at a working directory. Agentic CLIs
// such as claude and amp can inspect files in their working directory on
// their own initiative, so callers that want that (e.g. showcase summaries,
// which pass the repo path) benefit from it; callers that already have all
// the context they need in prompt/stdin (e.g. release notes, built from a
// diff) pass "" and get the process's own working directory instead.
type runnerFactory func(dir string) Runner

// registry is the single source of truth mapping each Tool to how it is
// invoked. Both the release-notes and showcase-summary flows go through
// NewRunner/RunChain instead of hand-rolling a type switch (release) or a
// raw-string switch (showcase) over these same four tools, so adding a new
// AI tool now means adding one entry here rather than touching both
// packages.
var registry = map[Tool]runnerFactory{
	ToolOpencode: func(dir string) Runner { return opencodeRunner{dir: dir} },
	ToolHexAI:    func(dir string) Runner { return hexaiRunner{dir: dir} },
	ToolClaude:   func(dir string) Runner { return claudeRunner{dir: dir} },
	ToolAmp:      func(dir string) Runner { return ampRunner{dir: dir} },
}

// NewRunner returns the Runner registered for tool, rooted at dir ("" uses
// the calling process's current working directory). It returns nil for a
// Tool with no registered implementation, which callers should treat as
// "skip this tool" rather than panicking.
func NewRunner(tool Tool, dir string) Runner {
	factory, ok := registry[tool]
	if !ok {
		return nil
	}

	return factory(dir)
}

// RunChain tries each tool in chain, in order, stopping at the first one
// that returns non-empty output. It centralizes the "iterate the available
// tools, log and skip failures, fall through to the next" loop that both
// release-notes generation and showcase-summary generation need, so neither
// package has to know how any individual tool is invoked. dir is forwarded
// to NewRunner for every tool (see runnerFactory for what it controls).
func RunChain(chain []Tool, dir, prompt, stdin string) (output string, used Tool, err error) {
	for _, tool := range chain {
		runner := NewRunner(tool, dir)
		if runner == nil {
			continue
		}

		out, runErr := runner.Run(prompt, stdin)
		if runErr != nil {
			fmt.Printf("  %s failed: %v\n", tool, runErr)
			continue
		}

		return out, tool, nil
	}

	return "", "", fmt.Errorf("all AI tools failed")
}

// runExec runs cmd and returns its trimmed stdout only. stderr is routed to
// the calling process's own os.Stderr rather than captured: some of these
// CLIs (e.g. claude with CLAUDE_DEBUG=1) write diagnostic/debug logging to
// stderr, and an earlier version of this code used CombinedOutput, which
// merged that debug noise into the "output" that then got used verbatim as
// release notes or a showcase summary. Keeping stderr separate means the
// operator running the command still sees it live, but it never ends up in
// the captured result. It treats a non-zero exit, empty output, or an
// in-band error message (a common way these CLIs report failure while still
// exiting 0) as a returned error rather than a successful-but-useless
// result, so callers can uniformly react with "try the next tool" regardless
// of how a given tool fails.
func runExec(cmd *exec.Cmd, toolName string) (string, error) {
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s command failed: %w. Output: %s", toolName, err, string(output))
	}

	content := strings.TrimSpace(string(output))
	if content == "" {
		return "", fmt.Errorf("received empty output from %s", toolName)
	}

	if strings.HasPrefix(content, "Error:") ||
		strings.Contains(content, "API Error") ||
		strings.Contains(content, "authentication_error") {
		return "", fmt.Errorf("%s returned an error: %s", toolName, content)
	}

	return content, nil
}

// combinedPrompt joins prompt and stdin for tools that only accept a single
// positional argument in our usage (opencode, claude). Tools that accept a
// separate piped payload (hexai, amp) keep prompt and stdin apart instead;
// see runners.go.
func combinedPrompt(prompt, stdin string) string {
	if stdin == "" {
		return prompt
	}

	return prompt + "\n\n" + stdin
}
