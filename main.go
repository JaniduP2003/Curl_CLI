package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ========== ADDED: APP_ART VARIABLE ==========
var APP_ART = `
                                      ██████╗██╗   ██╗██████╗ ██╗          ██████╗██╗     ██╗
                                     ██╔════╝██║   ██║██╔══██╗██║         ██╔════╝██║     ██║
                                     ██║     ██║   ██║██████╔╝██║         ██║     ██║     ██║
                                     ██║     ██║   ██║██╔══██╗██║         ██║     ██║     ██║
                                     ╚██████╗╚██████╔╝██║  ██║███████╗    ╚██████╗███████╗██║
                                      ╚═════╝ ╚═════╝ ╚═╝  ╚═╝╚══════╝     ╚═════╝╚══════╝╚═╝                                    
         
                                                   Interactive cURL Terminal UI
`

// ========== END OF ADDITION ==========

// Screen states for navigation
type screen int

const (
	introScreen screen = iota
	urlScreen screen = iota
	methodScreen
	headersScreen
	bodyScreen
	confirmScreen
	loadingScreen
	responseScreen
	resettingScreen
)

// HTTP methods available
var methods = []list.Item{
	item{title: "GET", desc: "Retrieve data from server"},
	item{title: "POST", desc: "Send data to server"},
	item{title: "PUT", desc: "Update data on server"},
	item{title: "DELETE", desc: "Delete data from server"},
}

// Item for the list component
type item struct {
	title, desc string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

// Message types for async operations
type responseMsg struct {
	output string
	err    error
}

// Main model holding all state
type model struct {
	currentScreen screen

	// Input components
	urlInput     textinput.Model
	methodList   list.Model
	headersInput textarea.Model
	bodyInput    textarea.Model

	// Display components
	spinner  spinner.Model
	viewport viewport.Model
	progress progress.Model

	// Data storage
	url      string
	method   string
	headers  string
	body     string
	response string

	// UI state
	ready bool
	err   error

	// Responsive sizing
	width  int
	height int

	// Request cancellation
	cancel   context.CancelFunc
	canceled bool

	// Intro border animation
	introDone        bool
	boxActive        bool
	animOffset       int
	animTopProgress  int // columns drawn on top/bottom
	animSideProgress int // rows drawn on sides

	// Reset progress state
	resetPercent float64
}

// Initialize the application
func initialModel() model {
	// URL input field
	ti := textinput.New()
	ti.Placeholder = "https://api.example.com/endpoint"
	ti.Focus()
	ti.CharLimit = 500
	ti.Width = 60

	// Method selection list
	delegate := list.NewDefaultDelegate()
	methodList := list.New(methods, delegate, 0, 0)
	methodList.Title = "Select HTTP Method"
	methodList.SetShowStatusBar(false)
	methodList.SetFilteringEnabled(false)
	methodList.Styles.Title = lipgloss.NewStyle().
		MarginLeft(2).
		Bold(true).
		Foreground(lipgloss.Color("62"))

	// Headers input (textarea for multiple lines)
	headersInput := textarea.New()
	headersInput.Placeholder = "Content-Type: application/json\nAuthorization: Bearer token"
	headersInput.SetWidth(60)
	headersInput.SetHeight(5)

	// Body input (textarea for JSON)
	bodyInput := textarea.New()
	bodyInput.Placeholder = `{"key": "value"}`
	bodyInput.SetWidth(60)
	bodyInput.SetHeight(10)

	// Loading spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	// Progress bar for reset transition
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
	)

	return model{
		currentScreen: introScreen,
		urlInput:      ti,
		methodList:    methodList,
		headersInput:  headersInput,
		bodyInput:     bodyInput,
		spinner:       s,
		progress:      p,
		ready:         false,
	// border intro defaults
	animOffset: 0,
	}
}

// Initialize command - starts the spinner
func (m model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
	m.spinner.Tick,
	tickAnim(),
	)
}

