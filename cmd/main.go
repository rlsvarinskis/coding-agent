package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/ollama/ollama/api"
	"pkgs.rlsvarinskis.xyz/go/coding-agent/cmd/agent"
	"pkgs.rlsvarinskis.xyz/go/coding-agent/cmd/ui"
)

func main() {
	// Command-line flags
	host := flag.String("host", "http://192.168.0.164:11434", "Ollama server host")
	model := flag.String("model", "gemma3:12b", "Model to use")
	flag.Parse()

	u, err := url.Parse(*host)
	if err != nil {
		fmt.Printf("Failed to create URL: %w\n", err)
		return
	}

	ctx := context.Background()
	client := api.NewClient(u, http.DefaultClient)
	session := agent.Start(ctx, client, *model, NewArchitect())
	summarizer := agent.NewSummarizer(ctx, client, *model)

	if len(os.Getenv("DEBUG")) >= 0 {
		f, err := tea.LogToFile("debug.log", "debug")
		if err != nil {
			fmt.Println("fatal:", err)
			os.Exit(1)
		}
		defer f.Close()
	}
	p := tea.NewProgram(ui.NewChat(session, summarizer))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error! %w\n", err)
	}
}
