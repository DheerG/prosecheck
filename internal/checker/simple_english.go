package checker

import (
	"regexp"
	"strings"
)

type simpleEnglishIssue struct {
	Code       string
	Message    string
	Suggestion string
	Line       int
}

type simpleEnglishPattern struct {
	pattern     *regexp.Regexp
	replacement string
}

var contractions = []simpleEnglishPattern{
	{regexp.MustCompile(`(?i)\baren't\b`), "are not"},
	{regexp.MustCompile(`(?i)\bcan't\b`), "cannot"},
	{regexp.MustCompile(`(?i)\bcouldn't\b`), "could not"},
	{regexp.MustCompile(`(?i)\bdidn't\b`), "did not"},
	{regexp.MustCompile(`(?i)\bdoesn't\b`), "does not"},
	{regexp.MustCompile(`(?i)\bdon't\b`), "do not"},
	{regexp.MustCompile(`(?i)\bhasn't\b`), "has not"},
	{regexp.MustCompile(`(?i)\bhaven't\b`), "have not"},
	{regexp.MustCompile(`(?i)\bisn't\b`), "is not"},
	{regexp.MustCompile(`(?i)\bshouldn't\b`), "should not"},
	{regexp.MustCompile(`(?i)\bwasn't\b`), "was not"},
	{regexp.MustCompile(`(?i)\bweren't\b`), "were not"},
	{regexp.MustCompile(`(?i)\bwon't\b`), "will not"},
	{regexp.MustCompile(`(?i)\bwouldn't\b`), "would not"},
	{regexp.MustCompile(`(?i)\b(?:i'm|you're|we're|they're)\b`), "write both words"},
	{regexp.MustCompile(`(?i)\b(?:i've|you've|we've|they've)\b`), "write both words"},
	{regexp.MustCompile(`(?i)\b(?:i'll|you'll|we'll|they'll)\b`), "write both words"},
	{regexp.MustCompile(`(?i)\b(?:i'd|you'd|we'd|they'd)\b`), "write both words"},
	{regexp.MustCompile(`(?i)\b(?:it's|that's|there's|what's|who's)\b`), "write both words"},
}

var complexPhrases = []simpleEnglishPattern{
	{regexp.MustCompile(`(?i)\bin order to\b`), "to"},
	{regexp.MustCompile(`(?i)\bprior to\b`), "before"},
	{regexp.MustCompile(`(?i)\bdue to the fact that\b`), "because"},
	{regexp.MustCompile(`(?i)\bin the event that\b`), "if"},
	{regexp.MustCompile(`(?i)\bwhen it comes to\b`), "for"},
	{regexp.MustCompile(`(?i)\b(?:leverage|utilize)\b`), "use"},
	{regexp.MustCompile(`(?i)\bfunctionality\b`), "function or feature"},
	{regexp.MustCompile(`(?i)\bout of the box\b`), "by default"},
	{regexp.MustCompile(`(?i)\bunder the hood\b`), "internally"},
	{regexp.MustCompile(`(?i)\b(?:enables|allows) you to\b`), "you can"},
	{regexp.MustCompile(`(?i)\b(?:is designed to|aims to)\b`), "state what the change does"},
	{regexp.MustCompile(`(?i)\bit is worth noting that\b`), "remove this phrase"},
	{regexp.MustCompile(`(?i)\bit is important to\b`), "state the fact or requirement"},
	{regexp.MustCompile(`(?i)\bthis pr aims to\b`), "state the outcome"},
	{regexp.MustCompile(`(?i)\bgracefully handles?\b`), "state the exact behavior"},
	{regexp.MustCompile(`(?i)\b(?:as needed|as necessary)\b`), "state the condition"},
}

var (
	ambiguousModal = regexp.MustCompile(`(?i)\b(?:should|would|may|might|could)\b`)
	latinForm      = regexp.MustCompile(`(?i)(?:\be\.g\.|\bi\.e\.|\betc\.)`)
	complexTense   = regexp.MustCompile(`(?i)\b(?:has|have) been\b`)
)

