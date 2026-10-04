package mcpsrv

import (
	"regexp"
	"sort"
	"strings"

	"github.com/patiently/anti-tangent-mcp/internal/verdict"
)

// listedPathMark stands in for a listed path while a claim is examined. It is
// a control character, which no claim text and no path contains.
const listedPathMark = "\x00"

// markedAnchor is the pattern for a listed path's stand-in together with a
// line anchor that followed the path: ":57", ":57-70", ":57,70".
const markedAnchor = listedPathMark + `(?::\d+(?:-\d+)?(?:,\s*\d+(?:-\d+)?)*)?`

var (
	markedAnchorRe = regexp.MustCompile(markedAnchor)
	// emptiedSpanRe matches a backticked span that held one listed path and
	// nothing else. Such spans are removed whole: with only their contents
	// blanked, the closing backtick of one and the opening backtick of the
	// next would read as a span around the prose between them.
	emptiedSpanRe = regexp.MustCompile("`\\s*" + markedAnchor + "\\s*`")
	// codeSpanRe matches a backticked span that still holds something once
	// the listed paths are gone.
	codeSpanRe = regexp.MustCompile("`[^`]*[^`\\s][^`]*`")
	// codeTokenRe matches what reads as a code reference outside backticks: a
	// dotted name or file name (Foo.Bar, x.go), a path, a call, a snake_case
	// or a camelCase identifier.
	codeTokenRe = regexp.MustCompile(`[A-Za-z_]\w*(?:\.[A-Za-z_]\w*)+|\w/\w|\w\(|[A-Za-z]\w*_\w+|[a-z]+[A-Z]\w*`)
	// midSentenceCapitalRe matches a capitalised word that does not open the
	// claim or a sentence: in a claim about code that is a type or symbol
	// name (Store, Reviewer) far more often than a proper noun.
	midSentenceCapitalRe = regexp.MustCompile(`[^\s.!?]\s+[A-Z]\w*`)
)

// isPathRune reports whether r can be part of a path, so that a listed path
// found next to one is only a piece of a longer, different path.
func isPathRune(r rune) bool {
	return r == '_' || r == '-' || r == '/' || r == '.' ||
		(r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// markListedPath replaces every whole occurrence of path p in s with
// listedPathMark and reports whether there was one. An occurrence is whole
// when no path character touches it on the left, and on the right nothing
// that would continue it: a path character other than a dot, or a dot that an
// extension follows. So with handlers.go listed, review_handlers.go,
// pkg/handlers.go and handlers.go.tmpl are left alone, and "handlers.go." at
// the end of a sentence is marked.
func markListedPath(s, p string) (string, bool) {
	var b strings.Builder
	marked := false
	for {
		i := strings.Index(s, p)
		if i < 0 {
			break
		}
		end := i + len(p)
		whole := (i == 0 || !isPathRune(rune(s[i-1]))) && !continuesPath(s[end:])
		if whole {
			b.WriteString(s[:i])
			b.WriteString(listedPathMark)
			marked = true
		} else {
			b.WriteString(s[:end])
		}
		s = s[end:]
	}
	b.WriteString(s)
	return b.String(), marked
}

// continuesPath reports whether rest, the text right after a path, would make
// the path a longer one.
func continuesPath(rest string) bool {
	if rest == "" {
		return false
	}
	if rest[0] == '.' {
		return len(rest) > 1 && isPathRune(rune(rest[1])) && rest[1] != '.'
	}
	return isPathRune(rune(rest[0]))
}

// claimIsOnlyListedPaths reports whether claim, an unverifiable-claim
// finding's evidence, names at least one of files, whole, and nothing else
// that reads as a code reference. Such a claim only repeats where the task says it
// works. A claim that also names a symbol, another path or a convention is
// still a claim, and anything this cannot tell apart is kept.
func claimIsOnlyListedPaths(claim string, files []string) bool {
	paths := append([]string(nil), files...)
	// Longest first, so a path that is a prefix of another does not split it.
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	rest := strings.ReplaceAll(claim, listedPathMark, " ")
	named := false
	for _, p := range paths {
		if p == "" {
			continue
		}
		var marked bool
		if rest, marked = markListedPath(rest, p); marked {
			named = true
		}
	}
	if !named {
		return false
	}
	rest = emptiedSpanRe.ReplaceAllString(rest, " ")
	rest = markedAnchorRe.ReplaceAllString(rest, " ")
	return !codeSpanRe.MatchString(rest) && !codeTokenRe.MatchString(rest) && !midSentenceCapitalRe.MatchString(rest)
}

// dropListedFileClaims removes every unverifiable_codebase_claim finding
// whose evidence is only about paths in files, the paths the task's own
// Files: section lists. Other findings pass through unchanged, and with no
// files the input slice is returned as it is.
func dropListedFileClaims(findings []verdict.Finding, files []string) []verdict.Finding {
	if len(files) == 0 || len(findings) == 0 {
		return findings
	}
	out := make([]verdict.Finding, 0, len(findings))
	for _, f := range findings {
		if f.Category == verdict.CategoryUnverifiableCodebaseClaim && claimIsOnlyListedPaths(f.Evidence, files) {
			continue
		}
		out = append(out, f)
	}
	return out
}
