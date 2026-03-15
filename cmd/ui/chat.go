package ui

// These imports will be used later in the tutorial. If you save the file
// now, Go might complain they are unused, but that's fine.
// You may also need to run `go mod tidy` to download bubbletea and its
// dependencies.
import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"pkgs.rlsvarinskis.xyz/go/coding-agent/cmd/agent"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
)

type newMessageMsg *agent.SessionPhase
type updatedMessageMsg *agent.SessionPhase
type doneMessageMsg *agent.SessionPhase
type promptMsg string
type sessionClosedMsg struct{}

type summaryMsg struct {
	message int
	content string
	summary string
}

type chatModel struct {
	ctx        context.Context
	windowSize tea.WindowSizeMsg

	session   *agent.Session
	messages  []*agent.SessionPhase
	summaries []string

	summarizer agent.Summarizer
	summaryWIP *summaryMsg

	waitingForUser bool

	viewport viewport.Model
	textarea textarea.Model
}

func NewChat(session *agent.Session, summarizer agent.Summarizer) chatModel {
	ta := textarea.New()
	ta.Placeholder = "What do you want to build?"
	ta.Blur()
	ta.SetVirtualCursor(true)

	ta.Prompt = "> "
	ta.CharLimit = 280
	ta.SetWidth(30)
	ta.SetHeight(3)

	s := ta.Styles()
	s.Focused.CursorLine = lipgloss.NewStyle()
	ta.SetStyles(s)

	ta.ShowLineNumbers = true
	ta.KeyMap.InsertNewline.SetEnabled(true)

	vp := viewport.New(viewport.WithWidth(30), viewport.WithHeight(5))
	vp.SetContent(`Welcome to your new AI agent.
Type a message and press Enter to send. (Ctrl+Enter for a newline)`)
	vp.KeyMap.Left.SetEnabled(false)
	vp.KeyMap.Right.SetEnabled(false)

	return chatModel{
		session:    session,
		summarizer: summarizer,
		textarea:   ta,
		viewport:   vp,
	}
}

func (chat chatModel) processAgentUpdate() tea.Msg {
	select {
	case message, ok := <-chat.session.Log:
		if !ok {
			chat.session.Log = nil
			return sessionClosedMsg(struct{}{})
		}
		if len(message.Content) == 0 {
			return newMessageMsg(message)
		} else if !message.Done {
			return updatedMessageMsg(message)
		} else {
			return doneMessageMsg(message)
		}
	case prompt, ok := <-chat.session.UserIO:
		if !ok {
			chat.session.UserIO = nil
			return sessionClosedMsg(struct{}{})
		}
		return promptMsg(prompt)
	}
}

func (chat chatModel) summarize() tea.Msg {
	summary, err := chat.summarizer.Summarize(chat.summaryWIP.content)
	if err != nil {
		chat.summaryWIP.summary = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(err.Error())
	} else {
		chat.summaryWIP.summary = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack).Render(summary)
	}
	return *chat.summaryWIP
}

func (chat *chatModel) renderMessages() {
	messages := make([]string, len(chat.messages))
	for i, m := range chat.messages {
		switch m.Type {
		case agent.Hidden:
			messages[i] = fmt.Sprintf("[%s] %s", m.Phase, m.Message.Role)
		case agent.Collapsible:
			messages[i] = fmt.Sprintf("[%s] %s: %s\n (tokens: %d, tokens2: %d)", m.Phase, m.Message.Role, chat.summaries[i], m.PromptTokens, m.ResponseTokens)
		case agent.UserInput:
			messages[i] = fmt.Sprintf("[%s] %s: %s\n (tokens: %d, tokens2: %d)", m.Phase, m.Message.Role, m.Message.Content, m.PromptTokens, m.ResponseTokens)
		case agent.Visible:
			messages[i] = fmt.Sprintf("[%s] %s: %s\n (tokens: %d, tokens2: %d)", m.Phase, m.Message.Role, m.Message.Content, m.PromptTokens, m.ResponseTokens)
		}
	}
	chat.viewport.SetContent(lipgloss.NewStyle().Width(chat.viewport.Width()).Render(strings.Join(messages, "\n")))
	chat.viewport.GotoBottom()
}

