package agent

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ollama/ollama/api"
)

type Summarizer struct {
	ctx    context.Context
	client *api.Client
	model  string
}

func NewSummarizer(ctx context.Context, client *api.Client, model string) Summarizer {
	return Summarizer{
		ctx:    ctx,
		client: client,
		model:  model,
	}
}

func (s Summarizer) Summarize(prompt, text string) (string, error) {
	stream := false
	chat := &api.GenerateRequest{
		Model:   s.model,
		Format:  json.RawMessage{},
		Stream:  &stream,
		System:  prompt,
		Prompt:  text,
		Options: map[string]any{
			// "stop":        []string{"</"},
			// "temperature": 0,
		},
	}
	var summary string
	err := s.client.Generate(s.ctx, chat, func(cr api.GenerateResponse) error {
		summary = cr.Response
		return nil
	})
	return strings.Trim(summary, " \n\t"), err
}
