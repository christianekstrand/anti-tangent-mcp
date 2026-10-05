package mcpsrv

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patiently/anti-tangent-mcp/internal/planrun"
	"github.com/patiently/anti-tangent-mcp/internal/providers"
	"github.com/patiently/anti-tangent-mcp/internal/verdict"
)

func TestClaimIsOnlyListedPaths(t *testing.T) {
	files := []string{"pkg/store.go", "pkg/store_test.go", "pkg/store", "store.go"}
	cases := []struct {
		name  string
		claim string
		want  bool
	}{
		{"bare path", "pkg/store.go", true},
		{"backticked path with a verb", "Modify: `pkg/store.go`", true},
		{"path with a line anchor", "Modify: `pkg/store.go:120-145, 160`", true},
		{"two listed paths in prose", "The task edits pkg/store.go and adds pkg/store_test.go; neither can be checked from the text.", true},
		{"a listed directory that prefixes a listed file", "Files under pkg/store and pkg/store.go", true},
		{"no listed path", "internal/other.go exists", false},
		{"listed path plus a backticked symbol", "`Store.Get` in `pkg/store.go` returns an error", false},
		{"listed path plus a bare dotted symbol", "pkg/store.go defines Store.Get", false},
		{"listed path plus a call", "pkg/store.go has newStore()", false},
		{"listed path plus a snake_case name", "pkg/store.go reads max_entries", false},
		{"listed path plus a camelCase name", "pkg/store.go keeps evictOldest", false},
		{"listed path plus an unlisted path", "pkg/store.go mirrors pkg/cache.go", false},
		{"listed path plus an unlisted path made of restatement words", "pkg/store.go and new/file", false},
		{"a two-operation label is not a path", "Create/Modify: `pkg/store.go`", true},
		{"listed path plus an unlisted file name", "pkg/store.go mirrors cache.go", false},
		{"a listed name inside a longer file name", "review_store.go holds the switch", false},
		{"a listed path under another directory", "lib/pkg/store.go holds the switch", false},
		{"a listed path with a further extension", "pkg/store.go.tmpl is the template", false},
		{"a listed path ending a sentence", "The task edits pkg/store.go.", true},
		{"a listed path in parentheses", "The file (pkg/store.go) cannot be checked.", true},
		{"listed path plus a capitalised symbol", "pkg/store.go already defines Store with a mutex", false},
		{"listed path plus two capitalised symbols", "pkg/store.go registers the Reviewer interface with Registry", false},
		{"a capital that opens a second sentence", "pkg/store.go is listed. It cannot be checked.", true},
		{"two backticked listed paths", "`pkg/store.go` and `pkg/store_test.go`", true},
		{"two backticked listed paths in prose", "Modify `pkg/store.go` and `pkg/store_test.go` per the plan", true},
		{"two backticked listed paths, comma-separated", "`pkg/store.go`, `pkg/store_test.go`", true},
		{"two backticked listed paths with line anchors", "`pkg/store.go:10-20` and ` pkg/store_test.go:5 `", true},
		{"a backticked listed path and a backticked symbol", "`pkg/store.go` and `Store.Get`", false},
		{"what a listed file already does", "pkg/store.go already retries on timeout", false},
		{"a convention a listed file follows", "pkg/store.go wraps errors with the repository's usual helper", false},
		{"what a listed file holds", "pkg/store.go has three exported methods", false},
		{"a label, a verb and filler only", "Create: pkg/store.go; the task also modifies pkg/store_test.go, which cannot be verified from the plan text.", true},
		{"empty claim", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, claimIsOnlyListedPaths(tc.claim, files))
		})
	}
	assert.False(t, claimIsOnlyListedPaths("pkg/store.go", nil), "no listed files, nothing to drop")
}

func TestDropListedFileClaims_OnlyTouchesUnverifiableClaims(t *testing.T) {
	files := []string{"pkg/store.go"}
	listed := verdict.Finding{Category: verdict.CategoryUnverifiableCodebaseClaim, Evidence: "Modify: `pkg/store.go`"}
	symbol := verdict.Finding{Category: verdict.CategoryUnverifiableCodebaseClaim, Evidence: "`Store.Get` exists in pkg/store.go"}
	other := verdict.Finding{Category: verdict.CategoryAmbiguousSpec, Evidence: "pkg/store.go"}

	assert.Equal(t, []verdict.Finding{symbol, other}, dropListedFileClaims([]verdict.Finding{listed, symbol, other}, files))
	in := []verdict.Finding{listed}
	assert.Equal(t, in, dropListedFileClaims(in, nil))
}

const planWithFilesSections = "# Plan\n\n" +
	"### Task 1: store\n\n**Goal:** g1\n\n**Files:**\n- Modify: `pkg/store.go:10-20`\n- Test: `pkg/store_test.go`\n\n**Acceptance criteria:**\n- ac1\n\n" +
	"### Task 2: cache\n\n**Goal:** g2\n\n**Acceptance criteria:**\n- ac2\n\n"

