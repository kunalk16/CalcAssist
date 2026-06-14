// Command calcassist is the entry point for the CalcAssist terminal AI agent. It
// parses flags, loads configuration, wires the LLM provider, tool registry and agent
// together, and then either runs a single prompt (one-shot, -p) or starts the
// interactive REPL.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"calcassist/internal/agent"
	"calcassist/internal/config"
	"calcassist/internal/llm"
	"calcassist/internal/tools"
	"calcassist/internal/tui"
	"calcassist/internal/version"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	var (
		configPath string
		prompt     string
	)

	cmd := &cobra.Command{
		Use:           "calcassist",
		Short:         "CalcAssist — a terminal AI agent for calculation and file tasks",
		Version:       version.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			provider, err := llm.New(toLLMConfig(cfg))
			if err != nil {
				return err
			}

			registry := tools.Default()
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			ag := agent.New(provider, registry, agent.SystemPrompt(cwd), cfg.MaxToolIterations)

			if prompt != "" {
				return runOneShot(cmd.Context(), ag, prompt)
			}
			return tui.Run(ag, registry, cfg.String(), version.Version)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "path to config file (default ~/.calcassist/config.yaml)")
	cmd.Flags().StringVarP(&prompt, "prompt", "p", "", "run a single prompt and exit (one-shot mode)")
	cmd.SetVersionTemplate("{{.Version}}\n")
	return cmd
}

// toLLMConfig maps the application configuration into the provider-neutral llm.Config.
// The config package never imports llm, so this mapping lives in main.
func toLLMConfig(c *config.Config) llm.Config {
	return llm.Config{
		Provider:    c.Provider,
		Model:       c.Model,
		APIKey:      c.APIKey,
		BaseURL:     c.BaseURL,
		MaxTokens:   c.MaxTokens,
		Temperature: c.Temperature,
		WebSearch:   c.WebSearch,
	}
}

// runOneShot executes a single prompt non-interactively: the final answer is written
// to stdout while tool activity is reported on stderr.
func runOneShot(ctx context.Context, ag *agent.Agent, prompt string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	hooks := agent.Hooks{
		OnToolCall: func(name, args string) {
			fmt.Fprintf(os.Stderr, "⚙ %s %s\n", name, args)
		},
		OnToolResult: func(name string, _ string, err error) {
			if err != nil {
				fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", name, err)
				return
			}
			fmt.Fprintf(os.Stderr, "  ✓ %s\n", name)
		},
	}

	out, err := ag.Run(ctx, prompt, hooks)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, out)
	return nil
}
