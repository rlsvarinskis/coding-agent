package main

import (
	_ "embed"
	"encoding/xml"
	"fmt"
	"path"
	"strings"

	"pkgs.rlsvarinskis.xyz/go/coding-agent/cmd/agent"
)

var (
	//go:embed agents/engineer/AGENT.md
	systemTemplate string

	//go:embed agents/engineer/phases/thinking-prompt.md
	systemThinkPhase string

	//go:embed agents/engineer/phases/command-prompt.md
	systemCommandPhase string
)

var (
	systemPrompt = agent.Phase{
		Id:     "start",
		Type:   agent.Hidden,
		Target: agent.System,
		Prompt: systemTemplate,
		NextPhase: func(s string) *agent.Phase {
			return phaseUserInput("What do you want to build today?")
		},
	}
	thinkingPrompt = agent.Phase{
		Id:     "thinking-prompt",
		Target: agent.System,
		Prompt: systemThinkPhase,
		Ttl:    1,
		NextPhase: func(s string) *agent.Phase {
			return &thinkingPhase
		},
	}
	thinkingPhase = agent.Phase{
		Id:     "thinking-phase",
		Type:   agent.Collapsible,
		Target: agent.AI,
		Ttl:    18,
		NextPhase: func(s string) *agent.Phase {
			return &toolPrompt
		},
	}
	toolPrompt = agent.Phase{
		Id:     "tool-prompt",
		Type:   agent.Hidden,
		Target: agent.System,
		Prompt: systemCommandPhase,
		Ttl:    1,
		NextPhase: func(s string) *agent.Phase {
			return &toolPhase
		},
	}
	toolPhase = agent.Phase{
		Id:     "tool-phase",
		Type:   agent.Visible,
		Target: agent.AI,
		NextPhase: func(s string) *agent.Phase {
			return getPhaseExecuteTool(s)
		},
	}

	getPhaseUserInput   func(string) *agent.Phase
	getPhaseExecuteTool func(string) *agent.Phase
)

func phaseUserInput(prompt string) *agent.Phase {
	return &agent.Phase{
		Id:     "user-input",
		Type:   agent.UserInput,
		Target: agent.User,
		Prompt: prompt,
		NextPhase: func(s string) *agent.Phase {
			return &thinkingPrompt
		},
	}
}

func phaseInvalidTool(prompt string) *agent.Phase {
	return &agent.Phase{
		Id:     "invalid-tool",
		Type:   agent.Visible,
		Target: agent.System,
		Prompt: prompt,
		NextPhase: func(s string) *agent.Phase {
			return &thinkingPrompt
		},
	}
}

func phaseToolResponse(prompt string) *agent.Phase {
	return &agent.Phase{
		Id:     "tool-response",
		Type:   agent.Visible,
		Target: agent.System,
		Prompt: prompt,
		NextPhase: func(s string) *agent.Phase {
			return &thinkingPrompt
		},
	}
}

type LeftOverAttrs struct {
	BadAttrs []xml.Attr `xml:",any,attr"`
}

type AskUserCommand struct {
	LeftOverAttrs
	XMLName xml.Name     `xml:"ask_user"`
	Prompt  xml.CharData `xml:",innerxml"`
}

type MkdirCommand struct {
	LeftOverAttrs
	XMLName xml.Name `xml:"mkdir"`
	File    string   `xml:"file,attr"`
}

type LsCommand struct {
	LeftOverAttrs
	XMLName xml.Name `xml:"ls"`
	File    string   `xml:"file,attr"`
}

type WriteFileCommand struct {
	LeftOverAttrs
	XMLName xml.Name     `xml:"write_file"`
	File    string       `xml:"file,attr"`
	Start   int          `xml:"start-line,attr"`
	End     int          `xml:"end-line,attr"`
	Data    xml.CharData `xml:",innerxml"`
}

type ReadFileCommand struct {
	LeftOverAttrs
	XMLName xml.Name `xml:"read_file"`
	File    string   `xml:"file,attr"`
	Start   int      `xml:"start-line,attr"`
	End     int      `xml:"end-line,attr"`
}

type RmCommand struct {
	LeftOverAttrs
	XMLName xml.Name `xml:"delete_file"`
	File    string   `xml:"file,attr"`
}

