package domain

import (
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NormalizeRepoOverlapIgnore validates complete, slash-separated path patterns.
func NormalizeRepoOverlapIgnore(patterns []string) ([]string, error) {
	if len(patterns) > 100 {
		return nil, Invalidf("overlap_ignore allows at most 100 entries")
	}
	result := make([]string, 0, len(patterns))
	for _, raw := range patterns {
		pattern := strings.TrimSpace(raw)
		driveQualified := len(pattern) >= 2 && pattern[1] == ':' &&
			((pattern[0] >= 'a' && pattern[0] <= 'z') || (pattern[0] >= 'A' && pattern[0] <= 'Z'))
		if pattern == "" || !utf8.ValidString(pattern) || utf8.RuneCountInString(pattern) > 200 || strings.Contains(pattern, "..") || strings.HasPrefix(pattern, "/") || driveQualified {
			return nil, Invalidf("overlap_ignore requires relative path patterns of 1 to 200 characters without '..'")
		}
		for _, r := range pattern {
			if unicode.IsControl(r) {
				return nil, Invalidf("overlap_ignore cannot contain control characters")
			}
		}
		for _, segment := range strings.Split(pattern, "/") {
			if segment == "" || segment == "." {
				return nil, Invalidf("overlap_ignore cannot contain empty or dot segments")
			}
			if _, err := path.Match(segment, ""); err != nil {
				return nil, Invalidf("invalid overlap_ignore glob: %v", err)
			}
		}
		result = append(result, pattern)
	}
	return result, nil
}

func matchesOverlapIgnore(file string, patterns []string) bool {
	segments := strings.Split(file, "/")
	for _, pattern := range patterns {
		parts := strings.Split(pattern, "/")
		if len(parts) != len(segments) {
			continue
		}
		matches := true
		for i, part := range parts {
			ok, err := path.Match(part, segments[i])
			if err != nil || !ok {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}
