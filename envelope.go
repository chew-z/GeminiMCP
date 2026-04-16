package main

import (
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// xmlAttr returns an XML-attribute-safe rendering of v. It escapes the five
// characters that are unsafe inside a double-quoted attribute value: ampersand,
// less-than, greater-than, and the double quote itself.
func xmlAttr(v string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return r.Replace(v)
}

// cdataWrap wraps v in a CDATA section. Any embedded "]]>" sequence is split
// across two CDATA sections so attacker-controlled content cannot close the
// section early.
func cdataWrap(v string) string {
	safe := strings.ReplaceAll(v, "]]>", "]]]]><![CDATA[>")
	return "<![CDATA[" + safe + "]]>"
}

// boolStr renders a Go bool as the literal "true" / "false" for an XML
// attribute value.
func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func partText(s string) *genai.Part { return genai.NewPartFromText(s) }

// wrapUserTurnWithContext builds the Parts for a request that has at least one
// context block. contextParts and fileParts are ALREADY rendered in XML form by
// the gatherers / file-handling code.
func wrapUserTurnWithContext(
	repo string,
	contextParts []*genai.Part,
	fileParts []*genai.Part,
	query string,
	warnings []string,
	finalInstruction string,
) []*genai.Part {
	parts := make([]*genai.Part, 0, len(contextParts)+len(fileParts)+7)
	parts = append(parts, partText(fmt.Sprintf("<context repo=\"%s\">\n", xmlAttr(repo))))
	parts = append(parts, contextParts...)
	parts = append(parts, fileParts...)
	parts = append(parts, partText("</context>\n\n"))
	parts = append(parts, partText("USING THE CONTEXT PROVIDED ABOVE, YOUR TASK IS:\n\n"))
	parts = append(parts, partText("<task>\n  <query>"+cdataWrap(query)+"</query>\n"))
	if len(warnings) > 0 {
		parts = append(parts, partText(renderUnloadedContext(warnings)))
	}
	parts = append(parts, partText("</task>\n\n"))
	parts = append(parts, partText("<final_instruction>\n"+finalInstruction+"\n</final_instruction>\n"))
	return parts
}

// wrapUserTurnQueryOnly builds the Parts for a request with no context.
func wrapUserTurnQueryOnly(query string, finalInstruction string) []*genai.Part {
	return []*genai.Part{
		partText("<task>\n  <query>" + cdataWrap(query) + "</query>\n</task>\n\n"),
		partText("<final_instruction>\n" + finalInstruction + "\n</final_instruction>\n"),
	}
}

// renderUnloadedContext emits the <unloaded_context> element listing items
// that could not be fetched. The list is capped at maxReportedWarnings.
func renderUnloadedContext(warnings []string) string {
	if len(warnings) > maxReportedWarnings {
		warnings = warnings[:maxReportedWarnings]
	}
	var b strings.Builder
	b.WriteString("  <unloaded_context>\n")
	for _, w := range warnings {
		fmt.Fprintf(&b, "    <item>%s</item>\n", cdataWrap(w))
	}
	b.WriteString("  </unloaded_context>\n")
	return b.String()
}