type ToolCall struct {
	XMLName   xml.Name           `xml:"tool"`
	AskUser   []AskUserCommand   `xml:"ask_user"`
	Mkdir     []MkdirCommand     `xml:"mkdir"`
	Ls        []LsCommand        `xml:"ls"`
	WriteFile []WriteFileCommand `xml:"write_file"`
	ReadFile  []ReadFileCommand  `xml:"read_file"`
	Rm        []RmCommand        `xml:"delete_file"`
}

type directory struct {
	entries map[string]any
}

func (d directory) find(p string) (directory, string, error) {
	paths := strings.Split(path.Clean(p), "/")
	t := d
	for _, x := range paths[:len(paths)-1] {
		if x == "." || x == "" {
			continue
		}
		c, ok := t.entries[x]
		if !ok {
			return directory{}, "", fmt.Errorf("No such file or directory")
		}
		switch d := c.(type) {
		case directory:
			t = d
		case string:
			return directory{}, "", fmt.Errorf("Not a directory")
		}
	}

	last := paths[len(paths)-1]
	if last == "." || last == "" {
		return directory{}, "", fmt.Errorf("Permission denied")
	}
	return t, paths[len(paths)-1], nil
}

func (d directory) find2(p string) (directory, string, error) {
	paths := strings.Split(path.Clean(p), "/")
	t := d
	for _, x := range paths[:len(paths)-1] {
		if x == "." || x == "" {
			continue
		}
		c, ok := t.entries[x]
		if !ok {
			return directory{}, "", fmt.Errorf("No such file or directory")
		}
		switch d := c.(type) {
		case directory:
			t = d
		case string:
			return directory{}, "", fmt.Errorf("Not a directory")
		}
	}

	last := paths[len(paths)-1]
	if last == "." || last == "" {
		last = ""
	}
	return t, last, nil
}

func (d directory) mkdir(p string) string {
	t, child, err := d.find(p)
	if err != nil {
		return fmt.Sprintf("mkdir: cannot create directory `%s`: %s", p, err.Error())
	}
	_, ok := t.entries[child]
	if ok {
		return "mkdir: cannot create directory `" + p + "`: File exists"
	}
	t.entries[child] = directory{
		entries: make(map[string]any),
	}
	return "mkdir: success"
}

func (d directory) ls(p string) string {
	t, child, err := d.find2(p)
	if err != nil {
		return fmt.Sprintf("ls: cannot access `%s`: %s", p, err.Error())
	}
	f, ok := t.entries[child]
	if child == "" {
		f = t
	} else if !ok {
		return "ls: cannot access `" + p + "`: No such file or directory"
	}
	switch d := f.(type) {
	case directory:
		res := fmt.Sprintf("ls: found %d files:", len(d.entries))
		for k := range d.entries {
			res += "\n" + k
		}
		return res
	default:
		return p
	}
}

func (d directory) rm(p string) string {
	t, child, err := d.find(p)
	if err != nil {
		return fmt.Sprintf("rm: cannot remove `%s`: %s", p, err.Error())
	}
	f, ok := t.entries[child]
	if !ok {
		return "rm: cannot remove `" + p + "`: No such file or directory"
	}
	switch d := f.(type) {
	case directory:
		if len(d.entries) > 0 {
			return "rm: failed to remove `" + p + "`: Directory not empty"
		} else {
			delete(t.entries, child)
			return "rm: success"
		}
	default:
		delete(t.entries, child)
		return "rm: success"
	}
}