// Update handles all user input and state changes
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			// Quit on any screen except loading
			if m.currentScreen != loadingScreen {
				return m, tea.Quit
			} else {
				// allow quit during loading by canceling first
				if m.cancel != nil {
					m.canceled = true
					m.cancel()
				}
				return m, tea.Quit
			}

		case "esc":
			// Go back to previous screen
			if m.currentScreen == loadingScreen {
				// cancel in-flight request
				if m.cancel != nil {
					m.canceled = true
					m.cancel()
				}
				return m, nil
			}
			if m.currentScreen == resettingScreen {
				// Skip resetting and jump to URL immediately
				m.resetInputs()
				m.currentScreen = urlScreen
				m.focusCurrentScreen()
				return m, nil
			}
			if m.currentScreen > urlScreen && m.currentScreen != loadingScreen {
				m.currentScreen--
				m.focusCurrentScreen()
				return m, nil
			}

		case "enter":
			if m.currentScreen == introScreen {
				// Skip animation on Enter
				m.introDone = true
				m.boxActive = false // no border after animation
				m.currentScreen = urlScreen
				m.focusCurrentScreen()
				return m, nil
			}
			return m.handleEnter()
		}

	case tea.WindowSizeMsg:
		// Handle window resize and initialize viewport
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.viewport = viewport.New(maxInt(20, msg.Width-4), maxInt(5, msg.Height-10))
			m.ready = true
		} else {
			m.viewport.Width = maxInt(20, msg.Width-4)
			m.viewport.Height = maxInt(5, msg.Height-10)
		}
		// Apply responsive sizing to inputs and lists
		m.applyLayoutSizes()
		m.methodList.SetSize(maxInt(20, msg.Width-4), maxInt(6, msg.Height-8))
		// Reset animation progress to fit new size if intro not done
		if m.currentScreen == introScreen && !m.introDone {
			m.animTopProgress = minInt(m.animTopProgress, maxInt(0, m.width-(m.animOffset*2)))
			m.animSideProgress = minInt(m.animSideProgress, maxInt(0, m.height-(m.animOffset*2)-2))
		}

	case responseMsg:
		// Received response from bash script
		if msg.err != nil {
			m.err = msg.err
			if m.canceled {
				m.response = "Request canceled by user."
			} else {
				m.response = fmt.Sprintf("Error: %v", msg.err)
			}
		} else {
			m.response = msg.output
		}
		m.currentScreen = responseScreen
		m.cancel = nil
		m.canceled = false

		// Initialize viewport with response content immediately
		if m.ready {
			m.viewport.SetContent(m.response)
			m.viewport.GotoTop()
		}
		return m, nil

	case spinner.TickMsg:
		// Update spinner animation
		if m.currentScreen == loadingScreen {
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case progress.FrameMsg:
		// Allow progress bar to animate smoothly
		if m.currentScreen == resettingScreen {
			var pModel tea.Model
			pModel, cmd = m.progress.Update(msg)
			if pm, ok := pModel.(progress.Model); ok {
				m.progress = pm
			}
			return m, cmd
		}

	case animTickMsg:
		// Advance intro border animation
		if m.currentScreen == introScreen && !m.introDone {
			// target lengths
			innerWidth := maxInt(0, m.width-(m.animOffset*2))
			innerHeight := maxInt(0, m.height-(m.animOffset*2))
			// Draw top/bottom first
			if m.animTopProgress < maxInt(0, innerWidth-2) {
				m.animTopProgress += maxInt(1, innerWidth/20)
			} else if m.animSideProgress < maxInt(0, innerHeight-2) {
				m.animSideProgress += maxInt(1, innerHeight/20)
			} else {
				m.introDone = true
				m.boxActive = false // remove border after animation completes
				// Transition to URL screen automatically after a short pause
			}
			if m.introDone {
				// Move on to URL screen
				m.currentScreen = urlScreen
				m.focusCurrentScreen()
				return m, nil
			}
			return m, tickAnim()
		}

	case resetTickMsg:
		// Drive resetting progress
		if m.currentScreen == resettingScreen {
			// increment with small steps; cap at 1.0
			step := float64(resetTickInterval) / float64(resetTotalDuration)
			if m.resetPercent+step > 1.0 {
				step = 1.0 - m.resetPercent
			}
			if step > 0 {
				cmds = append(cmds, m.progress.IncrPercent(step))
				m.resetPercent += step
			}
			if m.resetPercent >= 1.0 {
				// finished, go to URL screen and clear inputs
				m.resetInputs()
				m.currentScreen = urlScreen
				m.focusCurrentScreen()
				return m, nil
			}
			return m, tea.Batch(append(cmds, tickReset())...)
		}
	}

	// Update the active component based on current screen
	switch m.currentScreen {
	case urlScreen:
		m.urlInput, cmd = m.urlInput.Update(msg)
		cmds = append(cmds, cmd)
	case methodScreen:
		m.methodList, cmd = m.methodList.Update(msg)
		cmds = append(cmds, cmd)
	case headersScreen:
		m.headersInput, cmd = m.headersInput.Update(msg)
		cmds = append(cmds, cmd)
	case bodyScreen:
		m.bodyInput, cmd = m.bodyInput.Update(msg)
		cmds = append(cmds, cmd)
	case responseScreen:
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	case loadingScreen:
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	case resettingScreen:
		// Progress animation updates via progress.FrameMsg
		var pModel tea.Model
		pModel, cmd = m.progress.Update(msg)
		if pm, ok := pModel.(progress.Model); ok {
			m.progress = pm
		}
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// Handle Enter key press on each screen
func (m *model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.currentScreen {
	case urlScreen:
		// Save URL and move to method selection
		m.url = m.urlInput.Value()
		if m.url == "" {
			return m, nil
		}
		m.currentScreen = methodScreen
		return m, nil

	case methodScreen:
		// Save selected method
		if selectedItem, ok := m.methodList.SelectedItem().(item); ok {
			m.method = selectedItem.title
		}
		// Skip headers for GET/DELETE, go to body for POST/PUT
		if m.method == "POST" || m.method == "PUT" {
			m.currentScreen = headersScreen
			m.headersInput.Focus()
		} else {
			m.currentScreen = confirmScreen
		}
		return m, nil

	case headersScreen:
		// Save headers and move to body input
		m.headers = m.headersInput.Value()
		m.currentScreen = bodyScreen
		m.bodyInput.Focus()
		return m, nil

	case bodyScreen:
		// Save body and move to confirm
		m.body = m.bodyInput.Value()
		m.currentScreen = confirmScreen
		return m, nil

	case confirmScreen:
		// Execute the request
		m.currentScreen = loadingScreen
		// clear any previous cancel state
		m.canceled = false
		// create cancelable context for the request
		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel
		return m, tea.Batch(
			m.spinner.Tick,
			m.executeRequest(ctx),
		)

	case responseScreen:
	// Start a short reset progress animation, then go to URL input
	m.currentScreen = resettingScreen
	m.resetPercent = 0
	// reset the visual progress to 0 immediately
	cmd := m.progress.SetPercent(0)
	return m, tea.Batch(cmd, tickReset())
	}

	return m, nil
}

// Focus the appropriate input field for current screen
func (m *model) focusCurrentScreen() {
	switch m.currentScreen {
	case urlScreen:
		m.urlInput.Focus()
	case headersScreen:
		m.headersInput.Focus()
	case bodyScreen:
		m.bodyInput.Focus()
	}
}

// Reset inputs and view state without returning to intro
func (m *model) resetInputs() {
	m.url = ""
	m.method = ""
	m.headers = ""
	m.body = ""
	m.response = ""
	m.err = nil
	m.urlInput.SetValue("")
	m.headersInput.SetValue("")
	m.bodyInput.SetValue("")
	m.methodList.Select(0)
	if m.ready {
		m.viewport.SetContent("")
		m.viewport.GotoTop()
	}
}

// Execute the HTTP request via bash script
func (m *model) executeRequest(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		// Build command arguments
		args := []string{m.method, m.url}

		// Add headers if provided
		if m.headers != "" {
			args = append(args, m.headers)
		} else {
			args = append(args, "")
		}

		// Add body if provided
		if m.body != "" {
			args = append(args, m.body)
		}

		// Execute bash script with cancelable context
		cmd := exec.CommandContext(ctx, "./request.sh", args...)
		output, err := cmd.CombinedOutput()

		return responseMsg{
			output: string(output),
			err:    err,
		}
	}
}

