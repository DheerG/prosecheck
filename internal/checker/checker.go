package checker

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/DheerG/prosecheck/internal/config"
)

var (
	conventionalPrefix = regexp.MustCompile(`^(?i:(?:build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)(?:\([^)]+\))?!?:\s*)`)
	listPrefix         = regexp.MustCompile(`^(?:[-*+]\s+|\d+[.)]\s+)`)
	spacePattern       = regexp.MustCompile(`\s+`)
	trailerPattern     = regexp.MustCompile(`^[A-Za-z][A-Za-z-]+:\s+\S`)
)

var placeholderSubjects = map[string]bool{
	"asdf": true, "checkpoint": true, "changes": true, "fix": true, "fixes": true,
	"misc": true, "stuff": true, "temp": true, "temporary": true, "test": true,
	"tmp": true, "update": true, "updates": true, "wip": true,
}

var pastTenseOpeners = map[string]string{
	"added": "Add", "adjusted": "Adjust", "changed": "Change", "cleaned": "Clean",
	"created": "Create", "fixed": "Fix", "implemented": "Implement", "improved": "Improve",
	"moved": "Move", "refactored": "Refactor", "removed": "Remove", "renamed": "Rename",
	"replaced": "Replace", "updated": "Update",
}

func Check(raw string, cfg config.Config) Report {
	message := Parse(raw)
	report := Report{Subject: message.Subject, Findings: []Finding{}}
	add := func(code string, defaultSeverity Severity, text, suggestion string, line int) {
		severity, enabled := configuredSeverity(code, defaultSeverity, cfg.Rules)
		if !enabled {
			return
		}
		report.Findings = append(report.Findings, Finding{
			Code: code, Severity: severity, Source: SourceRule, Message: text, Suggestion: suggestion, Line: line,
		})
	}

	if message.Subject == "" {
		add("PC001", SeverityError, "The commit subject is empty.", "Write one line that states the durable outcome.", 1)
		return report
	}
	if isGeneratedSubject(message.Subject) {
		return report
	}

	if length := len([]rune(message.Subject)); length > cfg.Subject.MaxLength {
		add("PC002", SeverityWarning,
			"The commit subject is longer than the configured limit.",
			"Keep the subject at no more than "+itoa(cfg.Subject.MaxLength)+" characters.", message.SubjectLine)
	}

	meaningfulSubject := stripSubjectPrefix(message.Subject)
	if length := len([]rune(meaningfulSubject)); length < cfg.Subject.MinLength {
		add("PC003", SeverityWarning, "The commit subject gives too little information.",
			"State the result of the change, not only the type of work.", message.SubjectLine)
	}
	if strings.HasSuffix(message.Subject, ".") {
		add("PC004", SeverityWarning, "The commit subject ends with a period.",
			"Remove the final period from the subject.", message.SubjectLine)
	}

	normalizedSubject := normalizeWords(meaningfulSubject)
	if placeholderSubjects[normalizedSubject] {
		add("PC005", SeverityError, "The commit subject is a temporary label.",
			"Replace it with the outcome that a reader must remember.", message.SubjectLine)
	} else if isVagueSubject(normalizedSubject) {
		add("PC006", SeverityWarning, "The commit subject names work but not its outcome.",
			"State what changed for the system or its users.", message.SubjectLine)
	}

	firstWord := firstWord(normalizedSubject)
	if imperative, ok := pastTenseOpeners[firstWord]; ok {
		add("PC007", SeverityWarning, "The commit subject starts in the past tense.",
			"Use the imperative form: “"+imperative+"”.", message.SubjectLine)
	}

	if message.MissingSeparator {
		add("PC008", SeverityWarning, "The subject and body do not have a blank line between them.",
			"Add one blank line after the subject.", message.SubjectLine+1)
	}

	firstBodyLine, firstBodyLineNumber := firstNonBlankLine(message.BodyLines)
	if firstBodyLine != "" {
		lower := strings.ToLower(strings.TrimSpace(firstBodyLine))
		if listPrefix.MatchString(lower) || startsWithDetailHeading(lower) {
			add("PC009", SeverityWarning, "The body starts with implementation details.",
				"Start with the outcome and its reason. Put technical details after that context.", firstBodyLineNumber)
		}
		if startsWithProcessNarration(lower) {
			add("PC010", SeverityWarning, "The body starts by narrating the commit process.",
				"State why the change matters. The Git record already shows that a commit made the change.", firstBodyLineNumber)
		}
	}

	for _, line := range message.BodyLines {
		trimmed := strings.TrimSpace(line.Text)
		if trimmed == "" || trailerPattern.MatchString(trimmed) {
			continue
		}
		if len([]rune(line.Text)) > cfg.Body.MaxLineLength {
			add("PC011", SeverityWarning, "A body line is longer than the configured limit.",
				"Wrap the line at no more than "+itoa(cfg.Body.MaxLineLength)+" characters.", line.Number)
		}
		if strings.Contains(line.Text, ";") {
			add("PC014", SeverityInfo, "A body line contains a semicolon.",
				"Use two sentences when the line contains two separate facts.", line.Number)
		}
	}

	for _, sentence := range bodySentences(message.BodyLines) {
		if sentence.WordCount > cfg.Body.MaxSentenceWords {
			add("PC012", SeverityWarning, "A body sentence contains too many words.",
				"Split the sentence. Keep each body sentence at no more than "+itoa(cfg.Body.MaxSentenceWords)+" words.", sentence.Line)
		}
	}

	firstParagraph, paragraphLine := firstParagraph(message.BodyLines)
	if firstParagraph != "" && normalizeWords(firstParagraph) == normalizeWords(meaningfulSubject) {
		add("PC013", SeverityInfo, "The first body paragraph repeats the subject.",
			"Use the body for the reason, constraints, or context that the subject cannot contain.", paragraphLine)
	}

	SortFindings(report.Findings)
	return report
}