func TestValidatePlan_ListedFilePathsAreNotOnTheChecklist(t *testing.T) {
	raw := []byte(`{"plan_verdict":"warn","plan_quality":"actionable","plan_findings":[],
		"tasks":[
			{"task_index":1,"task_title":"Task 1: store","verdict":"warn","findings":[
				{"severity":"minor","category":"unverifiable_codebase_claim","criterion":"spec","evidence":"Modify: ` + "`pkg/store.go:10-20`" + `","suggestion":"verify"},
				{"severity":"minor","category":"unverifiable_codebase_claim","criterion":"spec","evidence":"` + "`Store.Get`" + ` in pkg/store.go returns an error","suggestion":"verify"}
			],"suggested_header_block":"","suggested_header_reason":""},
			{"task_index":2,"task_title":"Task 2: cache","verdict":"warn","findings":[
				{"severity":"minor","category":"unverifiable_codebase_claim","criterion":"spec","evidence":"pkg/store.go","suggestion":"verify"}
			],"suggested_header_block":"","suggested_header_reason":""}
		],
		"next_action":"n"}`)
	d := newDepsWithScripted(t, &scriptedReviewer{responses: []providers.Response{{RawJSON: raw, Model: "claude-sonnet-4-6"}}}, 8)
	h := &handlers{deps: d}

	_, pr, err := h.ValidatePlan(context.Background(), nil, ValidatePlanArgs{PlanText: planWithFilesSections})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"Task 1: `Store.Get` in pkg/store.go returns an error",
		"Task 2: pkg/store.go",
	}, pr.CodebaseReferenceChecklist, "Task 1 lists the path; Task 2 does not, so there it is still a claim")
}

func TestValidatePlan_APlanWhoseOnlyClaimsAreListedPathsHasNoChecklist(t *testing.T) {
	raw := []byte(`{"plan_verdict":"warn","plan_quality":"actionable","plan_findings":[],
		"tasks":[
			{"task_index":1,"task_title":"Task 1: store","verdict":"warn","findings":[
				{"severity":"minor","category":"unverifiable_codebase_claim","criterion":"spec","evidence":"pkg/store_test.go","suggestion":"verify"}
			],"suggested_header_block":"","suggested_header_reason":""},
			{"task_index":2,"task_title":"Task 2: cache","verdict":"pass","findings":[],"suggested_header_block":"","suggested_header_reason":""}
		],
		"next_action":"reviewer text"}`)
	d := newDepsWithScripted(t, &scriptedReviewer{responses: []providers.Response{{RawJSON: raw, Model: "claude-sonnet-4-6"}}}, 8)
	h := &handlers{deps: d}

	_, pr, err := h.ValidatePlan(context.Background(), nil, ValidatePlanArgs{PlanText: planWithFilesSections})
	require.NoError(t, err)
	assert.Empty(t, pr.CodebaseReferenceChecklist)
	assert.Equal(t, verdict.VerdictPass, pr.PlanVerdict)
	assert.NotContains(t, pr.NextAction, "codebase_reference_checklist")
}

func taskSpecClaimsReviewer() *fakeReviewer {
	return &fakeReviewer{name: "anthropic", resp: providers.Response{
		RawJSON: []byte(`{"verdict":"pass","findings":[
			{"severity":"minor","category":"unverifiable_codebase_claim","criterion":"spec","evidence":"pkg/store.go","suggestion":"verify"},
			{"severity":"minor","category":"unverifiable_codebase_claim","criterion":"spec","evidence":"pkg/cache.go","suggestion":"verify"}
		],"next_action":"go"}`),
		Model: "claude-sonnet-4-6",
	}}
}

func TestValidateTaskSpec_DropsPathsTheContextFilesSectionLists(t *testing.T) {
	h := &handlers{deps: newDeps(t, taskSpecClaimsReviewer())}
	_, env, err := h.ValidateTaskSpec(context.Background(), nil, ValidateTaskSpecArgs{
		TaskTitle: "store", Goal: "g",
		Context: "Background.\n\n**Files:**\n- Modify: `pkg/store.go`\n",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg/cache.go"}, env.CodebaseReferenceChecklist)
}

func TestValidateTaskSpec_DropsPathsThePlanRunListsForTheTask(t *testing.T) {
	h := &handlers{deps: newDeps(t, taskSpecClaimsReviewer())}
	run := h.deps.PlanRuns.CreateWithTasks("pass", "rigorous", []planrun.PlanTask{
		{Index: 1, Title: "Task 1: store", Files: []string{"pkg/store.go"}},
		{Index: 2, Title: "Task 2: cache", Files: []string{"pkg/cache.go"}},
	})

	_, byIndex, err := h.ValidateTaskSpec(context.Background(), nil, ValidateTaskSpecArgs{
		TaskTitle: "anything", Goal: "g", PlanRunID: run.ID, TaskIndex: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg/store.go"}, byIndex.CodebaseReferenceChecklist)

	_, byTitle, err := h.ValidateTaskSpec(context.Background(), nil, ValidateTaskSpecArgs{
		TaskTitle: "store", Goal: "g", PlanRunID: run.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg/cache.go"}, byTitle.CodebaseReferenceChecklist)

	_, unknown, err := h.ValidateTaskSpec(context.Background(), nil, ValidateTaskSpecArgs{
		TaskTitle: "store", Goal: "g", PlanRunID: "pr_000000000000",
	})
	require.NoError(t, err)
	assert.Len(t, unknown.CodebaseReferenceChecklist, 2, "an unknown run lists nothing, so nothing is dropped")
}