func (d directory) write(cmd WriteFileCommand) string {
	t, child, err := d.find(cmd.File)
	if err != nil {
		return fmt.Sprintf("write_file: cannot write to `%s`: %s", cmd.File, err.Error())
	}
	f, ok := t.entries[child]
	if !ok {
		f = ""
	}
	switch d := f.(type) {
	case directory:
		return "write_file: cannot write to `" + cmd.File + "`: Is a directory"
	case string:
		lines := strings.Split(d, "\n")
		data := strings.Split(string(cmd.Data), "\n")
		if data[0] != "" || data[len(data)-1] != "" {
			return fmt.Sprintf("write_file: invalid body provided, body must be on separate lines from XML tags")
		}
		if cmd.Start == 0 {
			cmd.Start = len(lines)
		}
		if cmd.End == 0 {
			cmd.End = cmd.Start
		}
		if cmd.End < cmd.Start {
			return fmt.Sprintf("write_file: cannot write to lines %d-%d: end cannot be before start", cmd.Start, cmd.End)
		}
		if cmd.Start < 1 {
			return fmt.Sprintf("write_file: cannot write to lines %d-%d: start cannot be before 1", cmd.Start, cmd.End)
		}
		if cmd.End > len(lines) {
			return fmt.Sprintf("write_file: cannot write to lines %d-%d: end cannot be after the end of the file", cmd.Start, cmd.End)
		}

		cmd.Start--
		cmd.End--
		data = append(lines[:cmd.Start], data...)
		data = append(data, lines[cmd.End:]...)

		t.entries[child] = strings.Join(data, "\n")
		return "write_file: success"
	}
	return "write_file: ERROR ERROR ERROR ERROR ERROR"
}

func (d directory) read(cmd ReadFileCommand) string {
	t, child, err := d.find(cmd.File)
	if err != nil {
		return fmt.Sprintf("read_file: cannot read `%s`: %s", cmd.File, err.Error())
	}
	f, ok := t.entries[child]
	if !ok {
		return "read_file: cannot read `" + cmd.File + "`: No such file or directory"
	}
	switch d := f.(type) {
	case directory:
		return "read_file: cannot read `" + cmd.File + "`: Is a directory"
	case string:
		lines := strings.Split(d, "\n")
		if cmd.Start == 0 {
			cmd.Start = 1
		}
		if cmd.End == 0 {
			cmd.End = len(lines)
		}
		if cmd.End < cmd.Start {
			return fmt.Sprintf("read_file: cannot read lines %d-%d: end cannot be before start", cmd.Start, cmd.End)
		}
		if cmd.Start < 1 {
			return fmt.Sprintf("read_file: cannot read lines %d-%d: start cannot be before 1", cmd.Start, cmd.End)
		}
		if cmd.End > len(lines) {
			return fmt.Sprintf("read_file: cannot read lines %d-%d: end cannot be after the end of the file", cmd.Start, cmd.End)
		}

		cmd.Start--
		data := lines[cmd.Start:cmd.End]
		digits := len(fmt.Sprintf("%d", cmd.End))
		formatter := fmt.Sprintf("%%%dd| %%s", digits)
		for i, x := range data {
			data[i] = fmt.Sprintf(formatter, i+cmd.Start+1, x)
		}
		return "```read_file:\n" + strings.Join(data, "\n") + "\n```"
	}
	return "read_file: ERROR ERROR ERROR ERROR ERROR"
}

var root = directory{
	entries: make(map[string]any),
}

func phaseExecuteTool(s string) *agent.Phase {
	var tool ToolCall
	err := xml.Unmarshal([]byte("<tool>"+s+"</tool>"), &tool)
	if err != nil {
		return phaseInvalidTool("Error: failed to execute tool: " + err.Error())
	}

	total := len(tool.AskUser) + len(tool.Mkdir) + len(tool.Ls) + len(tool.WriteFile) + len(tool.ReadFile) + len(tool.Rm)
	if total != 1 {
		return phaseInvalidTool(fmt.Sprintf("Error: expected 1 tool call, got: %d", total))
	}
	if len(tool.AskUser) > 0 {
		return getPhaseUserInput(string(tool.AskUser[0].Prompt))
	}
	if len(tool.Mkdir) > 0 {
		return phaseToolResponse(root.mkdir(tool.Mkdir[0].File))
	}
	if len(tool.Ls) > 0 {
		return phaseToolResponse(root.ls(tool.Ls[0].File))
	}
	if len(tool.WriteFile) > 0 {
		return phaseToolResponse(root.write(tool.WriteFile[0]))
	}
	if len(tool.ReadFile) > 0 {
		return phaseToolResponse(root.read(tool.ReadFile[0]))
	}
	if len(tool.Rm) > 0 {
		return phaseToolResponse(root.rm(tool.Rm[0].File))
	}
	return phaseInvalidTool(fmt.Sprintf("Error: expected 1 tool call, got: %d", total))
}

func NewArchitect() agent.Agent {
	getPhaseUserInput = phaseUserInput
	getPhaseExecuteTool = phaseExecuteTool
	return agent.NewAgent(&systemPrompt)
}
