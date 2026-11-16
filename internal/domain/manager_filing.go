package domain

import (
	"sort"
	"strings"
)

// The board manager files repair tickets on its own. On 2026-11-16 it filed a
// CI fix that an open pull request already made, and the two competed for the
// same runner. A manager-filed ticket therefore names the files it expects to
// change, and the board refuses it when an open ticket already names one of
// them; the manager comments on that ticket instead.

// MaxDeclaredFiles bounds the files one manager-filed ticket may name.
const MaxDeclaredFiles = 40

// filesHeading starts the list the board appends to a manager-filed spec, so
// later filings see those files too.
const filesHeading = "Files expected to change:"

// NamedFiles returns every repo path a text names, including CI, docs and
// build files, which design overlap ignores but duplicates still share.
func NamedFiles(text string) map[string]bool {
	files := map[string]bool{}
	for _, match := range designPath.FindAllStringSubmatch(text, -1) {
		file := strings.TrimPrefix(match[1], "./")
		if strings.Contains(file, "..") {
			continue
		}
		files[file] = true
	}
	return files
}

// NormalizeDeclaredFiles checks the files a manager-filed ticket names and
// returns them sorted without duplicates.
func NormalizeDeclaredFiles(files []string) ([]string, error) {
	if len(files) == 0 {
		return nil, Invalidf("files: the board manager must name the repo paths the ticket expects to change, so duplicates of open work are refused")
	}
	if len(files) > MaxDeclaredFiles {
		return nil, Invalidf("files: at most %d paths", MaxDeclaredFiles)
	}
	seen := map[string]bool{}
	var out []string
	for _, file := range files {
		file = strings.TrimPrefix(strings.TrimSpace(file), "./")
		if !NamedFiles(" " + file + " ")[file] {
			return nil, Invalidf("files: %q is not a repo-relative path such as src/app/Main.cpp", file)
		}
		if !seen[file] {
			seen[file] = true
			out = append(out, file)
		}
	}
	sort.Strings(out)
	return out, nil
}

// ManagerFilingOverlap returns the files an open ticket's spec or design
// names that the new filing also declares.
func ManagerFilingOverlap(declared []string, open Ticket) []string {
	named := NamedFiles(open.Spec + "\n" + open.Design)
	var shared []string
	for _, file := range declared {
		if named[file] {
			shared = append(shared, file)
		}
	}
	return shared
}

// WithDeclaredFiles appends the declared files to a spec.
func WithDeclaredFiles(spec string, files []string) string {
	if len(files) == 0 {
		return spec
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(spec, "\n"))
	b.WriteString("\n\n" + filesHeading + "\n")
	for _, file := range files {
		b.WriteString("- `" + file + "`\n")
	}
	return b.String()
}
