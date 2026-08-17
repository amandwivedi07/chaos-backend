// The Extract call: read a transcript and pull out durable facts.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const extractSystemPrompt = `Pull durable facts about one person out of a transcript.

A durable fact is something still true next month: a constraint, a budget, a
dietary rule, a taste, an availability pattern, a way of deciding. A one-off
logistic is not — "landing at 6pm on Friday" is noise, "never books anything
longer than a 6-hour flight" is a fact.

Return ONLY this JSON object:
{"facts": [{"label":"...","value":"...","category":"...","confidence":0.0}]}
- "label": 1-2 words, the dimension. "Budget ceiling", "Trip length", "Food".
- "value": one short line in the third person. "Keeps trips under 2L per person."
- "category": exactly one of basics, taste, constraints, money, food, style.
- "confidence": 0.5-0.95. How sure you are it is durable, not a mood.
- At most 8 facts. Prefer few and certain over many and vague.
- Skip anything already in the EXISTING list unless you are correcting it —
  in that case reuse its exact label so it replaces the old value.
- Never invent. If the transcript says nothing durable, return {"facts": []}.`

func (c *azureClient) Extract(ctx context.Context, in ExtractInput) ([]Fact, error) {
	var b strings.Builder
	if in.Source != "" {
		fmt.Fprintf(&b, "SOURCE: %s\n\n", in.Source)
	}
	if len(in.Existing) > 0 {
		b.WriteString("EXISTING:\n")
		for _, f := range in.Existing {
			fmt.Fprintf(&b, "- %s: %s\n", f.Label, f.Value)
		}
		b.WriteString("\n")
	}
	b.WriteString("TRANSCRIPT:\n")
	b.WriteString(in.Transcript)

	content, err := c.chat(ctx, extractSystemPrompt, b.String(), 900)
	if err != nil {
		return nil, err
	}
	var out struct {
		Facts []Fact `json:"facts"`
	}
	if err := json.Unmarshal([]byte(unfence(content)), &out); err != nil {
		return nil, err
	}
	return sanitiseFacts(out.Facts), nil
}

var factCategories = map[string]bool{
	"basics": true, "taste": true, "constraints": true,
	"money": true, "food": true, "style": true,
}

func sanitiseFacts(in []Fact) []Fact {
	out := make([]Fact, 0, 8)
	for _, f := range in {
		f.Label = strings.TrimSpace(f.Label)
		f.Value = strings.TrimSpace(f.Value)
		if f.Label == "" || f.Value == "" {
			continue
		}
		f.Category = strings.ToLower(strings.TrimSpace(f.Category))
		if !factCategories[f.Category] {
			f.Category = "basics"
		}
		if f.Confidence <= 0 || f.Confidence > 1 {
			f.Confidence = 0.7
		}
		out = append(out, f)
		if len(out) == 8 {
			break
		}
	}
	return out
}
