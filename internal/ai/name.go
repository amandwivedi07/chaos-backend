// The Name call: turn an opening line into a title and one emoji.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

const nameSystemPrompt = `Name a group conversation from its opening line.

Return ONLY this JSON object: {"name": "...", "emoji": "..."}
- "name": under 34 characters, how a friend would refer to the thread later —
  "Bangkok in September?", "Dinner Saturday", "Rohan's birthday gift".
  Keep the question mark if the opener was a question. No quotes, no title case.
- "emoji": exactly one, concrete to the subject.`

func (c *azureClient) Name(ctx context.Context, purpose string) (*Name, error) {
	content, err := c.chat(ctx, nameSystemPrompt, purpose, 120)
	if err != nil {
		return nil, err
	}
	var out Name
	if err := json.Unmarshal([]byte(unfence(content)), &out); err != nil {
		return nil, err
	}
	out.Name = strings.TrimSpace(strings.Trim(out.Name, `"`))
	if out.Name == "" {
		return nil, errors.New("no name returned")
	}
	if len([]rune(out.Name)) > 40 {
		out.Name = string([]rune(out.Name)[:40])
	}
	out.Emoji = firstRune(strings.TrimSpace(out.Emoji))
	if out.Emoji == "" {
		out.Emoji = "✦"
	}
	return &out, nil
}

func firstRune(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}