// Apply responsive sizes to inputs and content views
func (m *model) applyLayoutSizes() {
	if m.width == 0 || m.height == 0 {
		return
	}
	contentWidth := clampInt(30, 120, m.width-8)
	m.urlInput.Width = contentWidth
	m.headersInput.SetWidth(contentWidth)
	m.bodyInput.SetWidth(contentWidth)

	// Heights as a function of terminal height with sane bounds
	headersH := clampInt(4, 12, m.height/6)
	bodyH := clampInt(6, m.height-18, (m.height*3)/10)
	m.headersInput.SetHeight(headersH)
	m.bodyInput.SetHeight(bodyH)

	// Viewport already updated on WindowSize; ensure min sizes
	if m.ready {
		m.viewport.Width = maxInt(20, m.width-4)
		m.viewport.Height = maxInt(5, m.height-10)
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func clampInt(min, max, v int) int { return maxInt(min, minInt(max, v)) }

// ===== Animation tick handling =====
type animTickMsg struct{}

func tickAnim() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return animTickMsg{} })
}

// ===== Reset progress ticking =====
type resetTickMsg struct{}

// Configure reset progress timing
const (
	resetTotalDuration = 3 * time.Second
	resetTickInterval  = 100 * time.Millisecond
)

func tickReset() tea.Cmd {
	return tea.Tick(resetTickInterval, func(time.Time) tea.Msg { return resetTickMsg{} })
}

