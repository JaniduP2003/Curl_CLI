package main

import (
	"fmt"
	"os"
	"os/exec"
	 

	"github.com/charmbracelet/bubbles/list"
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
	urlScreen screen = iota
	methodScreen
	headersScreen
	bodyScreen
	confirmScreen
	loadingScreen
	responseScreen
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
	spinner      spinner.Model
	viewport     viewport.Model
	
	// Data storage
	url          string
	method       string
	headers      string
	body         string
	response     string
	
	// UI state
	ready        bool
	err          error
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

	return model{
		currentScreen: urlScreen,
		urlInput:      ti,
		methodList:    methodList,
		headersInput:  headersInput,
		bodyInput:     bodyInput,
		spinner:       s,
		ready:         false,
	}
}

// Initialize command - starts the spinner
func (m model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.spinner.Tick,
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
			}

		case "esc":
			// Go back to previous screen
			if m.currentScreen > urlScreen && m.currentScreen != loadingScreen {
				m.currentScreen--
				m.focusCurrentScreen()
				return m, nil
			}

		case "enter":
			return m.handleEnter()
		}

	case tea.WindowSizeMsg:
		// Handle window resize and initialize viewport
		if !m.ready {
			m.viewport = viewport.New(msg.Width-4, msg.Height-10)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - 4
			m.viewport.Height = msg.Height - 10
		}
		m.methodList.SetSize(msg.Width-4, msg.Height-4)

	case responseMsg:
		// Received response from bash script
		if msg.err != nil {
			m.err = msg.err
			m.response = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.response = msg.output
		}
		m.currentScreen = responseScreen
		
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
		return m, tea.Batch(
			m.spinner.Tick,
			m.executeRequest(),
		)

	case responseScreen:
		// Reset and start over
		*m = initialModel()
		return m, textinput.Blink
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

// Execute the HTTP request via bash script
func (m *model) executeRequest() tea.Cmd {
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

		// Execute bash script
		cmd := exec.Command("./request.sh", args...)
		output, err := cmd.CombinedOutput()
		
		return responseMsg{
			output: string(output),
			err:    err,
		}
	}
}

// Render the UI based on current screen
func (m model) View() string {
	var s string

	// Title bar
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		MarginBottom(1)

	switch m.currentScreen {
	case urlScreen:

			artStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")). // Purple color for the art
				Bold(true).
				MarginBottom(1).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("205"))

			s = artStyle.Render(APP_ART) + "\n"
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