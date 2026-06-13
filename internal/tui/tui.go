// Package tui implements the interactive, Copilot-style scrolling REPL built on the
// Charm stack (Bubble Tea + Bubbles + Lipgloss + Glamour). Finalized content is
// printed above the live region with tea.Println so it scrolls naturally; only the
// input box, spinner, in-progress streaming text and confirmation prompt are "live".
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"calcassist/internal/agent"
	"calcassist/internal/tools"
)

// ----- styles -----

var (
	userStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	assistStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	toolStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
	confirmStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
)

// ----- async messages from the agent goroutine -----

type agentTextMsg string
type agentToolCallMsg struct{ name, args string }
type agentToolResultMsg struct {
	name, result string
	err          error
}
type agentConfirmMsg struct {
	tool  tools.Tool
	args  string
	reply chan bool
}
type agentDoneMsg struct {
	text string
	err  error
}

type uiState int

const (
	stateIdle uiState = iota
	stateBusy
	stateConfirm
)

type model struct {
	ag         *agent.Agent
	registry   *tools.Registry
	cfgSummary string
	version    string

	input    textinput.Model
	spinner  spinner.Model
	renderer *glamour.TermRenderer
	width    int

	state     uiState
	streamBuf string
	status    string

	events  chan tea.Msg
	confirm *agentConfirmMsg

	ctx    context.Context
	cancel context.CancelFunc
}

// Run starts the interactive REPL. configSummary is a redacted, printable summary of
// the active configuration (shown by /config).
func Run(ag *agent.Agent, registry *tools.Registry, configSummary, version string) error {
	ti := textinput.New()
	ti.Placeholder = "Ask me to calculate, read files, convert Excel… (/help, /exit)"
	ti.Prompt = userStyle.Render("you ❯ ")
	ti.CharLimit = 0
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = toolStyle

	rnd, _ := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(80))

	m := model{
		ag:         ag,
		registry:   registry,
		cfgSummary: configSummary,
		version:    version,
		input:      ti,
		spinner:    sp,
		renderer:   rnd,
		width:      80,
		state:      stateIdle,
		events:     make(chan tea.Msg, 128),
	}

	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, printBanner(m.version))
}

func printBanner(version string) tea.Cmd {
	banner := dimStyle.Render(fmt.Sprintf("CalcAssist %s — type a request, or /help for commands. Ctrl+C to quit.", version))
	return tea.Println(banner)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.input.Width = max(10, msg.Width-8)
		wrap := max(20, msg.Width-2)
		if r, err := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(wrap)); err == nil {
			m.renderer = r
		}
		return m, nil

	case spinner.TickMsg:
		if m.state == stateIdle {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		return m.handleKey(msg)

	case agentTextMsg:
		m.streamBuf += string(msg)
		return m, waitForEvent(m.events)

	case agentToolCallMsg:
		cmds := []tea.Cmd{}
		if strings.TrimSpace(m.streamBuf) != "" {
			cmds = append(cmds, tea.Println(m.render(m.streamBuf)))
			m.streamBuf = ""
		}
		cmds = append(cmds, tea.Println(toolStyle.Render("⚙ "+msg.name)+dimStyle.Render(" "+compactArgs(msg.args))))
		m.status = "running " + msg.name + "…"
		cmds = append(cmds, waitForEvent(m.events))
		return m, tea.Batch(cmds...)

	case agentToolResultMsg:
		var line string
		if msg.err != nil {
			line = errStyle.Render("  ✗ "+msg.name) + dimStyle.Render(" "+truncate(msg.err.Error(), 200))
		} else {
			line = dimStyle.Render("  ✓ " + msg.name + " → " + truncate(oneLine(msg.result), 200))
		}
		return m, tea.Batch(tea.Println(line), waitForEvent(m.events))

	case agentConfirmMsg:
		m.confirm = &msg
		m.state = stateConfirm
		m.status = ""
		return m, waitForEvent(m.events)

	case agentDoneMsg:
		cmds := []tea.Cmd{}
		final := msg.text
		if strings.TrimSpace(final) == "" {
			final = m.streamBuf
		}
		if msg.err != nil {
			cmds = append(cmds, tea.Println(errStyle.Render("error: ")+msg.err.Error()))
		} else if strings.TrimSpace(final) != "" {
			cmds = append(cmds, tea.Println(m.render(final)))
		}
		m.streamBuf = ""
		m.status = ""
		m.state = stateIdle
		m.input.Focus()
		cmds = append(cmds, textinput.Blink)
		return m, tea.Batch(cmds...)
	}

	// Default: feed other messages (e.g. cursor blink) to the input when idle.
	if m.state == stateIdle {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateConfirm:
		switch msg.String() {
		case "y", "Y":
			m.answerConfirm(true)
			m.state = stateBusy
			return m, m.spinner.Tick
		case "ctrl+c":
			m.answerConfirm(false)
			return m, tea.Quit
		default: // n, N, esc, enter, anything else -> decline
			m.answerConfirm(false)
			m.state = stateBusy
			return m, m.spinner.Tick
		}

	case stateBusy:
		if msg.String() == "ctrl+c" {
			if m.cancel != nil {
				m.cancel()
			}
		}
		return m, nil

	default: // stateIdle
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			return m.submit()
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}
}

