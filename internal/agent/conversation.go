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

// TokenEstimate returns a rough token count using the chars/4 heuristic.
func (c *Conversation) TokenEstimate() int {
	total := 0
	for _, msg := range c.Messages {
		switch content := msg.Content.(type) {
		case string:
			total += len(content) / 4
		case []api.ContentBlock:
			for _, b := range content {
				total += (len(b.Text) + len(b.Content) + len(b.Thinking) + len(b.Input)) / 4
			}
		}
	}
	return total
}

// TrimForWindow removes the oldest non-essential messages until the estimated
// token count fits within budgetTokens. Always preserves the first 2 messages
// (initial context) and the last 6 messages (recent context).
// Returns the number of messages dropped.
func (c *Conversation) TrimForWindow(budgetTokens int) int {
	const minHead = 2
	const minTail = 6
	dropped := 0
	for c.TokenEstimate() > budgetTokens && len(c.Messages) > minHead+minTail {
		c.dropAt(minHead)
		dropped++
		// Never leave an orphaned tool_result as the next survivor: a
		// tool_result whose tool_use was just dropped is an API error.
		for len(c.Messages) > minHead+minTail && startsWithToolResult(c.Messages[minHead]) {
			c.dropAt(minHead)
			dropped++
		}
	}
	return dropped
}

func (c *Conversation) dropAt(i int) {
	copy(c.Messages[i:], c.Messages[i+1:])
	c.Messages = c.Messages[:len(c.Messages)-1]
}

func startsWithToolResult(msg api.Message) bool {
	blocks, ok := msg.Content.([]api.ContentBlock)
	return ok && len(blocks) > 0 && blocks[0].Type == "tool_result"
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
