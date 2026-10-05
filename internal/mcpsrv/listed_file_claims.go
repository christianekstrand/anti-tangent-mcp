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

// wordRe matches one word of a claim: a run of letters, digits and
// underscores that holds at least one letter. Line anchors and other bare
// numbers are not words.
var wordRe = regexp.MustCompile(`[0-9_]*[A-Za-z][A-Za-z0-9_]*`)

// joinedLabelRe matches a Files: bullet label that names two operations,
// such as "Create/Modify". It is removed before pathShapedRe looks for a
// path, since its slash joins two labels, not two path segments.
var joinedLabelRe = regexp.MustCompile(`(?i)\b(?:create|modify|delete|test)(?:/(?:create|modify|delete|test))+\b`)

// pathShapedRe matches what is left of a path once the listed paths are
// gone: two segments joined by a slash. A path made only of restatement
// words, such as new/file, would otherwise pass the word check.
var pathShapedRe = regexp.MustCompile(`[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+`)

// restatementWords are the words a claim may use, besides the listed paths
// themselves, and still say nothing but where the task works and that the
// reviewer could not check it: the Files: bullet labels, the verbs for
// touching a file, and filler. A word outside this set — a symbol, a verb
// about what the file does or holds, a name — makes the claim a statement
// about the codebase, which stays on the checklist.
var restatementWords = wordSet(
	"create creates created modify modifies modified delete deletes deleted test tests " +
		"add adds added edit edits edited update updates updated touch touches touched change changes changed " +
		"file files path paths line lines section task plan spec text list lists listed new existing " +
		"exist exists existence present verify verified unverifiable check checked confirm confirmed " +
		"a an the this that these those it its they their and or nor neither both not no " +
		"is are be can cannot could will would must should " +
		"in at of to from for on as with under per by alone also which whether if but",
)

func wordSet(words string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.Fields(words) {
		set[w] = true
	}
	return set
}

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
// finding's evidence, names at least one of files, whole, and beyond the
// listed paths uses only restatementWords. Such a claim only repeats where the
// task says it works. A claim that says anything else — what a listed file
// does or holds, a symbol, another path, a convention — is still a claim, and
// anything this cannot tell apart is kept.
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
	rest = joinedLabelRe.ReplaceAllString(rest, " ")
	if pathShapedRe.MatchString(rest) {
		return false
	}
	for _, word := range wordRe.FindAllString(rest, -1) {
		if !restatementWords[strings.ToLower(word)] {
			return false
		}
	}
	return true
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