func simpleEnglishIssues(message Message, allowed []string) []simpleEnglishIssue {
	lines := []BodyLine{{Number: message.SubjectLine, Text: message.Subject}}
	lines = append(lines, message.BodyLines...)
	issues := make([]simpleEnglishIssue, 0)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line.Text)
		if trimmed == "" || trailerPattern.MatchString(trimmed) {
			continue
		}
		plain := plainEnglishText(trimmed, allowed)
		if plain == "" {
			continue
		}

		if matched, replacement := firstPatternMatch(plain, contractions); matched != "" {
			suggestion := "Write the contraction in full."
			if replacement != "write both words" {
				suggestion = "Use “" + replacement + "” instead of “" + matched + "”."
			}
			issues = append(issues, simpleEnglishIssue{
				Code: "PC015", Message: "The text uses the contraction “" + matched + "”.",
				Suggestion: suggestion, Line: line.Number,
			})
		}
		if matched, replacement := firstPatternMatch(plain, complexPhrases); matched != "" {
			issues = append(issues, simpleEnglishIssue{
				Code: "PC016", Message: "The text uses the complex phrase “" + matched + "”.",
				Suggestion: "Use “" + replacement + "”.", Line: line.Number,
			})
		}
		if matched := ambiguousModal.FindString(plain); matched != "" {
			issues = append(issues, simpleEnglishIssue{
				Code: "PC017", Message: "The word “" + matched + "” can make the meaning uncertain.",
				Suggestion: modalSuggestion(matched), Line: line.Number,
			})
		}
		if matched := latinForm.FindString(plain); matched != "" {
			issues = append(issues, simpleEnglishIssue{
				Code: "PC018", Message: "The text uses the abbreviation “" + matched + "”.",
				Suggestion: latinSuggestion(matched), Line: line.Number,
			})
		}
		if matched := complexTense.FindString(plain); matched != "" {
			issues = append(issues, simpleEnglishIssue{
				Code: "PC019", Message: "The phrase “" + matched + "” uses a complex verb tense.",
				Suggestion: "Use the simple present or simple past tense.", Line: line.Number,
			})
		}
	}
	return issues
}

func firstPatternMatch(text string, patterns []simpleEnglishPattern) (string, string) {
	for _, candidate := range patterns {
		if matched := candidate.pattern.FindString(text); matched != "" {
			return matched, candidate.replacement
		}
	}
	return "", ""
}

func modalSuggestion(modal string) string {
	switch strings.ToLower(modal) {
	case "may", "might", "could":
		return "Use “can” for a possibility, or state the exact condition."
	case "should":
		return "Use “must” for a requirement, or state the fact directly."
	default:
		return "State the condition and its result directly."
	}
}

func latinSuggestion(value string) string {
	switch strings.ToLower(value) {
	case "e.g.":
		return "Use “for example”."
	case "i.e.":
		return "Use “that is”."
	default:
		return "Name the items, or remove the abbreviation."
	}
}

func plainEnglishText(text string, allowed []string) string {
	var builder strings.Builder
	inBackticks := false
	inQuotes := false
	for _, character := range text {
		switch character {
		case '`':
			inBackticks = !inBackticks
			builder.WriteRune(' ')
		case '"', '“', '”':
			inQuotes = !inQuotes
			builder.WriteRune(' ')
		default:
			if inBackticks || inQuotes {
				builder.WriteRune(' ')
			} else {
				builder.WriteRune(character)
			}
		}
	}

	plain := builder.String()
	for _, term := range allowed {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		plain = replaceFold(plain, term)
	}
	return strings.TrimSpace(plain)
}

func replaceFold(text, target string) string {
	lowerText := strings.ToLower(text)
	lowerTarget := strings.ToLower(target)
	for {
		index := strings.Index(lowerText, lowerTarget)
		if index < 0 {
			return text
		}
		spaces := strings.Repeat(" ", len(target))
		text = text[:index] + spaces + text[index+len(target):]
		lowerText = lowerText[:index] + spaces + lowerText[index+len(target):]
	}
}
