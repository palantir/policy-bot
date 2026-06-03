// Copyright 2018 Palantir Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Command mockui renders the policy-bot web UI with hardcoded fixture data so
// that designers can iterate on the templates and styles without configuring a
// GitHub App, database, or any credentials.
//
// Usage:
//
//	npm run build          # compile CSS/JS into build/static
//	go run ./cmd/mockui     # serve the mock UI on http://localhost:8080
//
// The mock uses the real templates and stylesheet, so what you see here is
// exactly what the running server renders.
package main

import (
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/bluekeyes/templatetree"
	"github.com/google/go-github/v81/github"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/server/handler"
)

const githubURL = "https://github.com"

// detailsData mirrors the anonymous struct that handler.Details passes to
// details.html.tmpl. The template accesses fields by name, so any struct with
// matching fields works.
type detailsData struct {
	BasePath  string
	User      string
	PolicyURL string

	ExpandRequiredReviewers bool

	Error            error
	IsTemporaryError bool

	PullRequest *github.PullRequest
	Result      *common.Result
	Codeowners  *pull.CodeownersResult
	PullContext pull.Context
}

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	staticDir := flag.String("static", "build/static", "directory with built static assets (run `npm run build`)")
	tmplDir := flag.String("templates", "server/templates", "directory with HTML templates")
	flag.Parse()

	templates, err := handler.LoadTemplates(&handler.FilesConfig{
		Static:    *staticDir,
		Templates: *tmplDir,
	}, "", githubURL)
	if err != nil {
		log.Fatalf("failed to load templates: %v", err)
	}

	scenarios := buildScenarios()
	order := []string{"approved", "pending", "disapproved", "complex", "error"}

	mux := http.NewServeMux()

	// Static assets (CSS/JS/images) built by webpack.
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(*staticDir))))
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/img/favicon.ico", http.StatusFound)
	})

	// Landing index of the app itself.
	mux.HandleFunc("/index", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			AppName    string
			Version    string
			GitHubURL  string
			PolicyPath string
		}
		data.AppName = "policy-bot"
		data.Version = "1.39.0"
		data.GitHubURL = githubURL
		data.PolicyPath = ".policy.yml"
		render(w, templates, "index.html.tmpl", &data)
	})

	// Each scenario of the details page.
	mux.HandleFunc("/mock/details/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/mock/details/")
		data, ok := scenarios[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		render(w, templates, "details.html.tmpl", data)
	})

	// htmx endpoint for the lazily-loaded "Required Reviewers" section in the
	// pending scenario. The path is whatever the template generated:
	// /details/<owner>/<repo>/<number>/reviewers
	mux.HandleFunc("/details/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/reviewers") {
			http.NotFound(w, r)
			return
		}
		render(w, templates, "details_reviewers.html.tmpl", mockReviewers())
	})

	// Gallery: a simple chooser so designers know where to look.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		writeGallery(w, order)
	})

	log.Printf("policy-bot mock UI: http://localhost%s", normalizeAddr(*addr))
	log.Printf("  static assets from %q (run `npm run build` if missing)", *staticDir)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func render(w http.ResponseWriter, t templatetree.Tree[*template.Template], name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var buf io.Writer = w
	if err := t.ExecuteTemplate(buf, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func normalizeAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return addr
	}
	return ":" + addr
}

