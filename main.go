package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type commandSpec struct {
	label        string
	description  string
	baseArgs     []string
	requiredArgs []string
	optionalArg  string
}

type runResultMsg struct {
	command string
	output  string
	err     error
}

type runStartedMsg struct {
	command string
}

type model struct {
	width           int
	height          int
	ready           bool
	devtunnelFound  bool
	selected        int
	commands        []commandSpec
	spinner         spinner.Model
	viewport        viewport.Model
	styles          styles
	status          string
	running         bool
	showHelp        bool
	lastCommand     []string
	lastCommandText string
	lastOutput      string

	inPrompt       bool
	promptIndex    int
	activeCommand  *commandSpec
	promptInputs   []textinput.Model
	promptValues   []string
	currentPrompt  string
	customCmdInput textinput.Model
}

type styles struct {
	header       lipgloss.Style
	subHeader    lipgloss.Style
	listSelected lipgloss.Style
	listItem     lipgloss.Style
	desc         lipgloss.Style
	statusOK     lipgloss.Style
	statusWarn   lipgloss.Style
	statusErr    lipgloss.Style
	box          lipgloss.Style
	help         lipgloss.Style
}

func defaultCommands() []commandSpec {
	return []commandSpec{
		{label: "user login", description: "Authenticate user credentials", baseArgs: []string{"user", "login"}},
		{label: "user logout", description: "Remove local credentials", baseArgs: []string{"user", "logout"}},
		{label: "list", description: "List tunnels", baseArgs: []string{"list"}, optionalArg: "optional args (example: --all)"},
		{label: "show", description: "Show tunnel details", baseArgs: []string{"show"}, requiredArgs: []string{"tunnel-id"}, optionalArg: "optional args"},
		{label: "create", description: "Create a tunnel", baseArgs: []string{"create"}, requiredArgs: []string{"tunnel-id"}, optionalArg: "optional args"},
		{label: "update", description: "Update tunnel properties", baseArgs: []string{"update"}, requiredArgs: []string{"tunnel-id"}, optionalArg: "optional args"},
		{label: "delete", description: "Delete one tunnel", baseArgs: []string{"delete"}, requiredArgs: []string{"tunnel-id"}, optionalArg: "optional args"},
		{label: "delete-all", description: "Delete all tunnels", baseArgs: []string{"delete-all"}, optionalArg: "optional args"},
		{label: "token", description: "Issue tunnel access token", baseArgs: []string{"token"}, requiredArgs: []string{"tunnel-id"}, optionalArg: "optional args"},
		{label: "set", description: "Set default tunnel", baseArgs: []string{"set"}, requiredArgs: []string{"tunnel-id"}, optionalArg: "optional args"},
		{label: "unset", description: "Clear default tunnel", baseArgs: []string{"unset"}, optionalArg: "optional args"},
		{label: "access", description: "Manage tunnel access control entries", baseArgs: []string{"access"}, optionalArg: "subcommand and args"},
		{label: "port", description: "Manage tunnel ports", baseArgs: []string{"port"}, optionalArg: "subcommand and args"},
		{label: "host", description: "Host a tunnel", baseArgs: []string{"host"}, optionalArg: "tunnel-id and args (blank = auto-create)"},
		{label: "connect", description: "Connect to tunnel", baseArgs: []string{"connect"}, requiredArgs: []string{"tunnel-id"}, optionalArg: "optional args"},
		{label: "limits", description: "List user limits", baseArgs: []string{"limits"}, optionalArg: "optional args"},
		{label: "clusters", description: "List service clusters", baseArgs: []string{"clusters"}, optionalArg: "optional args"},
		{label: "echo", description: "Run diagnostic echo server", baseArgs: []string{"echo"}, requiredArgs: []string{"protocol"}, optionalArg: "optional args"},
		{label: "ping", description: "Send messages to echo server", baseArgs: []string{"ping"}, requiredArgs: []string{"uri"}, optionalArg: "optional args"},
		{label: "custom", description: "Run any raw command after devtunnel", baseArgs: []string{}},
	}
}

func initialModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	customInput := textinput.New()
	customInput.Placeholder = "example: user show --verbose"
	customInput.Prompt = "> "
	customInput.CharLimit = 400
	customInput.Width = 60

	m := model{
		commands: defaultCommands(),
		spinner:  s,
		styles: styles{
			header:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("25")).Padding(0, 1),
			subHeader:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81")),
			listSelected: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("33")).Padding(0, 1),
			listItem:     lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Padding(0, 1),
			desc:         lipgloss.NewStyle().Foreground(lipgloss.Color("246")),
			statusOK:     lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true),
			statusWarn:   lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true),
			statusErr:    lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
			box:          lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1),
			help:         lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		},
		status:         "checking devtunnel binary",
		customCmdInput: customInput,
	}

	return m
}

