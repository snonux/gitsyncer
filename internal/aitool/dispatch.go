package aitool

import "os/exec"

type Tool string

const (
	ToolPi       Tool = "pi"
	ToolOpencode Tool = "opencode"
	ToolHexAI    Tool = "hexai"
	ToolClaude   Tool = "claude"
	ToolAmp      Tool = "amp"
)

type LookPathFunc func(file string) (string, error)

func Chain(preferred string) []Tool {
	switch preferred {
	case "", string(ToolPi), "openrouter", "pi-openrouter":
		return []Tool{ToolPi, ToolOpencode, ToolHexAI, ToolClaude, ToolAmp}
	case string(ToolOpencode):
		return []Tool{ToolOpencode, ToolHexAI, ToolClaude, ToolAmp}
	case string(ToolHexAI):
		return []Tool{ToolHexAI, ToolClaude, ToolAmp}
	case string(ToolClaude), "claude-code":
		return []Tool{ToolClaude, ToolAmp}
	case string(ToolAmp):
		return []Tool{ToolAmp}
	default:
		return nil
	}
}

func FirstAvailable(preferred string, lookPath LookPathFunc) Tool {
	for _, tool := range Chain(preferred) {
		if IsAvailable(tool, lookPath) {
			return tool
		}
	}

	return ""
}

func AvailableChain(preferred string, lookPath LookPathFunc) []Tool {
	chain := Chain(preferred)
	available := make([]Tool, 0, len(chain))
	for _, tool := range chain {
		if IsAvailable(tool, lookPath) {
			available = append(available, tool)
		}
	}

	return available
}

func IsAvailable(tool Tool, lookPath LookPathFunc) bool {
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	binary, ok := availabilityBinary(tool)
	if !ok {
		return false
	}

	_, err := lookPath(binary)
	return err == nil
}

func availabilityBinary(tool Tool) (string, bool) {
	switch tool {
	case ToolPi:
		return "pi", true
	case ToolOpencode:
		return "ollama", true
	case ToolHexAI, ToolClaude, ToolAmp:
		return string(tool), true
	default:
		return "", false
	}
}
