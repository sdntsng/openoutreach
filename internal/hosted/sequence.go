package hosted

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/andersmyrmel/cold-cli/internal"
)

// SequenceStepView is the dashboard/MCP view of one email step.
type SequenceStepView struct {
	Step    int    `json:"step"`
	Delay   int    `json:"delay"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// SequenceView is the visual-editor shape over the same YAML the engine parses.
type SequenceView struct {
	Name     string             `json:"name"`
	FromName string             `json:"from_name"`
	Steps    []SequenceStepView `json:"steps"`
	YAML     string             `json:"yaml"`
}

// RenderedStep is one ReplaceAll-rendered email for a recipient.
type RenderedStep struct {
	Step     int    `json:"step"`
	Delay    int    `json:"delay"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	SendAt   string `json:"send_at,omitempty"`
	SendNote string `json:"send_note,omitempty"`
}

var samplePreviewFields = map[string]string{
	"first_name": "Ada",
	"last_name":  "Lovelace",
	"company":    "Acme",
	"email":      "ada@acme.com",
	"domain":     "acme.com",
}

func sequenceViewFrom(seq *internal.Sequence, yamlOut string) SequenceView {
	if seq == nil {
		return SequenceView{Name: "outreach", FromName: "You", Steps: []SequenceStepView{}, YAML: yamlOut}
	}
	from := strings.TrimSpace(seq.Defaults.FromName)
	if from == "" {
		from = "You"
	}
	name := strings.TrimSpace(seq.Name)
	if name == "" {
		name = "outreach"
	}
	steps := make([]SequenceStepView, 0, len(seq.Steps))
	for i, st := range seq.Steps {
		n := st.Step
		if n == 0 {
			n = i + 1
		}
		steps = append(steps, SequenceStepView{
			Step: n, Delay: st.Delay, Subject: st.Subject, Body: st.Body,
		})
	}
	if yamlOut == "" {
		yamlOut = SequenceToYAML(seq)
	}
	return SequenceView{Name: name, FromName: from, Steps: steps, YAML: yamlOut}
}

func sequenceFromView(v SequenceView) *internal.Sequence {
	name := strings.TrimSpace(v.Name)
	if name == "" {
		name = "outreach"
	}
	from := strings.TrimSpace(v.FromName)
	if from == "" {
		from = "You"
	}
	seq := &internal.Sequence{
		Name:     name,
		Defaults: internal.SequenceDefaults{FromName: from},
	}
	for i, st := range v.Steps {
		n := st.Step
		if n == 0 {
			n = i + 1
		}
		seq.Steps = append(seq.Steps, internal.SequenceStep{
			Step: n, Delay: st.Delay, Subject: st.Subject, Body: st.Body,
		})
	}
	return seq
}

// SequenceToYAML writes the same format ParseSequenceFromBytes accepts.
func SequenceToYAML(seq *internal.Sequence) string {
	if seq == nil {
		return ""
	}
	var b strings.Builder
	name := strings.TrimSpace(seq.Name)
	if name == "" {
		name = "outreach"
	}
	from := strings.TrimSpace(seq.Defaults.FromName)
	if from == "" {
		from = "You"
	}
	b.WriteString("name: " + yamlScalar(name) + "\n")
	b.WriteString("defaults:\n")
	b.WriteString("  from_name: " + yamlScalar(from) + "\n")
	b.WriteString("steps:\n")
	for i, st := range seq.Steps {
		n := st.Step
		if n == 0 {
			n = i + 1
		}
		b.WriteString(fmt.Sprintf("  - step: %d\n    delay: %d\n", n, st.Delay))
		if strings.TrimSpace(st.Subject) != "" {
			b.WriteString("    subject: " + yamlScalar(st.Subject) + "\n")
		}
		b.WriteString("    body: |\n")
		body := strings.TrimRight(st.Body, "\n")
		if body == "" {
			b.WriteString("      \n")
			continue
		}
		for _, line := range strings.Split(body, "\n") {
			b.WriteString("      " + line + "\n")
		}
		if len(st.Variants) > 0 {
			b.WriteString("    variants:\n")
			for _, v := range st.Variants {
				b.WriteString("      - subject: " + yamlScalar(v.Subject) + "\n")
				b.WriteString("        body: |\n")
				for _, line := range strings.Split(strings.TrimRight(v.Body, "\n"), "\n") {
					b.WriteString("          " + line + "\n")
				}
			}
		}
	}
	return b.String()
}

func yamlScalar(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return `""`
	}
	need := strings.ContainsAny(s, ":#{}[]&*!|>'\"%@`,") ||
		strings.Contains(s, "\n") ||
		strings.HasPrefix(s, " ") ||
		s == "true" || s == "false" || s == "null"
	if !need {
		return s
	}
	return strconv.Quote(s)
}

func parseSequenceYAML(raw string) (*internal.Sequence, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", fmt.Errorf("sequence is empty")
	}
	seq, err := internal.ParseSequenceFromBytes([]byte(raw))
	if err != nil {
		return nil, raw, err
	}
	return seq, SequenceToYAML(seq), nil
}

func renderSequence(seq *internal.Sequence, fields map[string]string) []RenderedStep {
	if seq == nil {
		return nil
	}
	if fields == nil {
		fields = samplePreviewFields
	}
	out := make([]RenderedStep, 0, len(seq.Steps))
	for i, st := range seq.Steps {
		n := st.Step
		if n == 0 {
			n = i + 1
		}
		subject := internal.RenderTemplate(st.Subject, fields)
		body := internal.RenderTemplate(st.Body, fields)
		if i == 0 && strings.TrimSpace(st.Subject) == "" && len(st.Variants) > 0 {
			subject = internal.RenderTemplate(st.Variants[0].Subject, fields)
			body = internal.RenderTemplate(st.Variants[0].Body, fields)
		}
		note := "During the campaign send window after activate."
		if st.Delay > 0 {
			day := "days"
			if st.Delay == 1 {
				day = "day"
			}
			note = fmt.Sprintf("About %d %s after the previous email, in the same send window.", st.Delay, day)
		}
		out = append(out, RenderedStep{
			Step: n, Delay: st.Delay, Subject: subject, Body: body, SendNote: note,
		})
	}
	return out
}

func mergeLeadFields(email, first, last, company, domain string, extra map[string]string) map[string]string {
	fields := map[string]string{
		"email":      strings.TrimSpace(email),
		"first_name": strings.TrimSpace(first),
		"last_name":  strings.TrimSpace(last),
		"company":    strings.TrimSpace(company),
		"domain":     strings.TrimSpace(domain),
	}
	if fields["domain"] == "" && fields["email"] != "" {
		fields["domain"] = internal.ExtractDomain(fields["email"])
	}
	for k, v := range extra {
		if strings.TrimSpace(v) == "" {
			continue
		}
		fields[k] = v
	}
	return fields
}
