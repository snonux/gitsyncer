package aitool

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// opencodeRunner drives ollama's opencode agent. It only accepts a single
// positional prompt (no piped-stdin channel in our usage), so prompt and
// stdin are combined into one argument before invocation.
type opencodeRunner struct{ dir string }

func (r opencodeRunner) Run(prompt, stdin string) (string, error) {
	fmt.Println("  Running ollama launch opencode ...")

	cmd := exec.Command("ollama", "launch", "opencode", "--model", "glm-5.2:cloud", "-y", "--", "run", combinedPrompt(prompt, stdin))
	cmd.Dir = r.dir

	return runExec(cmd, "opencode")
}

// hexaiRunner drives the hexai CLI, which takes the instructional prompt as
// its argument and reads any additional payload from stdin.
type hexaiRunner struct{ dir string }

func (r hexaiRunner) Run(prompt, stdin string) (string, error) {
	fmt.Println("  Running hexai CLI command (stdin payload)...")

	cmd := exec.Command("hexai", prompt)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	cmd.Dir = r.dir

	return runExec(cmd, "hexai")
}

// claudeRunner drives the claude CLI. Like opencode it only takes a single
// positional prompt, so prompt and stdin are combined; CLAUDE_DEBUG=1 makes
// failures more diagnosable by turning on debug logging. That logging goes
// to stderr, which runExec routes to the process's own os.Stderr instead of
// capturing it, so it's visible to whoever is watching this run without
// polluting the stdout that becomes release notes or a showcase summary.
type claudeRunner struct{ dir string }

func (r claudeRunner) Run(prompt, stdin string) (string, error) {
	fmt.Println("  Running claude CLI command...")

	cmd := exec.Command("claude", "--model", "sonnet", combinedPrompt(prompt, stdin))
	cmd.Env = append(os.Environ(), "CLAUDE_DEBUG=1")
	cmd.Dir = r.dir

	return runExec(cmd, "claude")
}

// ampRunner drives the amp CLI, which -- like hexai -- takes the
// instructional prompt as its argument and reads any additional payload
// from stdin.
type ampRunner struct{ dir string }

func (r ampRunner) Run(prompt, stdin string) (string, error) {
	fmt.Println("  Running amp CLI command (stdin payload)...")

	cmd := exec.Command("amp", "--execute", prompt)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	cmd.Dir = r.dir

	return runExec(cmd, "amp")
}