func configuredSeverity(code string, fallback Severity, overrides map[string]string) (Severity, bool) {
	value, ok := overrides[code]
	if !ok || value == "" {
		return fallback, true
	}
	switch strings.ToLower(value) {
	case "off":
		return "", false
	case "error":
		return SeverityError, true
	case "warning":
		return SeverityWarning, true
	case "info":
		return SeverityInfo, true
	default:
		return fallback, true
	}
}

func isGeneratedSubject(subject string) bool {
	lower := strings.ToLower(subject)
	return strings.HasPrefix(lower, "merge ") || strings.HasPrefix(lower, "revert \"") ||
		strings.HasPrefix(lower, "fixup! ") || strings.HasPrefix(lower, "squash! ") ||
		strings.HasPrefix(lower, "amend! ")
}

func stripSubjectPrefix(subject string) string {
	return strings.TrimSpace(conventionalPrefix.ReplaceAllString(subject, ""))
}

func normalizeWords(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimRight(value, ".!?")
	return spacePattern.ReplaceAllString(value, " ")
}

func isVagueSubject(subject string) bool {
	fields := strings.Fields(subject)
	if len(fields) == 0 {
		return false
	}
	vagueNouns := map[string]bool{
		"code": true, "file": true, "files": true, "misc": true, "readme": true,
		"readme.md": true, "stuff": true, "test": true, "tests": true, "thing": true, "things": true,
	}
	vagueVerbs := map[string]bool{
		"adjust": true, "adjusted": true, "change": true, "changed": true, "changes": true,
		"fix": true, "fixed": true, "fixes": true, "modify": true, "modified": true,
		"tweak": true, "tweaked": true, "update": true, "updated": true, "updates": true,
	}
	return len(fields) <= 3 && vagueVerbs[fields[0]] && vagueNouns[fields[len(fields)-1]]
}

func firstWord(subject string) string {
	fields := strings.FieldsFunc(subject, func(r rune) bool { return !unicode.IsLetter(r) })
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(fields[0])
}

func firstNonBlankLine(lines []BodyLine) (string, int) {
	for _, line := range lines {
		if strings.TrimSpace(line.Text) != "" {
			return line.Text, line.Number
		}
	}
	return "", 0
}

func startsWithDetailHeading(line string) bool {
	prefixes := []string{
		"changes made", "files changed", "implementation detail", "implementation details",
		"technical detail", "technical details", "technical issue", "technical issues",
		"technical issues identified", "what changed",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(line, prefix+":") || line == prefix {
			return true
		}
	}
	return false
}

func startsWithProcessNarration(line string) bool {
	prefixes := []string{
		"in this commit,", "in this commit ", "this commit adds", "this commit changes",
		"this commit fixes", "this commit implements", "this commit modifies", "this commit removes",
		"this commit updates", "this pr adds", "this pr changes", "this pr fixes", "this pr updates",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func firstParagraph(lines []BodyLine) (string, int) {
	parts := make([]string, 0)
	lineNumber := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line.Text)
		if trimmed == "" {
			if len(parts) > 0 {
				break
			}
			continue
		}
		if lineNumber == 0 {
			lineNumber = line.Number
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, " "), lineNumber
}

type sentenceInfo struct {
	Line      int
	WordCount int
}

func bodySentences(lines []BodyLine) []sentenceInfo {
	var result []sentenceInfo
	wordCount := 0
	startLine := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line.Text)
		if trimmed == "" || trailerPattern.MatchString(trimmed) || listPrefix.MatchString(trimmed) {
			if wordCount > 0 {
				result = append(result, sentenceInfo{Line: startLine, WordCount: wordCount})
				wordCount, startLine = 0, 0
			}
			continue
		}
		for _, token := range strings.Fields(trimmed) {
			if startLine == 0 {
				startLine = line.Number
			}
			wordCount++
			if endsSentence(token) {
				result = append(result, sentenceInfo{Line: startLine, WordCount: wordCount})
				wordCount, startLine = 0, 0
			}
		}
	}
	if wordCount > 0 {
		result = append(result, sentenceInfo{Line: startLine, WordCount: wordCount})
	}
	return result
}

func endsSentence(token string) bool {
	trimmed := strings.TrimRight(token, "\"')]}>")
	return strings.HasSuffix(trimmed, ".") || strings.HasSuffix(trimmed, "!") || strings.HasSuffix(trimmed, "?")
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	for value > 0 {
		digits = append(digits, byte('0'+value%10))
		value /= 10
	}
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
