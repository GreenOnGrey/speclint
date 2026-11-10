package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GreenOnGrey/speclint/contract"
	"github.com/GreenOnGrey/speclint/starter"
)

// The checks of the mode service (FTR.HMR.SPEC-0002 R8, R9, R13, tech §2.1).
const (
	checkUnknownTestID = "unknown-test-id"
	checkVersionMinor  = "version-minor"
)

// The sources of the token (FTR.HMR.SPEC-0003 arch §3.2), never a flag, so it stays out of the command line:
// the installation token, the OIDC token of GitHub Actions, the OIDC token a GitLab job declares.
const (
	tokenEnv      = "HAMMURAPI_TOKEN"
	gitlabIDToken = "HAMMURAPI_ID_TOKEN"
	ghRequestURL  = "ACTIONS_ID_TOKEN_REQUEST_URL"
	ghRequestTok  = "ACTIONS_ID_TOKEN_REQUEST_TOKEN"
)

// ciToken finds the token of the run in this order: HAMMURAPI_TOKEN, the OIDC token of the GitHub Actions job
// for the audience of Hammurapi, HAMMURAPI_ID_TOKEN of a GitLab job. Without one the run fails with a hint for
// its CI (R5).
func ciToken(ctx context.Context, client *http.Client, audience string, env func(string) string) (string, error) {
	if t := env(tokenEnv); t != "" {
		return t, nil
	}
	if u, t := env(ghRequestURL), env(ghRequestTok); u != "" && t != "" {
		return githubOIDC(ctx, client, u, t, audience)
	}
	if t := env(gitlabIDToken); t != "" {
		return t, nil
	}
	switch {
	case env("GITHUB_ACTIONS") == "true":
		return "", &runError{"token-invalid", `no OIDC token: allow "id-token: write" in the permissions of the workflow, or set ` + tokenEnv}
	case env("GITLAB_CI") == "true":
		return "", &runError{"token-invalid", "no OIDC token: declare id_tokens " + gitlabIDToken + " with aud " + audience + " in the job, or set " + tokenEnv}
	}
	return "", &runError{"token-invalid", "no token: set " + tokenEnv + ", or run in GitHub Actions or GitLab CI with an OIDC token"}
}

