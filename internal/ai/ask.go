// The Ask call: answer a question from everything a group has said, across
// every conversation they have had, and say which ones it came from.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const askSystemPrompt = `You are Chaos, answering a question about what a group already worked out.

You are given the group's standing context, and transcripts from several of
their past conversations. The person is asking you to remember for them —
they know the answer is in there somewhere and do not want to scroll.

How to answer:
- Answer from the transcripts. If they settled it, say what they settled and
  when it came up. If they never did, say so plainly — "you never picked a
  date" is a useful answer and inventing one is not.
- Two or three sentences. This is a recall, not a plan.
- Name people when it matters ("Rahul wanted a beach"), because the point is
  usually who wanted what.
- Never mention being an AI, and never describe the transcripts as documents.

Return ONLY this JSON object:
{"text": "...", "references": ["conversation-id", ...]}

- "references": the ids of the conversations you actually used, most relevant
  first, at most 3. Use the exact CONVERSATION id strings given to you. If you
  used none — because the answer is not in there — return an empty list.`

// AskInput is a question put to a whole group.
type AskInput struct {
	GroupName string
	// Memory is the group's standing context, in its own words.
	Memory   []string
	Question string
	// Conversations are the threads to search, most recent first.
	Conversations []ConversationDigest
}

// ConversationDigest is one thread flattened for recall: enough to answer
// from, and an id to cite.
type ConversationDigest struct {
	ID         string
	Title      string
	Transcript string
}

// Ask answers from a group's history and cites the threads it used.
func (c *azureClient) Ask(ctx context.Context, in AskInput) (*Answer, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "GROUP: %s\n\n", in.GroupName)
	if len(in.Memory) > 0 {
		b.WriteString("WHAT THIS GROUP KNOWS:\n")
		for _, m := range in.Memory {
			fmt.Fprintf(&b, "- %s\n", m)
		}
		b.WriteString("\n")
	}
	for _, conv := range in.Conversations {
		fmt.Fprintf(&b, "CONVERSATION id=%s title=%q\n%s\n\n",
			conv.ID, conv.Title, conv.Transcript)
	}
	fmt.Fprintf(&b, "QUESTION: %s", in.Question)

	content, err := c.chat(ctx, askSystemPrompt, b.String(), 900)
	if err != nil {
		return nil, err
	}
	var out Answer
	if err := json.Unmarshal([]byte(unfence(content)), &out); err != nil {
		// A model that ignores the format still answered; showing raw JSON is
		// the one outcome that is never acceptable.
		text := strings.TrimSpace(content)
		if text == "" || strings.HasPrefix(text, "{") {
			return nil, fmt.Errorf("unparseable answer: %w", err)
		}
		return &Answer{Text: text}, nil
	}
	out.Text = strings.TrimSpace(out.Text)
	if out.Text == "" {
		return nil, errors.New("answer had no text")
	}
	if len(out.References) > 3 {
		out.References = out.References[:3]
	}
	return &out, nil
}
