package utils

import (
	"regexp"
	"strings"
)

var linkHeaderPattern = regexp.MustCompile(`<(.*)>; +rel="(\w+)"`)

func ParseLinkHeader(raw string) map[string]string {
	output := map[string]string{}

	chunks := strings.SplitSeq(raw, ",")
	for c := range chunks {
		c = strings.TrimSpace(c)
		matches := linkHeaderPattern.FindStringSubmatch(c)
		if matches != nil {
			output[matches[2]] = matches[1]
		}
	}

	return output
}
