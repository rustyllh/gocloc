package gocloc

import (
	"regexp"
	"strings"
)

func InsertPipesInTheMiddle(input string) string {
	var output strings.Builder
	re := regexp.MustCompile(`\s+`)
	words := strings.Fields(input)
	spaces := re.FindAllString(input, -1)

	for i := 0; i < len(words); i++ {
		if i < len(spaces) {
			middle := (len(spaces[i]) / 2) - 1
			output.WriteString(words[i] + strings.Repeat(" ", middle) + "|" + strings.Repeat(" ", middle))
		} else {
			output.WriteString(words[i] + " |")
		}
	}

	return output.String()
}

func trimBOM(line string) string {
	l := len(line)
	if l >= 3 {
		if line[0] == 0xef && line[1] == 0xbb && line[2] == 0xbf {
			trimLine := line[3:]
			return trimLine
		}
	}
	return line
}

func containsComment(line string, multiLines [][]string) bool {
	for _, comments := range multiLines {
		for _, comm := range comments {
			if strings.Contains(line, comm) {
				return true
			}
		}
	}
	return false
}

func nextRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}
