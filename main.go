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

type commandItem struct {
	name        string
	description string
	baseArgs    []string
	required    []string
	optional    string
	example     string
}

type commandCategory struct {
	name     string
	commands []commandItem
}

type runStartedMsg struct {
	cmdText string
}

type runFinishedMsg struct {
	cmdText string
	output  string
	err     error
}

type styles struct {
	header      lipgloss.Style
	headerInfo  lipgloss.Style
	pane        lipgloss.Style
	paneTitle   lipgloss.Style
	selected    lipgloss.Style
	normal      lipgloss.Style
	dim         lipgloss.Style
	ok          lipgloss.Style
	warn        lipgloss.Style
	err         lipgloss.Style
	hotkey      lipgloss.Style
	cmdline     lipgloss.Style
	statusBar   lipgloss.Style
	focusBorder lipgloss.Style
}

type model struct {
	width  int
	height int
	ready  bool

	styles  styles
	spinner spinner.Model

	devtunnelFound bool
	running        bool
	statusText     string
	statusErr      bool

	categories []commandCategory
	catIdx     int
	cmdIdx     int
	focusPane  int // visual hint only

	viewport viewport.Model

	filterMode  bool
	filterInput textinput.Model

	cmdMode  bool
	cmdInput textinput.Model

	formMode   bool
	formTitle  string
	formCmd    *commandItem
	formLabels []string
	formInputs []textinput.Model
	formIndex  int

	lastCmd    []string
	lastOutput string
}

func newStyles() styles {
	return styles{
		header:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24")).Padding(0, 1),
		headerInfo:  lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Background(lipgloss.Color("24")).Padding(0, 1),
		pane:        lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1),
		paneTitle:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81")),
		selected:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("31")).Padding(0, 1),
		normal:      lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Padding(0, 1),
		dim:         lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		ok:          lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true),
		warn:        lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true),
		err:         lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		hotkey:      lipgloss.NewStyle().Foreground(lipgloss.Color("121")).Bold(true),
		cmdline:     lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("236")).Padding(0, 1),
		statusBar:   lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("236")).Padding(0, 1),
		focusBorder: lipgloss.NewStyle().BorderForeground(lipgloss.Color("39")),
	}
}

func catalog() []commandCategory {
	return []commandCategory{
		{
			name: "Tunnels",
			commands: []commandItem{
				{name: "list", description: "List tunnels", baseArgs: []string{"list"}, optional: "flags", example: "list --all"},
				{name: "show", description: "Show tunnel details", baseArgs: []string{"show"}, required: []string{"tunnel-id"}},
				{name: "create", description: "Create a tunnel", baseArgs: []string{"create"}, required: []string{"tunnel-id"}, optional: "flags"},
				{name: "update", description: "Update tunnel properties", baseArgs: []string{"update"}, required: []string{"tunnel-id"}, optional: "flags"},
				{name: "delete", description: "Delete a tunnel", baseArgs: []string{"delete"}, required: []string{"tunnel-id"}},
				{name: "delete-all", description: "Delete all tunnels", baseArgs: []string{"delete-all"}},
				{name: "set", description: "Set default tunnel", baseArgs: []string{"set"}, required: []string{"tunnel-id"}},
				{name: "unset", description: "Clear default tunnel", baseArgs: []string{"unset"}},
				{name: "token", description: "Issue tunnel access token", baseArgs: []string{"token"}, required: []string{"tunnel-id"}, optional: "flags"},
			},
		},
		{
			name: "Ports & Access",
			commands: []commandItem{
				{name: "port", description: "Manage tunnel ports", baseArgs: []string{"port"}, optional: "subcommand and args", example: "port list <tunnel-id>"},
				{name: "access", description: "Manage access control", baseArgs: []string{"access"}, optional: "subcommand and args", example: "access list <tunnel-id>"},
			},
		},
		{
			name: "Connections",
			commands: []commandItem{
				{name: "host", description: "Host a tunnel", baseArgs: []string{"host"}, optional: "tunnel-id and flags"},
				{name: "connect", description: "Connect to tunnel", baseArgs: []string{"connect"}, required: []string{"tunnel-id"}, optional: "flags"},
			},
		},
		{
			name: "User",
			commands: []commandItem{
				{name: "user login", description: "Authenticate user credentials", baseArgs: []string{"user", "login"}},
				{name: "user logout", description: "Remove local credentials", baseArgs: []string{"user", "logout"}},
				{name: "user", description: "Run user subcommand", baseArgs: []string{"user"}, optional: "subcommand and args", example: "user show"},
			},
		},
		{
			name: "Diagnostics",
			commands: []commandItem{
				{name: "limits", description: "List user limits", baseArgs: []string{"limits"}},
				{name: "clusters", description: "List clusters", baseArgs: []string{"clusters"}},
				{name: "echo", description: "Run echo server", baseArgs: []string{"echo"}, required: []string{"protocol"}},
				{name: "ping", description: "Ping remote echo server", baseArgs: []string{"ping"}, required: []string{"uri"}},
			},
		},
		{
			name: "Custom",
			commands: []commandItem{
				{name: ": command mode", description: "Type any raw devtunnel command", baseArgs: []string{}},
			},
		},
	}
}

func initialModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	filter := textinput.New()
	filter.Placeholder = "filter commands"
	filter.CharLimit = 120
	filter.Prompt = "/ "
	filter.Width = 40

	cmd := textinput.New()
	cmd.Placeholder = "type command after 'devtunnel'"
	cmd.CharLimit = 500
	cmd.Prompt = ": "
	cmd.Width = 70

	return model{
		styles:      newStyles(),
		spinner:     s,
		categories:  catalog(),
		statusText:  "checking devtunnel binary",
		filterInput: filter,
		cmdInput:    cmd,
		focusPane:   1,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(checkBinaryCmd(), m.spinner.Tick)
}

func checkBinaryCmd() tea.Cmd {
	return func() tea.Msg {
		_, err := exec.LookPath("devtunnel")
		if err != nil {
			return runFinishedMsg{cmdText: "which devtunnel", output: "devtunnel not found in PATH", err: err}
		}
		return runFinishedMsg{cmdText: "which devtunnel", output: "devtunnel detected", err: nil}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.viewport = viewport.New(max(20, msg.Width-64), max(8, msg.Height-10))
			m.viewport.SetContent("Output will appear here")
			m.ready = true
		} else {
			m.viewport.Width = max(20, msg.Width-64)
			m.viewport.Height = max(8, msg.Height-10)
		}

	case spinner.TickMsg:
		if m.running {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case runStartedMsg:
		m.running = true
		m.statusErr = false
		m.statusText = "running " + msg.cmdText
		m.viewport.SetContent("$ " + msg.cmdText + "\n\nRunning...")
		m.viewport.GotoTop()
		return m, m.spinner.Tick

	case runFinishedMsg:
		if msg.cmdText == "which devtunnel" {
			if msg.err != nil {
				m.devtunnelFound = false
				m.statusErr = true
				m.statusText = "devtunnel not found in PATH"
			} else {
				m.devtunnelFound = true
				m.statusErr = false
				m.statusText = "ready"
			}
			return m, nil
		}

		m.running = false
		m.lastOutput = msg.output
		if msg.err != nil {
			m.statusErr = true
			m.statusText = "command failed"
		} else {
			m.statusErr = false
			m.statusText = "command completed"
		}
		m.viewport.SetContent("$ " + msg.cmdText + "\n\n" + msg.output)
		m.viewport.GotoTop()
		return m, nil

	case tea.KeyMsg:
		if m.formMode {
			return m.updateForm(msg)
		}
		if m.cmdMode {
			return m.updateCmdMode(msg)
		}
		if m.filterMode {
			return m.updateFilterMode(msg)
		}

		switch {
		case msg.Type == tea.KeyCtrlC || msg.String() == "q":
			return m, tea.Quit
		case msg.Type == tea.KeyLeft || msg.String() == "h":
			if m.catIdx > 0 {
				m.catIdx--
				m.cmdIdx = 0
			}
		case msg.Type == tea.KeyRight || msg.String() == "l":
			if m.catIdx < len(m.categories)-1 {
				m.catIdx++
				m.cmdIdx = 0
			}
		case msg.String() == ":":
			m.cmdMode = true
			m.cmdInput.SetValue("")
			m.cmdInput.Focus()
			return m, textinput.Blink
		case msg.String() == "/":
			m.filterMode = true
			m.focusPane = 1
			m.filterInput.Focus()
			return m, textinput.Blink
		case msg.String() == "r":
			if len(m.lastCmd) > 0 {
				return m, runCommandCmd(m.lastCmd)
			}
		case msg.String() == "g":
			m.cmdIdx = 0
		case msg.String() == "G":
			cmds := m.visibleCommands()
			if len(cmds) > 0 {
				m.cmdIdx = len(cmds) - 1
			}
		case msg.String() == "1", msg.String() == "2", msg.String() == "3", msg.String() == "4", msg.String() == "5", msg.String() == "6":
			i := int(msg.String()[0] - '1')
			if i >= 0 && i < len(m.categories) {
				m.catIdx = i
				m.cmdIdx = 0
				m.focusPane = 1
			}
		case msg.Type == tea.KeyUp || msg.String() == "k":
			m.moveUp()
		case msg.Type == tea.KeyDown || msg.String() == "j":
			m.moveDown()
		case msg.Type == tea.KeyPgUp:
			m.viewport.HalfViewUp()
		case msg.Type == tea.KeyPgDown:
			m.viewport.HalfViewDown()
		case msg.String() == "u":
			m.viewport.HalfViewUp()
		case msg.String() == "d":
			m.viewport.HalfViewDown()
		case msg.Type == tea.KeyEnter:
			return m.runSelected()
		}
	}

	return m, nil
}

