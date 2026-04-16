package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestXMLAttrEscape(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"plain", "plain"},
		{`a & b`, "a &amp; b"},
		{`<script>`, "&lt;script&gt;"},
		{`"quoted"`, "&quot;quoted&quot;"},
		{`</file>`, "&lt;/file&gt;"},
		{`all: & < > "`, `all: &amp; &lt; &gt; &quot;`},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, xmlAttr(tc.in))
		})
	}
}

func TestWrapUserTurnWithContextShape(t *testing.T) {
	ctxParts := []*genai.Part{
		genai.NewPartFromText("  <commit sha=\"abc\" />\n"),
	}
	fileParts := []*genai.Part{
		genai.NewPartFromText("  <file path=\"README.md\" ref=\"main\" kind=\"text\" mime=\"text/plain\">readme</file>\n"),
	}
	parts := wrapUserTurnWithContext(
		"owner/repo",
		ctxParts,
		fileParts,
		"please summarise",
		nil,
		"FINAL",
	)

	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.Text)
	}
	got := sb.String()

	want := "<context repo=\"owner/repo\">\n" +
		"  <commit sha=\"abc\" />\n" +
		"  <file path=\"README.md\" ref=\"main\" kind=\"text\" mime=\"text/plain\">readme</file>\n" +
		"</context>\n\n" +
		"USING THE CONTEXT PROVIDED ABOVE, YOUR TASK IS:\n\n" +
		"<task>\n  <query>please summarise</query>\n</task>\n\n" +
		"<final_instruction>\nFINAL\n</final_instruction>\n"
	assert.Equal(t, want, got)
}

func TestWrapUserTurnWithContextIncludesUnloadedWhenWarningsPresent(t *testing.T) {
	parts := wrapUserTurnWithContext(
		"o/r",
		nil,
		nil,
		"q",
		[]string{"a: fail", "b: fail"},
		"F",
	)
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.Text)
	}
	got := sb.String()

	assert.Contains(t, got, "<unloaded_context>")
	assert.Contains(t, got, "<item>a: fail</item>")
	assert.Contains(t, got, "<item>b: fail</item>")
	assert.Contains(t, got, "</unloaded_context>")
}

func TestWrapUserTurnQueryOnlyShape(t *testing.T) {
	parts := wrapUserTurnQueryOnly("hello?", "FINAL")
	require.Len(t, parts, 2)
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.Text)
	}
	want := "<task>\n  <query>hello?</query>\n</task>\n\n" +
		"<final_instruction>\nFINAL\n</final_instruction>\n"
	assert.Equal(t, want, sb.String())
}

func TestRenderUnloadedContextCap(t *testing.T) {
	var warnings []string
	for i := 1; i <= maxReportedWarnings+5; i++ {
		warnings = append(warnings, fmt.Sprintf("item-%02d", i))
	}
	got := renderUnloadedContext(warnings)
	for i := 1; i <= maxReportedWarnings; i++ {
		assert.Contains(t, got, fmt.Sprintf("item-%02d", i))
	}
	for i := maxReportedWarnings + 1; i <= maxReportedWarnings+5; i++ {
		assert.NotContains(t, got, fmt.Sprintf("item-%02d", i))
	}
}

func TestBoolStr(t *testing.T) {
	assert.Equal(t, "true", boolStr(true))
	assert.Equal(t, "false", boolStr(false))
}

func TestRenderPartsForDebug_TruncatesLargeBodies(t *testing.T) {
	huge := strings.Repeat("A", debugPartMaxBytes*5)
	small := "short"
	parts := []*genai.Part{
		genai.NewPartFromText(small),
		genai.NewPartFromText(huge),
		nil,
		genai.NewPartFromText(""),
	}

	got := renderPartsForDebug(parts)

	assert.Contains(t, got, small, "short parts must survive intact")
	assert.Contains(t, got, "[truncated", "oversized parts must carry a truncation marker")
	assert.NotContains(t, got, strings.Repeat("A", debugPartMaxBytes+1),
		"oversized parts must not be rendered in full")

	assert.Equal(t, len(small)+len(huge), totalPartBytes(parts),
		"totalPartBytes must reflect pre-truncation sizes")
}

func TestFinalInstructionFor(t *testing.T) {
	// Every declared category must resolve to a non-empty instruction.
	for _, cat := range []queryCategory{
		categoryGeneral, categoryAnalyze, categoryReview,
		categorySecurity, categoryDebug, categoryTests,
	} {
		got := finalInstructionFor(cat)
		assert.NotEmpty(t, got, "category %q has empty final instruction", cat)
	}
	// Unknown category falls back to the general instruction.
	assert.Equal(t, finalInstructionByCategory[categoryGeneral], finalInstructionFor(""))
	assert.Equal(t, finalInstructionByCategory[categoryGeneral], finalInstructionFor("unknown"))
}