func (chat chatModel) Init() tea.Cmd {
	return chat.processAgentUpdate
}

func (chat chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case newMessageMsg:
		chat.messages = append(chat.messages, msg)
		chat.summaries = append(chat.summaries, "")
		chat.renderMessages()
		return chat, chat.processAgentUpdate
	case updatedMessageMsg:
		chat.messages[len(chat.messages)-1] = msg
		chat.renderMessages()
		if msg.Type == agent.Collapsible && strings.Index(msg.Message.Content, "\n") >= 0 && chat.summaryWIP == nil {
			chat.summaryWIP = &summaryMsg{
				message: len(chat.messages) - 1,
				content: msg.Message.Content,
			}
			return chat, tea.Batch(chat.processAgentUpdate, chat.summarize)
		}
		return chat, chat.processAgentUpdate
	case doneMessageMsg:
		chat.messages[len(chat.messages)-1] = msg
		chat.renderMessages()
		if msg.Type == agent.Collapsible && chat.summaryWIP == nil {
			chat.summaryWIP = &summaryMsg{
				message: len(chat.messages) - 1,
				content: msg.Message.Content,
			}
			return chat, tea.Batch(chat.processAgentUpdate, chat.summarize)
		}
		return chat, chat.processAgentUpdate
	case promptMsg:
		chat.waitingForUser = true
		chat.textarea.Placeholder = string(msg)
		return chat, chat.textarea.Focus()

	case summaryMsg:
		chat.summaries[msg.message] = msg.summary
		chat.renderMessages()
		if chat.messages[msg.message].Message.Content != msg.content {
			// Content has changed, let's update the summary finally
			chat.summaryWIP = &summaryMsg{
				message: msg.message,
				content: chat.messages[msg.message].Message.Content,
			}
			return chat, chat.summarize
		} else {
			// We have the final summary, let's find the next message needing a summary
			for i := msg.message + 1; i < len(chat.summaries); i++ {
				if chat.messages[i].Type == agent.Collapsible && chat.messages[i].Done {
					// We found a message to summarize
					chat.summaryWIP = &summaryMsg{
						message: i,
						content: chat.messages[i].Message.Content,
					}
					return chat, chat.summarize
				}
			}
			chat.summaryWIP = nil
			return chat, nil
		}

	case tea.WindowSizeMsg:
		chat.windowSize = msg
		chat.viewport.SetWidth(msg.Width)
		chat.textarea.SetWidth(msg.Width)
		chat.viewport.SetHeight(msg.Height - chat.textarea.Height())
		chat.renderMessages()
		chat.viewport.GotoBottom()
		return chat, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			fmt.Println(chat.textarea.Value())
			return chat, tea.Quit
		case "enter":
			chat.session.UserIO <- chat.textarea.Value()
			chat.textarea.Reset()
			chat.viewport.GotoBottom()
			chat.textarea.Blur()
			return chat, chat.processAgentUpdate
		default:
			var cmd tea.Cmd
			chat.textarea, cmd = chat.textarea.Update(msg)
			return chat, cmd
		}
	case cursor.BlinkMsg:
		var cmd tea.Cmd
		chat.textarea, cmd = chat.textarea.Update(msg)
		return chat, cmd
	}

	return chat, nil
}

func (chat chatModel) View() tea.View {
	viewportView := chat.viewport.View()
	v := tea.NewView(viewportView + "\n" + chat.textarea.View())
	c := chat.textarea.Cursor()
	if c != nil {
		c.Y += lipgloss.Height(viewportView)
	}
	v.Cursor = c
	v.AltScreen = true
	return v
}
