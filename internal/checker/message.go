package checker

import "strings"

type Message struct {
	Raw              string
	Subject          string
	SubjectLine      int
	Body             string
	BodyLines        []BodyLine
	MissingSeparator bool
}

type BodyLine struct {
	Number int
	Text   string
}

func Parse(raw string) Message {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")

	message := Message{Raw: normalized}
	subjectIndex := -1
	for i, line := range lines {
		if isIgnoredLine(line) || strings.TrimSpace(line) == "" {
			continue
		}
		message.Subject = strings.TrimSpace(line)
		message.SubjectLine = i + 1
		subjectIndex = i
		break
	}
	if subjectIndex < 0 {
		return message
	}

	if subjectIndex+1 < len(lines) && strings.TrimSpace(lines[subjectIndex+1]) != "" && !isIgnoredLine(lines[subjectIndex+1]) {
		message.MissingSeparator = true
	}

	for i := subjectIndex + 1; i < len(lines); i++ {
		if isIgnoredLine(lines[i]) {
			continue
		}
		message.BodyLines = append(message.BodyLines, BodyLine{Number: i + 1, Text: strings.TrimRight(lines[i], " \t")})
	}
	message.BodyLines = trimBlankBodyLines(message.BodyLines)

	body := make([]string, 0, len(message.BodyLines))
	for _, line := range message.BodyLines {
		body = append(body, line.Text)
	}
	message.Body = strings.Join(body, "\n")
	return message
}

func isIgnoredLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "#")
}

func trimBlankBodyLines(lines []BodyLine) []BodyLine {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start].Text) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1].Text) == "" {
		end--
	}
	return lines[start:end]
}