func (m *model) answerConfirm(ok bool) {
	if m.confirm != nil {
		m.confirm.reply <- ok
		m.confirm = nil
	}
}

func (m model) submit() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	m.input.Reset()
	if text == "" {
		return m, nil
	}
	if strings.HasPrefix(text, "/") {
		return m.handleCommand(text)
	}

	// Echo the user's line into the scrollback, then start the agent run.
	echo := tea.Println(userStyle.Render("you ❯ ") + text)

	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.state = stateBusy
	m.streamBuf = ""
	m.status = "thinking…"

	events := m.events
	ag := m.ag
	ctx := m.ctx
	hooks := agent.Hooks{
		OnText:       func(s string) { events <- agentTextMsg(s) },
		OnToolCall:   func(name, args string) { events <- agentToolCallMsg{name, args} },
		OnToolResult: func(name, result string, err error) { events <- agentToolResultMsg{name, result, err} },
		Confirm: func(t tools.Tool, args string) (bool, error) {
			reply := make(chan bool, 1)
			events <- agentConfirmMsg{tool: t, args: args, reply: reply}
			return <-reply, nil
		},
	}
	go func() {
		out, err := ag.Run(ctx, text, hooks)
		events <- agentDoneMsg{text: out, err: err}
	}()

	return m, tea.Batch(echo, m.spinner.Tick, waitForEvent(m.events))
}

func (m model) handleCommand(cmd string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(cmd)
	switch fields[0] {
	case "/exit", "/quit", "/q":
		return m, tea.Quit
	case "/help":
		return m, tea.Println(helpText())
	case "/tools":
		return m, tea.Println(m.toolsText())
	case "/config":
		return m, tea.Println(dimStyle.Render(m.cfgSummary))
	case "/clear":
		m.ag.Reset()
		return m, tea.Println(dimStyle.Render("(conversation history cleared)"))
	default:
		return m, tea.Println(errStyle.Render("unknown command: ") + fields[0] + dimStyle.Render("  (try /help)"))
	}
}

func (m model) View() string {
	switch m.state {
	case stateBusy:
		head := m.spinner.View() + " " + dimStyle.Render(m.status)
		if strings.TrimSpace(m.streamBuf) != "" {
			return head + "\n" + assistStyle.Render(m.streamBuf)
		}
		return head
	case stateConfirm:
		return m.confirmView()
	default:
		return m.input.View()
	}
}

func (m model) confirmView() string {
	if m.confirm == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(confirmStyle.Render(fmt.Sprintf("Allow tool %q to run?", m.confirm.tool.Name())) + "\n")
	b.WriteString(describeArgs(m.confirm.args))
	b.WriteString("\n" + confirmStyle.Render("[y]es / [N]o ❯ "))
	return b.String()
}

// render turns assistant markdown into styled terminal text, trimming trailing space.
func (m model) render(md string) string {
	if m.renderer == nil {
		return assistStyle.Render(md)
	}
	out, err := m.renderer.Render(md)
	if err != nil {
		return assistStyle.Render(md)
	}
	return strings.TrimRight(out, "\n")
}

func (m model) toolsText() string {
	var b strings.Builder
	b.WriteString(toolStyle.Render(fmt.Sprintf("Available tools (%d):", m.registry.Len())) + "\n")
	for _, t := range m.registry.All() {
		marker := ""
		if t.Mutating() {
			marker = dimStyle.Render(" [writes]")
		}
		b.WriteString(fmt.Sprintf("  %s%s — %s\n", toolStyle.Render(t.Name()), marker, t.Description()))
	}
	return strings.TrimRight(b.String(), "\n")
}

func waitForEvent(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

// ----- formatting helpers -----

func helpText() string {
	return toolStyle.Render("Commands:") + "\n" +
		"  /help            show this help\n" +
		"  /tools           list available tools\n" +
		"  /config          show the active (redacted) configuration\n" +
		"  /clear           clear the conversation history\n" +
		"  /exit            quit\n" +
		dimStyle.Render("Otherwise, just type what you want — e.g. \"convert sales.xlsx to JSON and save it\".")
}

func compactArgs(args string) string {
	var v any
	if err := json.Unmarshal([]byte(args), &v); err != nil {
		return truncate(oneLine(args), 120)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return truncate(oneLine(args), 120)
	}
	return truncate(string(b), 120)
}

// describeArgs renders a friendly preview of tool arguments for the confirm prompt,
// highlighting a path and content snippet when present.
func describeArgs(args string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(args), &m); err != nil {
		return dimStyle.Render("  " + truncate(oneLine(args), 200))
	}
	var b strings.Builder
	if p, ok := m["path"].(string); ok {
		b.WriteString(dimStyle.Render("  path: ") + p + "\n")
	}
	if c, ok := m["content"].(string); ok {
		lines := strings.Split(c, "\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("  content: %d bytes, %d line(s)", len(c), len(lines))) + "\n")
		preview := lines
		if len(preview) > 8 {
			preview = preview[:8]
		}
		for _, ln := range preview {
			b.WriteString(dimStyle.Render("  │ ") + truncate(ln, 100) + "\n")
		}
		if len(lines) > 8 {
			b.WriteString(dimStyle.Render("  │ …\n"))
		}
	}
	if b.Len() == 0 {
		return dimStyle.Render("  " + truncate(compactArgs(args), 200))
	}
	return strings.TrimRight(b.String(), "\n")
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