func (m model) moveUp() {
	if m.cmdIdx > 0 {
		m.cmdIdx--
	}
}

func (m model) moveDown() {
	cmds := m.visibleCommands()
	if m.cmdIdx < len(cmds)-1 {
		m.cmdIdx++
	}
}

func (m model) visibleCommands() []commandItem {
	if m.catIdx < 0 || m.catIdx >= len(m.categories) {
		return nil
	}
	items := m.categories[m.catIdx].commands
	flt := strings.TrimSpace(strings.ToLower(m.filterInput.Value()))
	if flt == "" {
		return items
	}
	out := make([]commandItem, 0, len(items))
	for _, item := range items {
		hay := strings.ToLower(item.name + " " + item.description + " " + strings.Join(item.baseArgs, " "))
		if strings.Contains(hay, flt) {
			out = append(out, item)
		}
	}
	return out
}

func (m model) runSelected() (tea.Model, tea.Cmd) {
	if !m.devtunnelFound {
		m.statusErr = true
		m.statusText = "install devtunnel CLI first"
		return m, nil
	}
	cmds := m.visibleCommands()
	if len(cmds) == 0 {
		return m, nil
	}
	if m.cmdIdx >= len(cmds) {
		m.cmdIdx = len(cmds) - 1
	}
	cmd := cmds[m.cmdIdx]

	if cmd.name == ": command mode" {
		m.cmdMode = true
		m.cmdInput.SetValue("")
		m.cmdInput.Focus()
		return m, textinput.Blink
	}

	labels := append([]string{}, cmd.required...)
	if cmd.optional != "" {
		labels = append(labels, cmd.optional)
	}

	if len(labels) == 0 {
		parts := append([]string{"devtunnel"}, cmd.baseArgs...)
		m.lastCmd = parts
		return m, runCommandCmd(parts)
	}

	m.formMode = true
	m.formCmd = &cmd
	m.formTitle = cmd.name
	m.formLabels = labels
	m.formInputs = make([]textinput.Model, len(labels))
	m.formIndex = 0

	for i, label := range labels {
		ti := textinput.New()
		ti.Prompt = "> "
		ti.Placeholder = label
		ti.Width = 60
		ti.CharLimit = 300
		m.formInputs[i] = ti
	}
	m.formInputs[0].Focus()

	return m, textinput.Blink
}

