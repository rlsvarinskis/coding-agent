package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ollama/ollama/api"
)

type Target int

const (
	System Target = iota
	User
	AI
	Error
)

const (
	Hidden      = "hidden"
	Collapsible = "collapsible"
	UserInput   = "user-input"
	Visible     = "text"
)

// An Agent is a state graph of prompts and interactions.
type Agent struct {
	start *Phase
}

func NewAgent(start *Phase) Agent {
	return Agent{
		start: start,
	}
}

// A Phase in an agentic loop is a prompt and interaction that leads to a new
// phase.
type Phase struct {
	Id        string              // The unique name of this phase
	Type      string              // How should this prompt appear in UI?
	Target    Target              // Who should respond to this phase
	Prompt    string              // What should be prompted during this phase. If the target is System, then this gets appended to the chat history, if it's User, it gets printed to the user, otherwise it does nothing.
	Ttl       int                 // How long this phase should remain in the context, with 0 meaning indefinite
	NextPhase func(string) *Phase // Given a response, what should the next phase be
}

// A PhaseGroup is a phase that encompasses multiple sub-phases.
// This allows controlling the context with everything in this phase group together.
type PhaseGroup struct {
	id     string
	start  string
	phases map[string]Phase
	ttl    int
}

type AgentPhase interface {
}

type Session struct {
	UserIO chan string
	Log    <-chan *SessionPhase
}

type SessionPhase struct {
	Phase   string
	Type    string
	Target  Target
	Message api.Message
	Content []string
	Ttl     int
	Done    bool

	DebugTemplate  string
	PromptTokens   int
	ResponseTokens int
}

func Start(ctx context.Context, client *api.Client, model string, agent Agent) *Session {
	stream := true
	chat := &api.ChatRequest{
		Model:  model,
		Format: json.RawMessage{},
		Stream: &stream,
		Options: map[string]any{
			"num_ctx": 65536,
			// "stop":        []string{"</"},
			// "temperature": 0,
		},
	}

	userIo := make(chan string)
	messageLog := make(chan *SessionPhase)
	go func() {
		defer close(messageLog)
		defer close(userIo)
		var history []SessionPhase
		p := agent.start
		for ctx.Err() == nil && p != nil {
			phase := SessionPhase{
				Phase:  p.Id,
				Type:   p.Type,
				Target: p.Target,
				Ttl:    p.Ttl,
			}
			switch p.Target {
			case System:
				phase.Message = api.Message{
					Role:    "system",
					Content: p.Prompt,
				}
				phase.Done = true
			case User:
				userIo <- p.Prompt
				phase.Message = api.Message{
					Role:    "user",
					Content: <-userIo,
				}
				phase.Done = true
			case AI:
				currentTurn := len(history)
				chat.Messages = make([]api.Message, len(history))
				for turn, message := range history {
					if message.Ttl > 0 && turn+message.Ttl > currentTurn {
						continue
					}
					chat.Messages = append(chat.Messages, message.Message)
				}
				fullMessage := ""
				messageLog <- &phase
				err := client.Chat(ctx, chat, func(cr api.ChatResponse) error {
					// phase.DebugTemplate = cr.DebugInfo.RenderedTemplate
					phase.PromptTokens = cr.PromptEvalCount
					phase.ResponseTokens = cr.EvalCount
					phase.Content = append(phase.Content, cr.Message.Content)
					fullMessage += cr.Message.Content
					phase.Message = cr.Message
					phase.Message.Content = fullMessage
					phase.Done = cr.Done
					if cr.Done {
						phase.Message.Content = fullMessage
					} else {
						messageLog <- &phase
					}
					return nil
				})
				if err != nil {
					fmt.Printf("Error: %v\n", err)
					return
				}
			}
			history = append(history, phase)
			messageLog <- &phase
			p = p.NextPhase(phase.Message.Content)
		}
	}()

	return &Session{
		UserIO: userIo,
		Log:    messageLog,
	}
}