func writeGallery(w http.ResponseWriter, order []string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	b.WriteString(`<!doctype html><meta charset="utf-8"><title>PolicyBot mock UI</title>`)
	b.WriteString(`<style>body{font:16px/1.5 system-ui,sans-serif;max-width:640px;margin:4rem auto;padding:0 1.5rem;color:#1c2127}` +
		`@media(prefers-color-scheme:dark){body{background:#181c22;color:#f1f3f6}a{color:#8abbff}}` +
		`h1{letter-spacing:-.02em}a{display:inline-block}ul{list-style:none;padding:0}` +
		`li{margin:.5rem 0}.k{display:inline-block;min-width:7rem;font-weight:600}</style>`)
	b.WriteString(`<h1>PolicyBot &mdash; mock UI</h1>`)
	b.WriteString(`<p>Rendered with the real templates and stylesheet. Toggle your OS light/dark theme to preview both.</p>`)
	b.WriteString(`<h2>Details page</h2><ul>`)
	for _, name := range order {
		fmt.Fprintf(&b, `<li><span class="k">%s</span><a href="/mock/details/%s">/mock/details/%s</a></li>`, name, name, name)
	}
	b.WriteString(`</ul><h2>Landing page</h2><ul><li><span class="k">index</span><a href="/index">/index</a></li></ul>`)
	_, _ = w.Write([]byte(b.String()))
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

func buildScenarios() map[string]*detailsData {
	pr := mockPR()
	codeowners := &pull.CodeownersResult{
		Owners: map[string][]string{
			"api/server.go":   {"@acme/platform"},
			"api/handlers.go": {"@acme/platform"},
			"docs/guide.md":   {"@carol"},
		},
		OrphanFiles: []string{"scripts/release.sh"},
	}

	base := func(result *common.Result) *detailsData {
		return &detailsData{
			BasePath:    "",
			User:        "designer",
			PolicyURL:   "https://github.com/acme/example-repo/blob/main/.policy.yml",
			PullRequest: pr,
			Result:      result,
			Codeowners:  codeowners,
		}
	}

	approved := base(approvedTree())

	pending := base(pendingTree())
	pending.ExpandRequiredReviewers = true

	disapproved := base(disapprovedTree())

	complex := base(complexTree())
	complex.ExpandRequiredReviewers = true

	errData := base(nil)
	errData.Error = fmt.Errorf("failed to parse policy: yaml: line 12: did not find expected key")

	return map[string]*detailsData{
		"approved":    approved,
		"pending":     pending,
		"disapproved": disapproved,
		"complex":     complex,
		"error":       errData,
	}
}

func mockPR() *github.PullRequest {
	return &github.PullRequest{
		Number:  github.Ptr(248),
		Title:   github.Ptr("Add health check endpoint"),
		HTMLURL: github.Ptr("https://github.com/acme/example-repo/pull/248"),
		Base: &github.PullRequestBranch{
			Ref: github.Ptr("main"),
			Repo: &github.Repository{
				FullName: github.Ptr("acme/example-repo"),
				HTMLURL:  github.Ptr("https://github.com/acme/example-repo"),
			},
		},
	}
}

func githubReview() *common.Methods {
	t := true
	return &common.Methods{
		Comments:          []string{":+1:", "lgtm"},
		GithubReview:      &t,
		GithubReviewState: pull.ReviewApproved,
	}
}

func candidate(login, initials, color string) *common.Candidate {
	return &common.Candidate{
		Author: &pull.Author{Login: login, AvatarURL: avatarURI(initials, color)},
	}
}

func approvedTree() *common.Result {
	return &common.Result{
		Name:              "acme approval policy",
		Status:            common.StatusApproved,
		StatusDescription: "All required approvals have been met",
		Children: []*common.Result{
			{
				Name:              "engineering review",
				Description:       "At least one approval from the platform team",
				Status:            common.StatusApproved,
				StatusDescription: "Approved by alice",
				Methods:           githubReview(),
				Requires: common.RequiresResult{
					Count:     1,
					Actors:    common.Actors{Teams: []string{"acme/platform"}},
					Approvers: []*common.Candidate{candidate("alice", "AL", "#2d72d2")},
				},
			},
			{
				Name:              "security sign-off",
				Description:       "Security must approve changes to the API surface",
				Status:            common.StatusApproved,
				StatusDescription: "Approved by bob",
				Methods:           githubReview(),
				Requires: common.RequiresResult{
					Count:     1,
					Actors:    common.Actors{Organizations: []string{"acme"}},
					Approvers: []*common.Candidate{candidate("bob", "BO", "#238551")},
				},
			},
			{
				Name:              "codeowners",
				Description:       "Owners of the changed files must approve",
				Status:            common.StatusApproved,
				StatusDescription: "Approved by all required codeowners",
				Methods:           githubReview(),
				Requires: common.RequiresResult{
					Count:     1,
					Actors:    common.Actors{Codeowners: true},
					Approvers: []*common.Candidate{candidate("carol", "CA", "#c87619"), candidate("dave", "DV", "#cd4246")},
					// A directory co-owned by two teams forms a single ownership
					// group. The approver here satisfies it via membership in the
					// SECOND owner (@acme/team-beta), so the UI must show both
					// owners, not just the first.
					OwnershipGroups: []common.OwnershipGroupResult{
						{
							Key:       "@acme/team-alpha,@acme/team-beta",
							Owners:    []string{"@acme/team-alpha", "@acme/team-beta"},
							Files:     []string{"shared/config.yaml"},
							Satisfied: true,
							Approvers: []string{"dave"},
						},
					},
				},
			},
			skippedDocsRule(),
		},
	}
}

func pendingTree() *common.Result {
	return &common.Result{
		Name:              "acme approval policy",
		Status:            common.StatusPending,
		StatusDescription: "Waiting on 2 of 4 rules",
		Children: []*common.Result{
			{
				Name:              "engineering review",
				Description:       "At least one approval from the platform team",
				Status:            common.StatusApproved,
				StatusDescription: "Approved by alice",
				Methods:           githubReview(),
				Requires: common.RequiresResult{
					Count:     1,
					Actors:    common.Actors{Teams: []string{"acme/platform"}},
					Approvers: []*common.Candidate{candidate("alice", "AL", "#2d72d2")},
				},
			},
			{
				Name:              "security sign-off",
				Description:       "Security must approve changes to the API surface",
				Status:            common.StatusPending,
				StatusDescription: "0 of 1 required approvals",
				Methods:           githubReview(),
				Requires: common.RequiresResult{
					Count:  1,
					Actors: common.Actors{Organizations: []string{"acme"}, Users: []string{"bob", "judy"}},
				},
			},
			{
				Name:              "release manager",
				Description:       "A user with admin permission must approve releases",
				Status:            common.StatusPending,
				StatusDescription: "0 of 1 required approvals",
				Methods:           githubReview(),
				Requires: common.RequiresResult{
					Count:  1,
					Actors: common.Actors{Permissions: []pull.Permission{pull.PermissionAdmin}},
				},
			},
			skippedDocsRule(),
		},
	}
}

func disapprovedTree() *common.Result {
	return &common.Result{
		Name:              "acme approval policy",
		Status:            common.StatusDisapproved,
		StatusDescription: "Changes were explicitly requested",
		Children: []*common.Result{
			{
				Name:              "engineering review",
				Description:       "At least one approval from the platform team",
				Status:            common.StatusDisapproved,
				StatusDescription: "Changes requested by alice",
				Methods:           githubReview(),
				Requires: common.RequiresResult{
					Count:  1,
					Actors: common.Actors{Teams: []string{"acme/platform"}},
				},
			},
			{
				Name:              "security sign-off",
				Description:       "Security must approve changes to the API surface",
				Status:            common.StatusApproved,
				StatusDescription: "Approved by bob",
				Methods:           githubReview(),
				Requires: common.RequiresResult{
					Count:     1,
					Actors:    common.Actors{Organizations: []string{"acme"}},
					Approvers: []*common.Candidate{candidate("bob", "BO", "#238551")},
				},
			},
			skippedDocsRule(),
		},
	}
}

// complexTree demonstrates nested boolean composition: an "and" at the root,
// containing an "or" and a further "and" that itself nests another "or".
// This mirrors how policy-bot renders chained `and`/`or` approval policies.
func complexTree() *common.Result {
	return &common.Result{
		Name:              "and",
		Status:            common.StatusPending,
		StatusDescription: "2 of 3 requirements met",
		Children: []*common.Result{
			{
				Name:              "engineering review",
				Description:       "At least one approval from the platform team",
				Status:            common.StatusApproved,
				StatusDescription: "Approved by alice",
				Methods:           githubReview(),
				Requires: common.RequiresResult{
					Count:     1,
					Actors:    common.Actors{Teams: []string{"acme/platform"}},
					Approvers: []*common.Candidate{candidate("alice", "AL", "#2d72d2")},
				},
			},
			{
				Name:              "or",
				Status:            common.StatusApproved,
				StatusDescription: "Satisfied by 1 of 2 options",
				Children: []*common.Result{
					{
						Name:              "security sign-off",
						Description:       "Security must approve API changes",
						Status:            common.StatusApproved,
						StatusDescription: "Approved by bob",
						Methods:           githubReview(),
						Requires: common.RequiresResult{
							Count:     1,
							Actors:    common.Actors{Organizations: []string{"acme"}},
							Approvers: []*common.Candidate{candidate("bob", "BO", "#238551")},
						},
					},
					{
						Name:              "external security audit",
						Status:            common.StatusSkipped,
						StatusDescription: "The rule did not apply to this pull request",
						PredicateResults: []*common.PredicateResult{
							{
								ValuePhrase:     "changed files",
								Values:          []string{"api/server.go", "api/handlers.go"},
								ConditionPhrase: "match the patterns",
								ConditionValues: []string{"security/**", "crypto/**"},
							},
						},
						Requires: common.RequiresResult{
							Count:  1,
							Actors: common.Actors{Teams: []string{"acme/security-external"}},
						},
					},
				},
			},
			{
				Name:              "and",
				Status:            common.StatusPending,
				StatusDescription: "1 of 2 requirements met",
				Children: []*common.Result{
					{
						Name:              "release manager approval",
						Description:       "A user with admin permission signs off releases",
						Status:            common.StatusApproved,
						StatusDescription: "Approved by erin",
						Methods:           githubReview(),
						Requires: common.RequiresResult{
							Count:     1,
							Actors:    common.Actors{Permissions: []pull.Permission{pull.PermissionAdmin}},
							Approvers: []*common.Candidate{candidate("erin", "ER", "#9d3f9d")},
						},
					},
					{
						Name:              "or",
						Status:            common.StatusPending,
						StatusDescription: "Waiting on 1 of 2 options",
						Children: []*common.Result{
							{
								Name:              "qa sign-off",
								Description:       "QA verifies the release candidate",
								Status:            common.StatusPending,
								StatusDescription: "0 of 1 required approvals",
								Methods:           githubReview(),
								Requires: common.RequiresResult{
									Count:  1,
									Actors: common.Actors{Teams: []string{"acme/qa"}, Users: []string{"frank"}},
								},
							},
							{
								Name:              "product sign-off",
								Description:       "Product owner approves user-facing changes",
								Status:            common.StatusPending,
								StatusDescription: "0 of 1 required approvals",
								Methods:           githubReview(),
								Requires: common.RequiresResult{
									Count:  1,
									Actors: common.Actors{Users: []string{"grace", "heidi"}},
								},
							},
						},
					},
				},
			},
		},
	}
}

func skippedDocsRule() *common.Result {
	return &common.Result{
		Name:              "documentation review",
		Status:            common.StatusSkipped,
		StatusDescription: "The rule did not apply to this pull request",
		PredicateResults: []*common.PredicateResult{
			{
				ValuePhrase:     "changed files",
				Values:          []string{"api/server.go", "api/handlers.go"},
				ConditionPhrase: "match the patterns",
				ConditionValues: []string{"docs/**", "*.md"},
			},
		},
		Requires: common.RequiresResult{
			Count:  1,
			Actors: common.Actors{Teams: []string{"acme/docs"}},
		},
	}
}

func mockReviewers() *handler.DetailsReviewersData {
	return &handler.DetailsReviewersData{
		PullRequest: mockPR(),
		Groups: []handler.ReviewerGroup{
			{
				Name: "Members of the teams",
				Reviewers: []handler.ReviewerInfo{
					{Name: "Bob Stone", Username: "bob", Link: githubURL + "/bob", AvatarURL: avatarURI("BO", "#238551")},
					{Name: "Judy Hall", Username: "judy", Link: githubURL + "/judy", AvatarURL: avatarURI("JU", "#9d3f9d")},
				},
			},
			{
				Name: "Users",
				Reviewers: []handler.ReviewerInfo{
					{Name: "Sam Lee", Username: "sam", Link: githubURL + "/sam", AvatarURL: avatarURI("SL", "#c87619")},
				},
			},
		},
	}
}

// avatarURI returns a fake profile picture for the mock UI. It uses picsum.photos
// with a per-person seed so each user gets a stable, distinct placeholder image.
// The initials/color args are kept for call-site readability but only seed the URL.
func avatarURI(initials, color string) string {
	return "https://picsum.photos/seed/" + url.PathEscape(initials+color) + "/90/90"
}