func (m model) updateForm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.formMode = false
		m.formCmd = nil
		m.formInputs = nil
		m.formLabels = nil
		m.formTitle = ""
		m.formIndex = 0
		return m, nil
	case "enter":
		if m.formIndex < len(m.formInputs)-1 {
			m.formInputs[m.formIndex].Blur()
			m.formIndex++
			m.formInputs[m.formIndex].Focus()
			return m, nil
		}

		if m.formCmd == nil {
			m.formMode = false
			return m, nil
		}

		parts := append([]string{"devtunnel"}, m.formCmd.baseArgs...)
		reqCount := len(m.formCmd.required)
		for i := 0; i < reqCount; i++ {
			v := strings.TrimSpace(m.formInputs[i].Value())
			if v == "" {
				m.statusErr = true
				m.statusText = "missing required: " + m.formCmd.required[i]
				m.formMode = false
				m.formCmd = nil
				return m, nil
			}
			parts = append(parts, v)
		}

		if m.formCmd.optional != "" {
			i := len(m.formInputs) - 1
			if i >= 0 {
				extra := strings.TrimSpace(m.formInputs[i].Value())
				if extra != "" {
					parts = append(parts, strings.Fields(extra)...)
				}
			}
		}

		m.formMode = false
		m.formCmd = nil
		m.formInputs = nil
		m.formLabels = nil
		m.formTitle = ""
		m.formIndex = 0
		m.lastCmd = parts
		return m, runCommandCmd(parts)
	}

	var cmd tea.Cmd
	m.formInputs[m.formIndex], cmd = m.formInputs[m.formIndex].Update(k)
	return m, cmd
}

func (m model) updateCmdMode(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.cmdMode = false
		m.cmdInput.Blur()
		return m, nil
	case "enter":
		raw := strings.TrimSpace(m.cmdInput.Value())
		m.cmdMode = false
		m.cmdInput.Blur()
		if raw == "" {
			return m, nil
		}
		parts := append([]string{"devtunnel"}, strings.Fields(raw)...)
		m.lastCmd = parts
		return m, runCommandCmd(parts)
	}
	var cmd tea.Cmd
	m.cmdInput, cmd = m.cmdInput.Update(k)
	return m, cmd
}

func (m model) updateFilterMode(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.filterMode = false
		m.filterInput.Blur()
		return m, nil
	case "enter":
		m.filterMode = false
		m.filterInput.Blur()
		m.cmdIdx = 0
		return m, nil
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(k)
	m.cmdIdx = 0
	return m, cmd
}

func runCommandCmd(parts []string) tea.Cmd {
	if len(parts) == 0 {
		return nil
	}
	cmdText := strings.Join(parts, " ")
	return tea.Sequence(
		func() tea.Msg { return runStartedMsg{cmdText: cmdText} },
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			err := cmd.Run()

			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return runFinishedMsg{cmdText: cmdText, output: out.String() + "\n\nTimed out after 10 minutes.", err: ctx.Err()}
			}
			return runFinishedMsg{cmdText: cmdText, output: out.String(), err: err}
		},
	)
}

func (m model) View() string {
	if !m.ready {
		return "Loading..."
	}

	header := m.renderHeader()
	main := m.renderMain()
	bar := m.renderBottomBar()

	return header + "\n" + main + "\n" + bar
}

func (m model) renderHeader() string {
	left := m.styles.header.Render(" DevTunnels TUI ")
	mode := "NORMAL"
	if m.filterMode {
		mode = "FILTER"
	} else if m.cmdMode {
		mode = "COMMAND"
	} else if m.formMode {
		mode = "FORM"
	} else if m.running {
		mode = "RUNNING"
	}

	statusStyle := m.styles.ok
	if m.running {
		statusStyle = m.styles.warn
	}
	if m.statusErr {
		statusStyle = m.styles.err
	}

	statusText := m.statusText
	if m.running {
		statusText = m.spinner.View() + " " + statusText
	}
	right := m.styles.headerInfo.Render(fmt.Sprintf("mode:%s  %s", mode, statusStyle.Render(statusText)))

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m model) renderMain() string {
	leftW := max(20, m.width/5)
	midW := max(36, m.width/3)
	rightW := max(30, m.width-leftW-midW-4)
	height := max(8, m.height-6)

	catPane := m.renderCategories(leftW, height)
	cmdPane := m.renderCommands(midW, height)
	outPane := m.renderOutput(rightW, height)

	return lipgloss.JoinHorizontal(lipgloss.Top, catPane, cmdPane, outPane)
}