func checkBinaryCmd() tea.Cmd {
	return func() tea.Msg {
		_, err := exec.LookPath("devtunnel")
		if err != nil {
			return runResultMsg{
				command: "which devtunnel",
				output:  "devtunnel was not found in PATH",
				err:     err,
			}
		}
		return runResultMsg{
			command: "which devtunnel",
			output:  "devtunnel found and ready",
			err:     nil,
		}
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(checkBinaryCmd(), m.spinner.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.viewport = viewport.New(msg.Width-4, max(8, msg.Height-14))
			m.viewport.SetContent("Waiting for command output...")
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - 4
			m.viewport.Height = max(8, msg.Height-14)
		}

	case tea.KeyMsg:
		if m.running {
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			}
			break
		}

		if m.inPrompt {
			return m.handlePromptInput(msg)
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "?":
			m.showHelp = !m.showHelp
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.commands)-1 {
				m.selected++
			}
		case "r":
			if len(m.lastCommand) > 0 {
				return m, runCommandCmd(m.lastCommand)
			}
		case "enter":
			selectedCommand := m.commands[m.selected]
			return m.startPrompt(selectedCommand)
		}

	case runStartedMsg:
		m.running = true
		m.status = "running: " + msg.command
		m.lastCommandText = msg.command
		m.viewport.SetContent("Running command...\n\n" + msg.command)
		return m, m.spinner.Tick

	case runResultMsg:
		if msg.command == "which devtunnel" {
			if msg.err != nil {
				m.devtunnelFound = false
				m.status = "devtunnel not found in PATH"
			} else {
				m.devtunnelFound = true
				m.status = "ready"
			}
		} else {
			m.running = false
			if msg.err != nil {
				m.status = "command failed"
			} else {
				m.status = "command completed"
			}
			m.lastOutput = msg.output
			m.viewport.SetContent("$ " + msg.command + "\n\n" + msg.output)
			m.viewport.GotoTop()
		}
		return m, nil

	case spinner.TickMsg:
		if m.running {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	return m, vpCmd
}

func (m model) startPrompt(cmd commandSpec) (model, tea.Cmd) {
	if !m.devtunnelFound {
		m.status = "install devtunnel CLI first"
		return m, nil
	}

	if cmd.label == "custom" {
		m.inPrompt = true
		m.activeCommand = &cmd
		m.currentPrompt = "Custom command (after `devtunnel`)"
		m.customCmdInput.SetValue("")
		m.customCmdInput.Focus()
		return m, textinput.Blink
	}

	fields := append([]string{}, cmd.requiredArgs...)
	if cmd.optionalArg != "" {
		fields = append(fields, cmd.optionalArg)
	}

	if len(fields) == 0 {
		parts := append([]string{"devtunnel"}, cmd.baseArgs...)
		m.lastCommand = parts
		return m, runCommandCmd(parts)
	}

	m.promptInputs = make([]textinput.Model, len(fields))
	m.promptValues = make([]string, len(fields))
	for i, label := range fields {
		ti := textinput.New()
		ti.Prompt = "> "
		ti.Placeholder = label
		ti.CharLimit = 300
		ti.Width = 60
		m.promptInputs[i] = ti
	}
	m.promptInputs[0].Focus()
	m.inPrompt = true
	m.promptIndex = 0
	m.activeCommand = &cmd
	m.currentPrompt = fields[0]

	return m, textinput.Blink
}