// Render the UI based on current screen
func (m model) View() string {
	var s string

	// Title bar
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		MarginBottom(1)

	if m.currentScreen == introScreen {
		// Intro: animated border with APP_ART inside
		s = m.renderAnimatedFrame()
		return s
	}

	switch m.currentScreen {
	case urlScreen:
		// Only render large art when the terminal is wide enough
		if m.width >= 100 {
			artStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")).
				Bold(true).
				MarginBottom(1).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("205"))

			s = artStyle.Render(APP_ART) + "\n"
		}
		s += titleStyle.Render("🌐 Postman CLI - Enter URL") + "\n\n"
		s += m.urlInput.View() + "\n\n"
		s += helpStyle("Press Enter to continue • Ctrl+C to quit")

	case methodScreen:
		s = titleStyle.Render("🔧 Select HTTP Method") + "\n\n"
		s += m.methodList.View() + "\n"
		s += helpStyle("↑/↓ to navigate • Enter to select • Esc to go back")

	case headersScreen:
		s = titleStyle.Render("📋 Enter Headers (Optional)") + "\n\n"
		s += "One header per line (e.g., Content-Type: application/json)\n\n"
		s += m.headersInput.View() + "\n\n"
		s += helpStyle("Enter to continue • Esc to go back")

	case bodyScreen:
		s = titleStyle.Render("📝 Enter Request Body") + "\n\n"
		s += m.bodyInput.View() + "\n\n"
		s += helpStyle("Enter to continue • Esc to go back")

	case confirmScreen:
		s = titleStyle.Render("🚀 Ready to Send") + "\n\n"
		s += boxStyle(fmt.Sprintf("Method: %s\nURL: %s", m.method, m.url))
		if m.headers != "" {
			s += "\n\n" + boxStyle("Headers:\n"+m.headers)
		}
		if m.body != "" {
			s += "\n\n" + boxStyle("Body:\n"+m.body)
		}
		s += "\n\n" + helpStyle("Press Enter to send request • Esc to go back")

	case loadingScreen:
		s = titleStyle.Render("⏳ Sending Request...") + "\n\n"
		s += m.spinner.View() + " Please wait...\n"

	case responseScreen:
		s = titleStyle.Render("✅ Response") + "\n\n"
		if !m.ready || m.response == "" {
			s += "Preparing response..."
		} else {
			s += m.viewport.View() + "\n\n"
			s += helpStyle(fmt.Sprintf("↑/↓ to scroll • Page: %d%% • Enter for new request • Q to quit",
				int(m.viewport.ScrollPercent()*100)))
		}

	case resettingScreen:
		s = titleStyle.Render("🔁 Returning to start...") + "\n\n"
		s += m.progress.View() + "\n\n"
		s += helpStyle("Press Esc to skip")
	}
	// Wrap everything inside persistent border frame after intro
	if m.boxActive {
		return m.renderFrameWithContent(s)
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(s)
}

// Helper function for help text styling
func helpStyle(s string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(s)
}

// Helper function for box styling
func boxStyle(s string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		Render(s)
}

// ===== Frame rendering helpers =====