func (m model) paneStyleForFocus(pane int, width, height int) lipgloss.Style {
	s := m.styles.pane.Width(width).Height(height)
	if m.focusPane == pane && !m.formMode && !m.cmdMode && !m.filterMode {
		s = s.BorderForeground(lipgloss.Color("39"))
	}
	return s
}

func (m model) renderCategories(width, height int) string {
	var b strings.Builder
	b.WriteString(m.styles.paneTitle.Render("Resources"))
	b.WriteString("\n")
	for i, c := range m.categories {
		line := fmt.Sprintf("%d %s", i+1, c.name)
		if i == m.catIdx {
			b.WriteString(m.styles.selected.Render(line))
		} else {
			b.WriteString(m.styles.normal.Render(line))
		}
		b.WriteString("\n")
	}
	return m.paneStyleForFocus(0, width, height).Render(b.String())
}

func (m model) renderCommands(width, height int) string {
	cmds := m.visibleCommands()
	if m.cmdIdx >= len(cmds) {
		m.cmdIdx = max(0, len(cmds)-1)
	}

	var b strings.Builder
	b.WriteString(m.styles.paneTitle.Render("Commands"))
	b.WriteString("\n")
	if strings.TrimSpace(m.filterInput.Value()) != "" {
		b.WriteString(m.styles.dim.Render("filter: " + m.filterInput.Value()))
		b.WriteString("\n")
	}

	if len(cmds) == 0 {
		b.WriteString(m.styles.dim.Render("No commands match filter"))
		b.WriteString("\n")
	} else {
		for i, c := range cmds {
			line := fmt.Sprintf("%-14s %s", c.name, c.description)
			if i == m.cmdIdx {
				b.WriteString(m.styles.selected.Render(line))
			} else {
				b.WriteString(m.styles.normal.Render(line))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		selected := cmds[m.cmdIdx]
		b.WriteString(m.styles.dim.Render("selected: " + strings.Join(append([]string{"devtunnel"}, selected.baseArgs...), " ")))
		b.WriteString("\n")
		if selected.example != "" {
			b.WriteString(m.styles.dim.Render("example: devtunnel " + selected.example))
			b.WriteString("\n")
		}
	}

	return m.paneStyleForFocus(1, width, height).Render(b.String())
}

func (m model) renderOutput(width, height int) string {
	var b strings.Builder
	b.WriteString(m.styles.paneTitle.Render("Output"))
	b.WriteString("\n")
	b.WriteString(m.viewport.View())
	return m.paneStyleForFocus(2, width, height).Render(b.String())
}

func (m model) renderBottomBar() string {
	if m.formMode {
		return m.renderFormOverlay()
	}
	if m.cmdMode {
		return m.styles.cmdline.Render(m.cmdInput.View() + "  (Enter run, Esc cancel)")
	}
	if m.filterMode {
		return m.styles.cmdline.Render(m.filterInput.View() + "  (Enter apply, Esc cancel)")
	}

	help := []string{
		m.styles.hotkey.Render("←/→") + " category",
		m.styles.hotkey.Render("↑/↓") + " command",
		m.styles.hotkey.Render("enter") + " run",
		m.styles.hotkey.Render(":") + " raw cmd",
		m.styles.hotkey.Render("/") + " filter",
		m.styles.hotkey.Render("u/d") + " output scroll",
		m.styles.hotkey.Render("r") + " rerun",
		m.styles.hotkey.Render("q") + " quit",
	}
	return m.styles.statusBar.Render(strings.Join(help, "  "))
}

func (m model) renderFormOverlay() string {
	if m.formCmd == nil {
		return m.styles.cmdline.Render("form unavailable")
	}

	var b strings.Builder
	b.WriteString("Run: devtunnel " + strings.Join(m.formCmd.baseArgs, " "))
	b.WriteString("\n")
	b.WriteString("Field " + fmt.Sprintf("%d/%d", m.formIndex+1, len(m.formInputs)) + " - " + m.formLabels[m.formIndex])
	b.WriteString("\n")
	b.WriteString(m.formInputs[m.formIndex].View())
	b.WriteString("\n")
	b.WriteString("Enter next/run, Esc cancel")

	return m.styles.cmdline.Render(b.String())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
