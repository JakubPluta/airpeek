// Package tui is a read-only terminal browser for Airflow:
// environments → DAGs → runs → task instances → logs.
package tui

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JakubPluta/airpeek/internal/airflow"
)

// Config is everything the UI needs from outside. Dependencies are plain
// functions, so tests run the whole UI against fakes.
type Config struct {
	// Env is the environment to open. When empty the user picks one from
	// ListEnvironments.
	Env string

	ListEnvironments func(ctx context.Context) ([]string, error)
	// Connect opens the environment named env. It fails when the
	// environment cannot be reached at all, such as with a missing secret.
	Connect func(env string) (Environment, error)
}

// Environment is an Airflow opened by Config.Connect.
type Environment struct {
	Getter airflow.Getter
	// Highlight paints the header red, to make production unmistakable.
	Highlight bool
}

// App is the Bubble Tea model: a stack of screens with a shared header,
// status bar and help line.
type App struct {
	cfg       Config
	ctx       context.Context
	env       string
	highlight bool
	stack     []screen
	err       error
	width     int
	height    int
	// showHelp replaces the current screen with the help page; the screen
	// keeps its state underneath.
	showHelp bool
	// spinner turns in the header while any screen waits for Airflow; it
	// only ticks then, so an idle UI does not redraw.
	spinner  spinner.Model
	spinning bool
}

// New starts at the DAG list of cfg.Env, or at the environment picker when
// cfg.Env is empty or cannot be opened.
func New(cfg Config) *App {
	a := &App{
		cfg:     cfg,
		ctx:     context.Background(),
		spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(busyStyle)),
	}
	if cfg.Env != "" {
		if a.err = a.openEnv(cfg.Env); a.err == nil {
			return a
		}
	}
	a.stack = []screen{newEnvsScreen(a.ctx, cfg.ListEnvironments)}
	return a
}

// Init loads the first screen and starts the clock tick.
func (a *App) Init() tea.Cmd { return tea.Batch(a.top().init(), tick()) }

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := a.update(msg)
	return a, tea.Batch(cmd, a.startSpinner())
}

func (a *App) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		return a.broadcast(a.resize())
	case tea.KeyMsg:
		return a.handleKey(msg)
	case tea.MouseMsg:
		// Screens get coordinates relative to their own area, below the header.
		msg.Y--
		if a.showHelp || msg.Y < 0 {
			return nil
		}
		return a.top().update(msg)
	case pushMsg:
		a.stack = append(a.stack, msg.s)
		return tea.Batch(msg.s.update(a.resize()), msg.s.init())
	case envChosenMsg:
		if a.err = a.openEnv(msg.name); a.err != nil {
			return nil
		}
		return tea.Batch(a.top().update(a.resize()), a.top().init())
	case errMsg:
		a.err = msg.err
		return nil
	case tickMsg:
		return tea.Batch(a.broadcast(msg), tick())
	case spinner.TickMsg:
		if !a.busy() {
			a.spinning = false
			return nil
		}
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		return cmd
	case addressed:
		// A screen that was closed meanwhile no longer wants its answer.
		if slices.Contains(a.stack, msg.target()) {
			return msg.target().update(msg)
		}
		return nil
	}
	return a.broadcast(msg)
}

// startSpinner starts the spinner when a screen became busy; once started
// it keeps itself ticking until nothing is busy.
func (a *App) startSpinner() tea.Cmd {
	if a.spinning || !a.busy() {
		return nil
	}
	a.spinning = true
	return a.spinner.Tick
}

func (a *App) busy() bool {
	return slices.ContainsFunc(a.stack, func(s screen) bool { return s.busy() })
}

func (a *App) handleKey(msg tea.KeyMsg) tea.Cmd {
	if msg.String() == "ctrl+c" {
		return tea.Quit
	}
	a.err = nil
	if a.showHelp {
		switch msg.String() {
		case "q":
			return tea.Quit
		case "?", "esc":
			a.showHelp = false
		}
		return nil
	}
	if a.top().capturing() {
		return a.top().update(msg)
	}
	switch msg.String() {
	case "q":
		return tea.Quit
	case "?":
		a.showHelp = true
		return nil
	case "esc", "backspace", "h", "left":
		a.pop()
		return nil
	}
	return a.top().update(msg)
}

// View implements tea.Model.
func (a *App) View() string {
	if a.width == 0 {
		return ""
	}
	label := cmp.Or(a.env, "airpeek")
	style := envStyle
	if a.highlight {
		style = prodEnvStyle
	}
	crumbs := make([]string, len(a.stack))
	for i, s := range a.stack {
		crumbs[i] = s.title()
	}
	header := style.Render(label) + " " + titleStyle.Render(strings.Join(crumbs, " › ")) + dimStyle.Render("  read-only")
	if a.busy() {
		header += "  " + a.spinner.View()
	}

	status := ""
	if a.err != nil {
		status = errorStyle.Render("error: " + a.err.Error())
	}
	helpLine := a.top().help()
	if len(a.stack) > 1 {
		helpLine += " · esc back"
	}
	helpLine += " · ? help · q quit"

	content := a.top().view()
	if a.showHelp {
		content = helpView(a.width)
		helpLine = "? / esc close help · q quit"
	}
	bodyHeight := a.resize().height
	body := lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight).Render(content)
	return header + "\n" + body + "\n" + fit(status, a.width) + "\n" + dimStyle.Render(fit(helpLine, a.width))
}

func (a *App) top() screen { return a.stack[len(a.stack)-1] }

// openEnv replaces the whole stack with the DAG list of env. When env cannot
// be opened the stack stays as it is.
func (a *App) openEnv(env string) error {
	e, err := a.cfg.Connect(env)
	if err != nil {
		return fmt.Errorf("open %s: %w", env, err)
	}
	for _, s := range a.stack {
		s.close()
	}
	a.env, a.highlight = env, e.Highlight
	a.stack = []screen{newDAGsScreen(a.ctx, e.Getter)}
	return nil
}

func (a *App) pop() {
	if len(a.stack) > 1 {
		a.top().close()
		a.stack = a.stack[:len(a.stack)-1]
	}
}

func (a *App) broadcast(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(a.stack))
	for i, s := range a.stack {
		cmds[i] = s.update(msg)
	}
	return tea.Batch(cmds...)
}

// resize leaves one line for the header, one for status and one for help.
func (a *App) resize() resizeMsg {
	return resizeMsg{width: a.width, height: max(a.height-3, 3)}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}
