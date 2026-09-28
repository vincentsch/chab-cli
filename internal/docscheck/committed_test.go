package docscheck

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

// TestCommittedDocsPassSafetyLint scans committed docs so new guide and release
// files inherit the same safety checks by default. A doc that genuinely needs
// external URLs must update the lint policy deliberately in the same pass.
func TestCommittedDocsPassSafetyLint(t *testing.T) {
	repoRoot := findRepoRoot(t)
	docsRoot := filepath.Join(repoRoot, "docs")
	info, err := os.Stat(docsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", docsRoot)
	}

	var violations []string
	for _, rootRel := range []string{"docs", "framework", ".agents", ".claude", ".chab-agent-skill"} {
		rootPath := filepath.Join(repoRoot, filepath.FromSlash(rootRel))
		if info, statErr := os.Stat(rootPath); statErr != nil {
			if os.IsNotExist(statErr) && rootRel != "docs" {
				continue
			}
			t.Fatal(statErr)
		} else if !info.IsDir() {
			t.Fatalf("%s is not a directory", rootPath)
		}
		err = filepath.WalkDir(rootPath, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !entry.Type().IsRegular() || filepath.Ext(path) != ".md" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			for _, violation := range testutil.SafetyViolations(string(data)) {
				violations = append(violations, filepath.ToSlash(rel)+": "+violation)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(violations) != 0 {
		t.Fatalf("docs safety violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestPublicEntryPointsPassSafetyLint(t *testing.T) {
	repoRoot := findRepoRoot(t)
	var violations []string
	for _, rel := range []string{"README.md", "examples/README.md", ".chab-agent-skill/README.md"} {
		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		for _, violation := range testutil.SafetyViolations(string(data)) {
			violations = append(violations, rel+": "+violation)
		}
	}
	if len(violations) != 0 {
		t.Fatalf("public entry point safety violations:\n%s", strings.Join(violations, "\n"))
	}
}

const mcpRestartSentence = "Changing `CHAB_API_KEY` in the host environment requires restarting or relaunching the MCP server process."

func TestAgentSkillEntrypointsAreStandalone(t *testing.T) {
	repoRoot := findRepoRoot(t)
	agentRel := ".agents/skills/chab/SKILL.md"
	claudeRel := ".claude/skills/chab/SKILL.md"
	agentPath := filepath.Join(repoRoot, filepath.FromSlash(agentRel))
	claudePath := filepath.Join(repoRoot, filepath.FromSlash(claudeRel))
	agentData, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatal(err)
	}
	claudeData, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(agentData) != string(claudeData) {
		t.Fatal("agent skill entrypoints are not byte-equivalent")
	}
	text := string(agentData)
	if !strings.HasPrefix(text, "---\nname: chab\ndescription: ") || !strings.Contains(text, "\n---\n") {
		t.Fatalf("skill frontmatter missing name/description:\n%s", text)
	}
	if strings.Contains(text, "../") {
		t.Fatalf("skill entrypoint contains parent-directory reference:\n%s", text)
	}
	for _, ref := range repoRootRelativeSkillPaths() {
		if strings.Contains(text, ref) {
			t.Fatalf("skill entrypoint contains repo-root-relative path %q:\n%s", ref, text)
		}
	}
	assertSkillViltReferencesKnown(t, string(agentData)+"\n"+mustReadString(t, repoRoot, ".chab-agent-skill/README.md")+"\n"+mustReadString(t, repoRoot, ".chab-agent-skill/commands.md"))
}

func TestAgentSkillStandaloneLayouts(t *testing.T) {
	repoRoot := findRepoRoot(t)
	skillText := mustReadString(t, repoRoot, ".agents/skills/chab/SKILL.md")
	for _, test := range []struct {
		name      string
		skillPath func(string) string
	}{
		{
			name: "repository Codex",
			skillPath: func(root string) string {
				return filepath.Join(root, ".agents", "skills", "chab", "SKILL.md")
			},
		},
		{
			name: "user Codex",
			skillPath: func(root string) string {
				return filepath.Join(root, "codex-home", "skills", "chab", "SKILL.md")
			},
		},
		{
			name: "repository Claude",
			skillPath: func(root string) string {
				return filepath.Join(root, ".claude", "skills", "chab", "SKILL.md")
			},
		},
		{
			name: "user Claude",
			skillPath: func(root string) string {
				return filepath.Join(root, "home", ".claude", "skills", "chab", "SKILL.md")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tmp := t.TempDir()
			skillPath := test.skillPath(tmp)
			skillDir := filepath.Dir(skillPath)
			if err := os.MkdirAll(skillDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(skillPath, []byte(skillText), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, ref := range skillLocalReferences(skillText) {
				assertReferenceStaysInsideSkillDir(t, skillDir, ref)
			}
		})
	}
}

func TestSkillAndInstallProseMakesNoPublishedArtifactClaim(t *testing.T) {
	repoRoot := findRepoRoot(t)
	for _, rel := range []string{
		".agents/skills/chab/SKILL.md",
		".claude/skills/chab/SKILL.md",
		".chab-agent-skill/README.md",
	} {
		text := strings.ToLower(mustReadString(t, repoRoot, rel))
		for _, phrase := range []string{"release archive", "published release", "published package"} {
			if strings.Contains(text, phrase) {
				t.Fatalf("%s contains active-user publication claim %q", rel, phrase)
			}
		}
	}

	install := mustReadString(t, repoRoot, "docs/install.md")
	for _, want := range []string{
		"go install ./cmd/chab",
		".agents/skills/chab/SKILL.md",
		"$CODEX_HOME/skills/chab/SKILL.md",
		".claude/skills/chab/SKILL.md",
		"~/.claude/skills/chab/SKILL.md",
	} {
		if !strings.Contains(install, want) {
			t.Fatalf("docs/install.md missing %q:\n%s", want, install)
		}
	}
}

func TestMCPRestartSentenceAppearsOnInstalledSkillSurfaces(t *testing.T) {
	repoRoot := findRepoRoot(t)
	for _, rel := range []string{
		".agents/skills/chab/SKILL.md",
		".claude/skills/chab/SKILL.md",
		"docs/getting-started.md",
	} {
		text := mustReadString(t, repoRoot, rel)
		if !strings.Contains(text, mcpRestartSentence) {
			t.Fatalf("%s missing restart sentence %q", rel, mcpRestartSentence)
		}
	}
}

func TestAgentSkillCommandReferenceMatchesCatalog(t *testing.T) {
	repoRoot := findRepoRoot(t)
	got := mustReadString(t, repoRoot, ".chab-agent-skill/commands.md")
	want := generatedAgentCommandReference()
	if got != want {
		t.Fatalf(".chab-agent-skill/commands.md drifted from catalog")
	}
}

func TestAgentSkillChecksumManifest(t *testing.T) {
	repoRoot := findRepoRoot(t)
	manifestRel := ".chab-agent-skill/manifest.sha256"
	data := mustReadString(t, repoRoot, manifestRel)
	lines := strings.Split(strings.TrimSpace(data), "\n")
	wantFiles := []string{
		".agents/skills/chab/SKILL.md",
		".claude/skills/chab/SKILL.md",
		".chab-agent-skill/README.md",
		".chab-agent-skill/commands.md",
	}
	if len(lines) != len(wantFiles) {
		t.Fatalf("manifest lines = %d, want %d", len(lines), len(wantFiles))
	}
	for i, rel := range wantFiles {
		fields := strings.Fields(lines[i])
		if len(fields) != 2 || fields[1] != rel {
			t.Fatalf("manifest line %d = %q", i+1, lines[i])
		}
		sum := sha256.Sum256([]byte(mustReadString(t, repoRoot, rel)))
		if got := fields[0]; got != hex.EncodeToString(sum[:]) {
			t.Fatalf("%s checksum = %s, want %x", rel, got, sum)
		}
	}
}

func TestCommittedManualDocsPassSafetyLint(t *testing.T) {
	repoRoot := findRepoRoot(t)
	expected := expectedManualDocPaths()
	var violations []string
	for _, rel := range expected {
		data, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		for _, violation := range testutil.SafetyViolations(string(data)) {
			violations = append(violations, rel+": "+violation)
		}
		for _, entry := range cli.Catalog() {
			if entry.Sidecar.Owner != "" && strings.Contains(string(data), entry.Sidecar.Owner) {
				violations = append(violations, rel+": exposes catalog owner "+entry.Sidecar.Owner)
			}
		}
	}
	if len(violations) != 0 {
		t.Fatalf("manual doc safety violations:\n%s", strings.Join(violations, "\n"))
	}
}

// TestReadmeLinksGeneratedManualIndex keeps the generated command reference
// discoverable from the main public entrypoint.
func TestReadmeLinksGeneratedManualIndex(t *testing.T) {
	repoRoot := findRepoRoot(t)
	data, err := os.ReadFile(filepath.Join(repoRoot, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "manual/commands/README.md") {
		t.Fatalf("README.md does not link the generated manual command index")
	}
}

// TestReadmeLinksExistingExampleIndex keeps executable automation guidance
// discoverable and prevents a stale local link from satisfying the guard.
func TestReadmeLinksExistingExampleIndex(t *testing.T) {
	repoRoot := findRepoRoot(t)
	data, err := os.ReadFile(filepath.Join(repoRoot, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	const link = "[examples/README.md](examples/README.md)"
	if !strings.Contains(string(data), link) {
		t.Fatalf("README.md does not link the checked automation example index")
	}
	info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash("examples/README.md")))
	if err != nil {
		t.Fatalf("example index link target: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("example index link target is not a regular file")
	}
}

var (
	// The guard only needs Markdown destinations, not a full Markdown AST. These
	// regexes cover the public-doc link forms used in this repo: inline links,
	// reference definitions, and raw HTML anchors in Markdown files.
	markdownInlineLinkRE = regexp.MustCompile(`\]\(([^)]+)\)`)
	markdownRefDefRE     = regexp.MustCompile(`(?m)^\s*\[[^\]]+\]:\s*(\S+)`)
	markdownHTMLHrefRE   = regexp.MustCompile(`(?i)<a\s+[^>]*\bhref\s*=\s*["']?([^"'\s>]+)`)
	pathShapedTokenRE    = regexp.MustCompile(`(?:^|[\s("'` + "`" + `])([.$~A-Za-z0-9_{}/:-]+/[.$~A-Za-z0-9_{}./:-]+)`)
)

// markdownLinkTargets returns the raw targets of inline links [text](target) and
// reference definitions [ref]: target, plus HTML anchors embedded in Markdown.
// Optional inline titles ([text](t "x")) are dropped by keeping only the first
// whitespace-separated field.
func markdownLinkTargets(text string) []string {
	var targets []string
	for _, m := range markdownInlineLinkRE.FindAllStringSubmatch(text, -1) {
		fields := strings.Fields(m[1])
		if len(fields) == 0 {
			continue
		}
		targets = append(targets, fields[0])
	}
	for _, m := range markdownRefDefRE.FindAllStringSubmatch(text, -1) {
		targets = append(targets, m[1])
	}
	for _, m := range markdownHTMLHrefRE.FindAllStringSubmatch(text, -1) {
		targets = append(targets, m[1])
	}
	return targets
}

func TestMarkdownLinkTargetsAcceptsReferenceDefinitionWithoutWhitespace(t *testing.T) {
	got := markdownLinkTargets("[porting]:docs/boilerplate-porting.md\n")
	if len(got) != 1 || got[0] != "docs/boilerplate-porting.md" {
		t.Fatalf("markdownLinkTargets() = %v, want [docs/boilerplate-porting.md]", got)
	}
}

func TestMarkdownLinkTargetsAcceptsHTMLHref(t *testing.T) {
	got := markdownLinkTargets(`<a class="x" href="docs/boilerplate-porting.md">Porting</a>`)
	if len(got) != 1 || got[0] != "docs/boilerplate-porting.md" {
		t.Fatalf("markdownLinkTargets() = %v, want [docs/boilerplate-porting.md]", got)
	}
}

// TestPortingGuideStaysUnlinked enforces that the maintainer-only porting guide is
// never linked from public docs. It is the inverse of assertLinkTarget: walk every
// public doc and fail if any markdown link resolves to docs/boilerplate-porting.md.
// Walking docs/ and manual/commands/ means a newly added public doc is covered
// automatically; the guide file itself is the one excluded source.
func TestPortingGuideStaysUnlinked(t *testing.T) {
	repoRoot := findRepoRoot(t)
	const guideRel = "docs/boilerplate-porting.md"
	const guideBase = "boilerplate-porting.md"

	// Only public docs are scanned. The maintainer guide itself is excluded so it
	// can point maintainers at supporting private workflow context if needed.
	publicDocs := []string{"README.md", filepath.FromSlash("examples/README.md")}
	for _, dirRel := range []string{"docs", filepath.FromSlash("manual/commands")} {
		dirPath := filepath.Join(repoRoot, dirRel)
		entries, err := os.ReadDir(dirPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			if entry.Name() == guideBase {
				continue // the guide may reference itself; it is the only allowed source.
			}
			publicDocs = append(publicDocs, filepath.Join(dirRel, entry.Name()))
		}
	}

	var violations []string
	for _, rel := range publicDocs {
		full := filepath.Join(repoRoot, rel)
		data, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		for _, target := range markdownLinkTargets(string(data)) {
			clean := strings.Trim(strings.TrimSpace(target), "<>")
			if i := strings.IndexAny(clean, "#?"); i >= 0 {
				clean = clean[:i]
			}
			clean = strings.Trim(clean, "<>")
			if clean == "" || strings.Contains(clean, "://") {
				continue // Empty or absolute URL: cannot target a repo file.
			}
			// Resolve relative links from the source document's directory, then
			// compare against the repo-relative guide path. The basename fallback
			// catches shortened local links such as (boilerplate-porting.md).
			resolved := filepath.Clean(filepath.Join(filepath.Dir(full), filepath.FromSlash(clean)))
			if resolvedRel, err := filepath.Rel(repoRoot, resolved); err == nil &&
				filepath.ToSlash(resolvedRel) == guideRel {
				violations = append(violations, filepath.ToSlash(rel)+" links "+target)
				continue
			}
			if filepath.Base(filepath.FromSlash(clean)) == guideBase {
				violations = append(violations, filepath.ToSlash(rel)+" links "+target)
			}
		}
	}
	if len(violations) != 0 {
		t.Fatalf("public docs must not link the maintainer-only porting guide:\n%s",
			strings.Join(violations, "\n"))
	}
}

func repoRootRelativeSkillPaths() []string {
	return []string{
		"README.md",
		"CLAUDE.md",
		"go.mod",
		"cmd/",
		"docs/",
		"examples/",
		"internal/",
		"manual/",
		"scripts/",
		".agents/",
		".claude/",
		".chab-agent-skill/",
		".vroni/",
	}
}

func skillLocalReferences(text string) []string {
	seen := map[string]bool{}
	var refs []string
	add := func(value string) {
		value = strings.Trim(strings.TrimSpace(value), "`'\"<>()[]{}.,;:")
		if value == "" || strings.Contains(value, "://") || strings.HasPrefix(value, "#") {
			return
		}
		if i := strings.IndexAny(value, "#?"); i >= 0 {
			value = value[:i]
		}
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		refs = append(refs, value)
	}
	for _, target := range markdownLinkTargets(text) {
		add(target)
	}
	for _, match := range pathShapedTokenRE.FindAllStringSubmatch(text, -1) {
		add(match[1])
	}
	sort.Strings(refs)
	return refs
}

func assertReferenceStaysInsideSkillDir(t *testing.T, skillDir, ref string) {
	t.Helper()
	if filepath.IsAbs(ref) {
		t.Fatalf("skill reference %q is absolute", ref)
	}
	clean := filepath.Clean(filepath.FromSlash(ref))
	target := filepath.Clean(filepath.Join(skillDir, clean))
	rel, err := filepath.Rel(skillDir, target)
	if err != nil {
		t.Fatalf("resolve skill reference %q: %v", ref, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		t.Fatalf("skill reference %q resolves outside %s as %s", ref, skillDir, target)
	}
}

func mustReadString(t *testing.T, repoRoot, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

func generatedAgentCommandReference() string {
	var b strings.Builder
	b.WriteString("# Chab Command Reference\n\n")
	b.WriteString("This section is generated from the active command catalog.\n\n")
	b.WriteString("| Command | Summary | Output |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, entry := range cli.Catalog() {
		fmt.Fprintf(&b, "| `chab %s` | %s | %s |\n", entry.Spec.Path, entry.Spec.Summary, agentOutputModes(entry))
	}
	return b.String()
}

func agentOutputModes(entry cli.Entry) string {
	labels := map[cli.OutputMode]string{
		cli.OutputHuman:       "human",
		cli.OutputJSON:        "JSON",
		cli.OutputPlain:       "plain",
		cli.OutputJQ:          "jq",
		cli.OutputTemplate:    "template",
		cli.OutputHelp:        "help",
		cli.OutputShellScript: "shell script",
		cli.OutputMCPStdio:    "MCP stdio",
	}
	var modes []string
	for _, mode := range entry.Spec.OutputModes {
		if label, ok := labels[cli.OutputMode(mode)]; ok {
			modes = append(modes, label)
		} else {
			modes = append(modes, mode)
		}
	}
	for _, mode := range entry.Sidecar.DocOnlyOutputModes {
		if label, ok := labels[mode]; ok {
			modes = append(modes, label)
		} else {
			modes = append(modes, string(mode))
		}
	}
	if len(modes) == 0 {
		if entry.Sidecar.Status == cli.StatusReserved {
			return "none (planned command)"
		}
		return "none (command family)"
	}
	return strings.Join(modes, ", ")
}

func assertSkillViltReferencesKnown(t *testing.T, text string) {
	t.Helper()
	known := map[string]bool{"": true}
	for _, entry := range cli.Catalog() {
		known[entry.Spec.Path] = true
	}
	for _, ref := range viltCommandReferences(text, known) {
		if !known[ref] {
			t.Fatalf("skill references unknown chab command %q", ref)
		}
	}
}

func viltCommandReferences(text string, known map[string]bool) []string {
	var refs []string
	for _, line := range strings.Split(text, "\n") {
		for {
			idx := strings.Index(line, "chab ")
			if idx < 0 {
				break
			}
			ref, ok := parseViltReference(line[idx:], known)
			if ok {
				refs = append(refs, ref)
			}
			line = line[idx+len("chab "):]
		}
	}
	sort.Strings(refs)
	return refs
}

func parseViltReference(text string, known map[string]bool) (string, bool) {
	fields := strings.Fields(strings.Trim(text, "`"))
	if len(fields) == 0 || fields[0] != "chab" {
		return "", false
	}
	args := normalizeViltArgs(fields[1:])
	if len(args) > 0 && args[0] == "chab" {
		args = args[1:]
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		return "", true
	}
	paths := make([]string, 0, len(known))
	for path := range known {
		if path != "" {
			paths = append(paths, path)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		left := len(strings.Fields(paths[i]))
		right := len(strings.Fields(paths[j]))
		if left == right {
			return paths[i] < paths[j]
		}
		return left > right
	})
	for _, path := range paths {
		pathFields := strings.Fields(path)
		if len(pathFields) > len(args) {
			continue
		}
		matches := true
		for i, field := range pathFields {
			if args[i] != field {
				matches = false
				break
			}
		}
		if matches {
			return path, true
		}
	}
	return strings.Join(args, " "), true
}

func normalizeViltArgs(args []string) []string {
	valueFlags := map[string]bool{
		"--profile":      true,
		"--config":       true,
		"--auth-file":    true,
		"--base-url":     true,
		"--api-base-url": true,
		"--locale":       true,
		"--jq":           true,
		"--template":     true,
	}
	var out []string
	for i := 0; i < len(args); i++ {
		field := strings.Trim(args[i], "`'\".,")
		if field == "" {
			continue
		}
		if strings.HasPrefix(field, "--") {
			if valueFlags[field] && i+1 < len(args) {
				i++
			}
			continue
		}
		out = append(out, field)
	}
	return out
}

// findRepoRoot lets this test-only package run from any package working
// directory selected by `go test ./...`.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root")
		}
		dir = parent
	}
}

// expectedManualDocPaths derives the committed manual-doc set from the catalog
// so docscheck fails when a command row lacks a generated page.
func expectedManualDocPaths() []string {
	paths := []string{"manual/commands/README.md"}
	for _, entry := range cli.Catalog() {
		paths = append(paths, entry.Sidecar.DocPath)
	}
	return paths
}
