package main

// The <final_instruction> bodies are defined as named constants so each stays
// within the project's line-length limit and can be referenced from tests.
const (
	finalInstructionGeneral = "Answer directly and concisely. Use code blocks when showing examples. " +
		"Say so explicitly if you are uncertain."

	finalInstructionAnalyze = "Explain the code's purpose, structure, and key design decisions. " +
		"When describing specific behavior, reference the exact `file:line`."

	finalInstructionReview = "List findings ordered by severity (Critical → Major → Minor → Nit). " +
		"For each finding cite `file:line` and propose a corrected snippet."

	finalInstructionSecurity = "Scan for OWASP Top 10 vulnerabilities. For each finding return severity " +
		"(Critical/High/Medium/Low/Informational), `file:line`, impact, and a remediation with corrected code."

	finalInstructionDebug = "State the root-cause hypothesis, trace the failing execution path, " +
		"and propose a concrete fix with corrected code. Reference `file:line` for every claim."

	finalInstructionTests = "Generate runnable tests covering happy path, edges, and error conditions. " +
		"Use the language's idiomatic patterns (table-driven in Go, parametrize in pytest, describe/it in JS). " +
		"Reference the function under test by `file:symbol`."

	finalInstructionSearch = "Synthesize across sources, cite inline, and distinguish established facts " +
		"from recent developments. If results are insufficient, say so."
)

// finalInstructionByCategory maps each query category produced by the
// pre-qualifier to the <final_instruction> body injected at the end of the
// user-turn envelope. Search has its own hard-coded instruction because it
// never routes through the pre-qualifier.
var finalInstructionByCategory = map[queryCategory]string{
	categoryGeneral:  finalInstructionGeneral,
	categoryAnalyze:  finalInstructionAnalyze,
	categoryReview:   finalInstructionReview,
	categorySecurity: finalInstructionSecurity,
	categoryDebug:    finalInstructionDebug,
	categoryTests:    finalInstructionTests,
}

// finalInstructionFor returns the <final_instruction> body for cat, falling
// back to the general instruction if the category is unknown or empty.
func finalInstructionFor(cat queryCategory) string {
	if s, ok := finalInstructionByCategory[cat]; ok {
		return s
	}
	return finalInstructionByCategory[categoryGeneral]
}
