package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/codejedi-ai/adkgobot/internal/config"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gorilla/websocket"
)

type chatMsg struct {
	text string
}

type errMsg struct {
	err error
}

type model struct {
	conn    *websocket.Conn
	input   textinput.Model
	lines   []string
	width   int
	height  int
	status  string
	quiting bool
}

func New(wsURL string) (*model, error) {
	if wsURL == "" {
		wsURL = config.DefaultGatewayWS
	}
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, err
	}

	in := textinput.New()
	in.Placeholder = "Type message and press Enter"
	in.Focus()
	in.CharLimit = 2000
	in.Width = 80

	m := &model{
		conn:   c,
		input:  in,
		lines:  []string{"adkbot tui connected", "type /quit to exit"},
		status: "connected",
	}
	return m, nil
}

func (m *model) Init() tea.Cmd {
	return m.readLoop()
}

func (m *model) readLoop() tea.Cmd {
	return func() tea.Msg {
		for {
			_, payload, err := m.conn.ReadMessage()
			if err != nil {
				return errMsg{err: err}
			}
			var resp struct {
				Type  string          `json:"type"`
				Data  json.RawMessage `json:"data"`
				Error string          `json:"error"`
			}
			if err := json.Unmarshal(payload, &resp); err != nil {
				return errMsg{err: err}
			}
			if resp.Error != "" {
				return chatMsg{text: "error: " + resp.Error}
			}
			if resp.Type == "chat" {
				var data struct {
					Reply string `json:"reply"`
				}
				if err := json.Unmarshal(resp.Data, &data); err != nil {
					return errMsg{err: err}
				}
				return chatMsg{text: "adkbot> " + data.Reply}
			}
			return chatMsg{text: fmt.Sprintf("%s> %s", resp.Type, string(resp.Data))}
		}
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = msg.Width - 4
		return m, nil
	case chatMsg:
		m.lines = append(m.lines, msg.text)
		return m, m.readLoop()
	case errMsg:
		m.status = "disconnected"
		m.lines = append(m.lines, "connection closed: "+msg.err.Error())
		m.quiting = true
		return m, tea.Quit
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quiting = true
			_ = m.conn.Close()
			return m, tea.Quit
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			if text == "/quit" {
				m.quiting = true
				_ = m.conn.Close()
				return m, tea.Quit
			}
			m.lines = append(m.lines, "you> "+text)
			payload := map[string]any{"type": "chat", "content": text}
			if err := m.conn.WriteJSON(payload); err != nil {
				m.lines = append(m.lines, "error: "+err.Error())
			}
			m.input.SetValue("")
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *model) View() string {
	if m.quiting {
		return "\nBye\n"
	}
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).Render("adkbot tui")
	status := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("status: " + m.status)
	body := strings.Join(m.lines, "\n")
	footer := m.input.View()
	return fmt.Sprintf("%s\n%s\n\n%s\n\n%s", header, status, body, footer)
}

func Run(wsURL string) error {
	m, err := New(wsURL)
	if err != nil {
		return err
	}
	defer m.conn.Close()
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
