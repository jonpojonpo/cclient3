package agent

import (
	"encoding/json"
	"os"

	"github.com/jonpo/cclient3/internal/api"
)

// Conversation manages message history.
type Conversation struct {
	Messages []api.Message
}

func NewConversation() *Conversation {
	return &Conversation{}
}

func (c *Conversation) AddUser(content string) {
	c.Messages = append(c.Messages, api.Message{
		Role:    "user",
		Content: content,
	})
}

func (c *Conversation) AddAssistant(blocks []api.ContentBlock) {
	c.Messages = append(c.Messages, api.Message{
		Role:    "assistant",
		Content: blocks,
	})
}

func (c *Conversation) AddToolResults(results []api.ContentBlock) {
	c.Messages = append(c.Messages, api.Message{
		Role:    "user",
		Content: results,
	})
}

func (c *Conversation) Clear() {
	c.Messages = nil
}

func (c *Conversation) Save(path string) error {
	data, err := json.MarshalIndent(c.Messages, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (c *Conversation) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &c.Messages)
}