func (m model) handlePromptInput(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.activeCommand == nil {
		m.inPrompt = false
		return m, nil
	}

	if m.activeCommand.label == "custom" {
		switch k.String() {
		case "esc":
			m.inPrompt = false
			m.activeCommand = nil
			m.currentPrompt = ""
			return m, nil
		case "enter":
			raw := strings.TrimSpace(m.customCmdInput.Value())
			m.inPrompt = false
			m.activeCommand = nil
			m.currentPrompt = ""
			if raw == "" {
				return m, nil
			}
			parts := append([]string{"devtunnel"}, strings.Fields(raw)...)
			m.lastCommand = parts
			return m, runCommandCmd(parts)
		}
		var cmd tea.Cmd
		m.customCmdInput, cmd = m.customCmdInput.Update(k)
		return m, cmd
	}

	switch k.String() {
	case "esc":
		m.inPrompt = false
		m.activeCommand = nil
		m.currentPrompt = ""
		return m, nil
	case "enter":
		m.promptValues[m.promptIndex] = strings.TrimSpace(m.promptInputs[m.promptIndex].Value())
		if m.promptIndex < len(m.promptInputs)-1 {
			m.promptInputs[m.promptIndex].Blur()
			m.promptIndex++
			m.promptInputs[m.promptIndex].Focus()
			m.currentPrompt = m.promptInputs[m.promptIndex].Placeholder
			return m, nil
		}

		args := append([]string{}, m.activeCommand.baseArgs...)
		for i, label := range append([]string{}, m.activeCommand.requiredArgs...) {
			v := strings.TrimSpace(m.promptValues[i])
			if v == "" {
				m.status = "missing required argument: " + label
				m.inPrompt = false
				m.activeCommand = nil
				m.currentPrompt = ""
				return m, nil
			}
			args = append(args, v)
		}

		if m.activeCommand.optionalArg != "" {
			optIndex := len(m.promptInputs) - 1
			if len(m.activeCommand.requiredArgs) == len(m.promptInputs) {
				optIndex = -1
			}
			if optIndex >= 0 {
				if extra := strings.TrimSpace(m.promptValues[optIndex]); extra != "" {
					args = append(args, strings.Fields(extra)...)
				}
			}
		}

		parts := append([]string{"devtunnel"}, args...)
		m.lastCommand = parts
		m.inPrompt = false
		m.activeCommand = nil
		m.currentPrompt = ""
		return m, runCommandCmd(parts)
	}

	var cmd tea.Cmd
	m.promptInputs[m.promptIndex], cmd = m.promptInputs[m.promptIndex].Update(k)
	return m, cmd
}

func runCommandCmd(parts []string) tea.Cmd {
	if len(parts) == 0 {
		return nil
	}
	cmdText := strings.Join(parts, " ")
	return tea.Sequence(
		func() tea.Msg {
			return runStartedMsg{command: cmdText}
		},
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			command := exec.CommandContext(ctx, parts[0], parts[1:]...)
			var out bytes.Buffer
			command.Stdout = &out
			command.Stderr = &out
			err := command.Run()

			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return runResultMsg{
					command: cmdText,
					output:  out.String() + "\n\nTimed out after 5 minutes.",
					err:     ctx.Err(),
				}
			}

			return runResultMsg{
				command: cmdText,
				output:  out.String(),
				err:     err,
			}
		},
	)
}

func (m model) View() string {
	if !m.ready {
		return "Loading..."
	}

	var b strings.Builder
	title := " DevTunnels TUI | macOS + Linux "
	b.WriteString(m.styles.header.Render(title))
	b.WriteString("\n")

	statusStyle := m.styles.statusOK
	if !m.devtunnelFound {
		statusStyle = m.styles.statusErr
	} else if m.running {
		statusStyle = m.styles.statusWarn
	}

	if m.running {
		b.WriteString(statusStyle.Render(m.spinner.View() + " " + m.status))
	} else {
		b.WriteString(statusStyle.Render(m.status))
	}
	b.WriteString("\n\n")

	if m.inPrompt {
		b.WriteString(m.styles.subHeader.Render("Input"))
		b.WriteString("\n")
		b.WriteString(m.styles.desc.Render(m.currentPrompt))
		b.WriteString("\n")
		if m.activeCommand != nil && m.activeCommand.label == "custom" {
			b.WriteString(m.customCmdInput.View())
		} else if len(m.promptInputs) > 0 {
			b.WriteString(m.promptInputs[m.promptIndex].View())
		}
		b.WriteString("\n")
		b.WriteString(m.styles.help.Render("Enter: next/run  Esc: cancel"))
		b.WriteString("\n\n")
	} else {
		b.WriteString(m.styles.subHeader.Render("Commands"))
		b.WriteString("\n")
		for i, cmd := range m.commands {
			line := fmt.Sprintf("%-12s  %s", cmd.label, cmd.description)
			if i == m.selected {
				b.WriteString(m.styles.listSelected.Render(line))
			} else {
				b.WriteString(m.styles.listItem.Render(line))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(m.styles.subHeader.Render("Output"))
	b.WriteString("\n")
	b.WriteString(m.styles.box.Width(m.width - 2).Render(m.viewport.View()))
	b.WriteString("\n")

	if m.showHelp {
		b.WriteString(m.styles.help.Render("j/k or up/down: navigate | Enter: execute | r: rerun last | ?: toggle help | q: quit"))
	} else {
		b.WriteString(m.styles.help.Render("Press ? for key help"))
	}

	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