// renderAnimatedFrame draws a progressive border animation and APP_ART inside.
func (m model) renderAnimatedFrame() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}
	// Ensure minimums to avoid negative sizes
	outerW := maxInt(10, m.width)
	outerH := maxInt(6, m.height)
	innerW := maxInt(6, outerW-2)
	innerH := maxInt(3, outerH-2)

	topCount := minInt(m.animTopProgress, innerW)
	sideCount := minInt(m.animSideProgress, innerH)

	var b strings.Builder
	// Top
	b.WriteString("┌")
	b.WriteString(strings.Repeat("─", topCount))
	if innerW-topCount > 0 {
		b.WriteString(strings.Repeat(" ", innerW-topCount))
	}
	b.WriteString("┐\n")

	// Middle with sides growth
	artLines := prepareIntroArt(innerW)
	artStartRow := maxInt(0, (innerH-len(artLines))/2)

	for row := 0; row < innerH; row++ {
		left := " "
		right := " "
		if row < sideCount {
			left, right = "│", "│"
		}
		b.WriteString(left)
		var line string
		if row >= artStartRow && row < artStartRow+len(artLines) {
			line = artLines[row-artStartRow]
			// ensure exact width
			line = padOrTrim(line, innerW)
		} else {
			line = strings.Repeat(" ", innerW)
		}
		b.WriteString(line)
		b.WriteString(right)
		b.WriteString("\n")
	}

	// Bottom
	b.WriteString("└")
	b.WriteString(strings.Repeat("─", topCount))
	if innerW-topCount > 0 {
		b.WriteString(strings.Repeat(" ", innerW-topCount))
	}
	b.WriteString("┘")

	// Colorize border a bit using lipgloss by wrapping
	frame := b.String()
	// simple color: we won't recolor mixed lines; rely on terminal default
	return frame
}

// renderFrameWithContent draws a full border and places given content inside, wrapping as needed.
func (m model) renderFrameWithContent(content string) string {
	if m.width == 0 || m.height == 0 {
		return content
	}
	outerW := maxInt(10, m.width)
	outerH := maxInt(6, m.height)
	innerW := maxInt(6, outerW-2)
	innerH := maxInt(3, outerH-2)

	// Wrap content into lines of innerW
	var lines []string
	for _, ln := range strings.Split(content, "\n") {
		chunks := wrapLine(ln, innerW)
		lines = append(lines, chunks...)
	}
	// Fit exactly innerH lines
	if len(lines) < innerH {
		pad := make([]string, innerH-len(lines))
		for i := range pad {
			pad[i] = ""
		}
		lines = append(lines, pad...)
	} else if len(lines) > innerH {
		lines = lines[:innerH]
	}

	var b strings.Builder
	// Top
	b.WriteString("┌")
	b.WriteString(strings.Repeat("─", innerW))
	b.WriteString("┐\n")
	// Middle
	for _, ln := range lines {
		b.WriteString("│")
		b.WriteString(padOrTrim(ln, innerW))
		b.WriteString("│\n")
	}
	// Bottom
	b.WriteString("└")
	b.WriteString(strings.Repeat("─", innerW))
	b.WriteString("┘")
	return b.String()
}

// prepareIntroArt centers APP_ART and a hint inside given width.
func prepareIntroArt(innerW int) []string {
	hint := "Press Enter to skip animation…"
	art := strings.TrimRight(APP_ART, "\n")
	lines := strings.Split(art, "\n")
	// center each line and trim to width
	for i, ln := range lines {
		ln = strings.TrimRight(ln, " ")
		lines[i] = centerLine(ln, innerW)
	}
	// add spacing and hint
	lines = append(lines, "")
	lines = append(lines, centerLine(hint, innerW))
	return lines
}

func visibleLen(s string) int { return len([]rune(s)) }

func padOrTrim(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		return string(r[:w])
	}
	if len(r) < w {
		return string(r) + strings.Repeat(" ", w-len(r))
	}
	return s
}

func centerLine(s string, w int) string {
	r := []rune(s)
	if len(r) >= w {
		return string(r[:w])
	}
	pad := (w - len(r)) / 2
	return strings.Repeat(" ", pad) + string(r) + strings.Repeat(" ", w-pad-len(r))
}

func wrapLine(s string, w int) []string {
	if w <= 0 {
		return []string{""}
	}
	r := []rune(s)
	var out []string
	for len(r) > w {
		out = append(out, string(r[:w]))
		r = r[w:]
	}
	out = append(out, padOrTrim(string(r), w))
	return out
}

func main() {
	// Check if request.sh exists
	if _, err := os.Stat("./request.sh"); os.IsNotExist(err) {
		fmt.Println("Error: request.sh not found in current directory")
		os.Exit(1)
	}

	// Start the Bubble Tea program
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
