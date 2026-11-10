package starter

import (
	"bytes"
	"embed"
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

// The files of the check in CI that Hammurapi proposes to repositories (FTR.HMR.SPEC-0002 R6, R11, arch §3.6).
const (
	// CIMarker is the first line of a file that Hammurapi manages: it changes only its version line.
	CIMarker = "# Managed by Hammurapi (FTR.HMR.SPEC-0002): only the version line is updated."
	// GitHubCIPath is the workflow of the check in a GitHub repository.
	GitHubCIPath = ".github/workflows/speclint.yml"
	// GitLabCIPath is the file of the check in a GitLab repository; .gitlab-ci.yml includes it as local.
	GitLabCIPath = ".gitlab/speclint.gitlab-ci.yml"
	// GitLabRootCI is the pipeline of a GitLab repository.
	GitLabRootCI = ".gitlab-ci.yml"
)

//go:embed ci
var ciFiles embed.FS

// CIParams fill a file of the check.
type CIParams struct {
	Version   string // the version of speclint, the version of the contract: 1.2.0
	Mode      string // spec | service
	Hammurapi string // the public address of the API of Hammurapi, for service
	Service   string // the key of the service in the catalog, for service
	Branch    string // the default branch, for GitHub
}

var safeValue = regexp.MustCompile(`^[A-Za-z0-9._:/?=&%~+-]*$`)

// CIFile is the path and the content of the file of the check for a provider: github or gitlab.
func CIFile(provider string, p CIParams) (string, []byte, error) {
	name, path := "", ""
	switch provider {
	case "github":
		name, path = "ci/github.yml", GitHubCIPath
	case "gitlab":
		name, path = "ci/gitlab.yml", GitLabCIPath
	default:
		return "", nil, fmt.Errorf("unknown provider %q", provider)
	}
	if p.Mode != "spec" && p.Mode != "service" {
		return "", nil, fmt.Errorf("unknown mode %q", p.Mode)
	}
	if _, ok := ParseVersion(p.Version); !ok {
		return "", nil, fmt.Errorf("bad version %q", p.Version)
	}
	if p.Mode == "service" && (p.Hammurapi == "" || p.Service == "") {
		return "", nil, fmt.Errorf("the mode service needs the address of Hammurapi and the key of the service")
	}
	if p.Branch == "" {
		p.Branch = "main"
	}
	// the values go into YAML strings: no quotes, spaces or line breaks
	for _, v := range []string{p.Hammurapi, p.Service, p.Branch} {
		if !safeValue.MatchString(v) {
			return "", nil, fmt.Errorf("unsafe value %q for a file of CI", v)
		}
	}
	src, err := ciFiles.ReadFile(name)
	if err != nil {
		return "", nil, err
	}
	t, err := template.New(name).Delims("[[", "]]").Parse(string(src))
	if err != nil {
		return "", nil, err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, p); err != nil {
		return "", nil, err
	}
	return path, b.Bytes(), nil
}

// ciVersionRe is the version line of a file of the check: the action of GitHub or the variable of GitLab.
var ciVersionRe = regexp.MustCompile(`(?m)^(\s*-?\s*uses:\s*GreenOnGrey/speclint@v|\s*SPECLINT_VERSION:\s*")(\d+\.\d+\.\d+)`)

// CIVersion reads a file of the check: whether Hammurapi manages it (the marker in the first line) and its
// version; a file without the marker or with no single version line is managed by people (status manual).
func CIVersion(content []byte) (version string, managed bool) {
	s := string(content)
	first, _, _ := strings.Cut(s, "\n")
	if strings.TrimSpace(first) != CIMarker {
		return "", false
	}
	m := ciVersionRe.FindAllStringSubmatch(s, -1)
	if len(m) != 1 {
		return "", false
	}
	return m[0][2], true
}

// SetCIVersion changes only the version line of a managed file.
func SetCIVersion(content []byte, version string) ([]byte, error) {
	if _, ok := CIVersion(content); !ok {
		return nil, fmt.Errorf("the file is not managed by Hammurapi")
	}
	if _, ok := ParseVersion(version); !ok {
		return nil, fmt.Errorf("bad version %q", version)
	}
	return ciVersionRe.ReplaceAll(content, []byte("${1}"+version)), nil
}

// CompareVersions compares two versions x.y.z: -1, 0 or 1; ok is false when one of them is not a version.
func CompareVersions(a, b string) (int, bool) {
	x, ok1 := ParseVersion(a)
	y, ok2 := ParseVersion(b)
	if !ok1 || !ok2 {
		return 0, false
	}
	for i := range x {
		if x[i] != y[i] {
			if x[i] < y[i] {
				return -1, true
			}
			return 1, true
		}
	}
	return 0, true
}

// ParseVersion reads a version x.y.z of the contract; a pre-release or a v prefix is not one.
func ParseVersion(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n := 0
		if p == "" || len(p) > 6 {
			return out, false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return out, false
			}
			n = n*10 + int(c-'0')
		}
		out[i] = n
	}
	return out, true
}