// githubOIDC asks the runner of GitHub Actions for the OIDC token of the job, issued for the audience.
func githubOIDC(ctx context.Context, client *http.Client, requestURL, requestToken, audience string) (string, error) {
	u, err := url.Parse(requestURL)
	if err != nil {
		return "", &runError{"token-invalid", "bad " + ghRequestURL + ": " + err.Error()}
	}
	q := u.Query()
	q.Set("audience", audience)
	u.RawQuery = q.Encode()
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+requestToken)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", &runError{"token-invalid", "the OIDC token of GitHub Actions: " + err.Error()}
	}
	defer resp.Body.Close()
	var body struct {
		Value string `json:"value"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", &runError{"token-invalid", fmt.Sprintf("the runner refused the OIDC token of GitHub Actions: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(raw)))}
	}
	if err := json.Unmarshal(raw, &body); err != nil || body.Value == "" {
		return "", &runError{"token-invalid", "the runner answered without an OIDC token"}
	}
	return body.Value, nil
}

// serviceCases is the answer of GET /api/v1/ci/services/{key}/test-cases of Hammurapi (tech §3.1).
type serviceCases struct {
	ContractVersion string `json:"contractVersion"`
	Service         string `json:"service"`
	Features        []struct {
		Key      string `json:"key"`
		Phase    string `json:"phase"`
		Required bool   `json:"required"`
	} `json:"features"`
	Cases []struct {
		ID      string `json:"id"`
		Feature string `json:"feature"`
		Level   string `json:"level"`
	} `json:"cases"`
}

// runError is a run that could not check: Hammurapi, the token, the service or the version (exit code 2).
type runError struct{ check, msg string }

func (e *runError) Error() string { return e.check + ": " + e.msg }

// The waits between the attempts of a request to Hammurapi (R12): three attempts, then the run fails.
var retryWaits = []time.Duration{2 * time.Second, 4 * time.Second}

const requestTimeout = 10 * time.Second

// fetchCases asks Hammurapi for the features and test cases of the service; it retries a failed connection, a 5xx
// and a 429, and fails at once on an answer about the token or the service.
func fetchCases(ctx context.Context, client *http.Client, base, service, token string) (*serviceCases, error) {
	u, err := apiURL(base, "/api/v1/ci/services/"+url.PathEscape(service)+"/test-cases")
	if err != nil {
		return nil, err
	}
	var last string
	for attempt := 0; ; attempt++ {
		got, retry, err := fetchOnce(ctx, client, u.String(), token)
		if err == nil {
			return got, nil
		}
		if !retry {
			return nil, err
		}
		last = err.Error()
		if attempt >= len(retryWaits) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryWaits[attempt]):
		}
	}
	return nil, &runError{"hammurapi-unreachable", fmt.Sprintf("Hammurapi is unreachable at %s after %d attempts: %s", base, len(retryWaits)+1, last)}
}

func fetchOnce(ctx context.Context, client *http.Client, u, token string) (*serviceCases, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "speclint/"+moduleVersion())
	resp, err := client.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, true, err
	}
	if resp.StatusCode == http.StatusOK {
		var got serviceCases
		if err := json.Unmarshal(body, &got); err != nil {
			return nil, false, &runError{"hammurapi-unreachable", "the answer of Hammurapi is not the list of test cases: " + err.Error()}
		}
		return &got, false, nil
	}
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Code string `json:"code"`
	}
	_ = json.Unmarshal(body, &e)
	code, msg := e.Error.Code, e.Error.Message
	if code == "" {
		code = e.Code
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized && code == "token_revoked":
		return nil, false, &runError{"token-revoked", "the installation token is revoked: a global administrator issues a new one in the settings of Hammurapi"}
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, false, &runError{"token-invalid", "Hammurapi does not accept the token: " + orDefault(msg, "unknown token")}
	case resp.StatusCode == http.StatusForbidden && code == "repo_not_in_catalog":
		return nil, false, &runError{"repo-not-in-catalog", orDefault(msg, "the repository is not the repository of any service in Hammurapi")}
	case resp.StatusCode == http.StatusForbidden:
		return nil, false, &runError{"token-scope", orDefault(msg, "the token is of another repository")}
	case resp.StatusCode == http.StatusNotFound:
		return nil, false, &runError{"service-not-found", "Hammurapi has no such service in its catalog"}
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return nil, true, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil, false, &runError{"hammurapi-unreachable", fmt.Sprintf("Hammurapi answered HTTP %d %s", resp.StatusCode, code)}
}

// compareVersions checks speclint against the contract of Hammurapi (R13): another major version fails the run,
// an older minor one is a warning.
func compareVersions(mine, theirs string) (*issue, error) {
	a, okA := starter.ParseVersion(strings.TrimPrefix(mine, "v"))
	b, okB := starter.ParseVersion(strings.TrimPrefix(theirs, "v"))
	if !okA || !okB {
		return nil, nil // a development build, or an old Hammurapi without a version: nothing to compare
	}
	if a[0] != b[0] {
		return nil, &runError{"version-major", fmt.Sprintf("speclint %s reads contract %d.x, Hammurapi is on contract %s: update speclint to %d.x", mine, a[0], theirs, b[0])}
	}
	if a[1] < b[1] {
		return &issue{"", checkVersionMinor, theirs, fmt.Sprintf("speclint %s is older than the contract of Hammurapi %s: update speclint", mine, theirs), 0}, nil
	}
	return nil, nil
}

// codeHit is an ID found on a line of a file of the repository.
type codeHit struct {
	file string
	line int
	id   string
}

// repoHits are the IDs of a repository with their places and the feature keys each file names.
type repoHits struct {
	hits  []codeHit
	files map[string]map[string]bool // file → feature keys named in it
}

var grepLineRe = regexp.MustCompile(`^(.+?):(\d+):(.*)$`)

// scanRepo looks for what looks like a test case ID in the tracked files of root, except the baseline: its lines
// name the IDs they excuse.
func scanRepo(root, baseline string) (*repoHits, error) {
	// quotePath=false: a file name outside ASCII comes as it is, not in quotes with octal escapes
	args := []string{"-C", root, "-c", "core.quotePath=false", "grep", "-n", "-I", "-E", codeIDRe, "--", "."}
	if baseline != "" {
		args = append(args, ":(exclude)"+baseline)
	}
	cmd := exec.Command("git", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		var e *exec.ExitError
		if !errors.As(err, &e) || e.ExitCode() != 1 { // 1: no matches
			return nil, fmt.Errorf("git grep in %s: %w", root, err)
		}
	}
	return parseGrep(string(out)), nil
}

func parseGrep(out string) *repoHits {
	r := &repoHits{files: map[string]map[string]bool{}}
	for _, l := range strings.Split(out, "\n") {
		m := grepLineRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		file, text := m[1], m[3]
		n, _ := strconv.Atoi(m[2])
		for _, k := range featureKeyRe.FindAllString(text, -1) {
			if r.files[file] == nil {
				r.files[file] = map[string]bool{}
			}
			r.files[file][k] = true
		}
		// a feature key has the shape of an ID at its end (DEV-0001): it is not an ID
		for _, id := range codeIDPattern.FindAllString(featureKeyRe.ReplaceAllString(text, " "), -1) {
			r.hits = append(r.hits, codeHit{file, n, id})
		}
	}
	return r
}

// idPrefix is the part of an ID before its last number: MOCK-01 → MOCK (arch §3.4).
func idPrefix(id string) string {
	if i := strings.LastIndex(id, "-"); i > 0 {
		return id[:i]
	}
	return id
}

// checkService compares the test cases of the features of the service with the IDs in its code (R8, R9).
func checkService(sc *serviceCases, code *repoHits) []issue {
	var issues []issue
	required := map[string]bool{}
	for _, f := range sc.Features {
		required[f.Key] = f.Required
	}
	owners := map[string]map[string]bool{} // ID → features
	prefixes := map[string]bool{}
	for _, c := range sc.Cases {
		if owners[c.ID] == nil {
			owners[c.ID] = map[string]bool{}
		}
		owners[c.ID][c.Feature] = true
		prefixes[idPrefix(c.ID)] = true
	}
	found := map[string]bool{}     // ID
	qualified := map[string]bool{} // "<key> <ID>": the ID in a file that names the key
	for _, h := range code.hits {
		found[h.id] = true
		for k := range code.files[h.file] {
			qualified[k+" "+h.id] = true
		}
	}

	// R8: a required automated case without its ID in the code; a shared ID counts in a file that names the feature
	seen := map[string]bool{}
	for _, c := range sc.Cases {
		if !required[c.Feature] || c.Level == "M" || seen[c.Feature+" "+c.ID] {
			continue
		}
		seen[c.Feature+" "+c.ID] = true
		shared := len(owners[c.ID]) > 1
		switch {
		case shared && qualified[c.Feature+" "+c.ID]:
		case shared && found[c.ID]:
			issues = append(issues, issue{c.Feature + "/qa", checkNotInCode, c.Feature + " " + c.ID,
				fmt.Sprintf("%s of %s is a test case of several features: name %s in the file of its test", c.ID, c.Feature, c.Feature), 0})
		case !found[c.ID]:
			issues = append(issues, issue{c.Feature + "/qa", checkNotInCode, c.Feature + " " + c.ID,
				fmt.Sprintf("test case %s of %s (%s) is not in the code", c.ID, c.Feature, phaseOf(sc, c.Feature)), 0})
		}
	}

	// R9: an ID that looks like a case of the service and is not one, or is not a case of the feature its file names
	reported := map[string]bool{}
	for _, h := range code.hits {
		var msg string
		if o := owners[h.id]; o == nil && prefixes[idPrefix(h.id)] {
			msg = fmt.Sprintf("%s is not a test case of the features of service %s", h.id, sc.Service)
		} else if o != nil {
			var named []string
			for k := range code.files[h.file] {
				if _, ok := required[k]; ok {
					named = append(named, k)
				}
			}
			if len(named) > 0 && !anyOf(o, named) {
				sort.Strings(named)
				msg = fmt.Sprintf("%s is not a test case of %s, which the file names", h.id, strings.Join(named, ", "))
			}
		}
		if msg != "" && !reported[h.file+" "+h.id] {
			reported[h.file+" "+h.id] = true
			issues = append(issues, issue{h.file, checkUnknownTestID, h.id, msg, h.line})
		}
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].path != issues[j].path {
			return issues[i].path < issues[j].path
		}
		return issues[i].line < issues[j].line
	})
	return issues
}

func anyOf(set map[string]bool, keys []string) bool {
	for _, k := range keys {
		if set[k] {
			return true
		}
	}
	return false
}

func phaseOf(sc *serviceCases, key string) string {
	for _, f := range sc.Features {
		if f.Key == key {
			return f.Phase
		}
	}
	return ""
}

// apiURL is an address of the API of Hammurapi: -hammurapi without a trailing slash and the path.
func apiURL(base, path string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(base, "/") + path)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, &runError{"hammurapi-unreachable", fmt.Sprintf("bad -hammurapi %q: an address like https://hammurapi.example", base)}
	}
	return u, nil
}

func orDefault(s, d string) string {
	if s != "" {
		return s
	}
	return d
}

// runService checks the tests of a service repository against its features in Hammurapi (R8–R13).
func runService(args []string) {
	fl := flag.NewFlagSet("service", flag.ExitOnError)
	root := fl.String("root", ".", "root of the service repository")
	baseline := fl.String("baseline", "", "file of known errors, which do not fail the run")
	update := fl.Bool("write-baseline", false, "write the current errors to the baseline and exit")
	quiet := fl.Bool("errors-only", false, "print only new errors and the summary")
	format := fl.String("format", "text", "output: text, github or gitlab")
	base := fl.String("hammurapi", "", "address of Hammurapi, https://hammurapi.example")
	service := fl.String("service", "", "key of the service in the catalog of Hammurapi")
	_ = fl.Parse(args)
	out := mustOutput(*format, *root, *quiet)
	if *base == "" || *service == "" {
		fail(errors.New("service needs -hammurapi and -service; the token is the OIDC token of the CI job or " + tokenEnv))
	}
	// in the mode service a test case without a test is an error (R8); only the older minor version warns
	warnings = map[string]bool{checkVersionMinor: true}

	ctx, client := context.Background(), &http.Client{}
	if _, err := apiURL(*base, ""); err != nil { // before a token is asked for this address
		fail(err)
	}
	// the audience is the public address of the API of Hammurapi, as -hammurapi names it (FTR.HMR.SPEC-0003 arch A2)
	token, err := ciToken(ctx, client, strings.TrimRight(*base, "/"), os.Getenv)
	if err != nil {
		fail(err)
	}
	sc, err := fetchCases(ctx, client, *base, *service, token)
	if err != nil {
		fail(err)
	}
	var issues []issue
	// the version of speclint is the version of its contract (R1)
	warn, err := compareVersions(contract.Version, sc.ContractVersion)
	if err != nil {
		fail(err)
	}
	if warn != nil {
		issues = append(issues, *warn)
	}
	exclude := ""
	if *baseline != "" {
		exclude = inside(*root, *baseline) // a baseline outside the repository is not in its grep anyway
	}
	code, err := scanRepo(*root, exclude)
	if err != nil {
		fail(err)
	}
	issues = append(issues, checkService(sc, code)...)
	finish(issues, *baseline, *update, out)
}

// inside is path relative to root, as a pathspec of git grep in root, or "" when it is outside root.
func inside(root, path string) string {
	a, err1 := filepath.Abs(root)
	b, err2 := filepath.Abs(path)
	if err1 != nil || err2 != nil {
		return ""
	}
	r, err := filepath.Rel(a, b)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(r)
}
