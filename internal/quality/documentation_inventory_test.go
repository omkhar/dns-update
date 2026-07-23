package quality_test

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const steDeclaration = "ASD-STE100 Simplified Technical English"
const maximumSimplifiedEnglishSentenceWords = 25

var (
	inlineCodePattern  = regexp.MustCompile("`[^`]*`")
	linkPattern        = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	urlPattern         = regexp.MustCompile(`https?://\S+`)
	wordPattern        = regexp.MustCompile(`[[:alnum:]]+(?:[-'][[:alnum:]]+)*`)
	sentencePattern    = regexp.MustCompile(`[^.!?]+[.!?]?`)
	tableRulePattern   = regexp.MustCompile(`^[-:| ]+$`)
	orderedListPattern = regexp.MustCompile(`^[0-9]+[.] `)
	roffFontPattern    = regexp.MustCompile(`\\f[BRI]`)
	passivePattern     = regexp.MustCompile(
		`(?i)\b(?:am|are|is|was|were|be|been|being)\s+(?:not\s+)?(?:[a-z]+ly\s+)*` +
			`(?:[a-z]{3,}ed|bent|bound|broken|built|bought|caught|chosen|done|driven|` +
			`found|given|held|kept|known|left|lost|made|meant|put|read|run|seen|sent|` +
			`set|shown|spoken|taken|told|written)\b`,
	)
	yamlKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+:\s*(?:[>|][+-]?)?\s*`)
	tomlKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+\s*=\s*`)
)

type documentationInventory struct {
	STE               []string                        `json:"ste"`
	Generated         []generatedDocumentation        `json:"generated"`
	Structured        []excludedDocumentation         `json:"structured"`
	Verbatim          []excludedDocumentation         `json:"verbatim"`
	SectionExclusions []documentationSectionExclusion `json:"section_exclusions"`
}

type generatedDocumentation struct {
	Path   string `json:"path"`
	Source string `json:"source"`
}

type excludedDocumentation struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type documentationSectionExclusion struct {
	Path    string `json:"path"`
	Section string `json:"section"`
	Reason  string `json:"reason"`
}

func TestDocumentationInventoryCoversLiveSurfaces(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	inventory := readDocumentationInventory(t, root)
	classified := make(map[string]string)

	requireSortedUniquePaths(t, "ste", inventory.STE)
	for _, path := range inventory.STE {
		classifyDocumentationPath(t, root, classified, path, "ste")
		data := mustReadContractFile(t, root, path)
		if !strings.Contains(data, steDeclaration) {
			t.Errorf("%s does not declare %s", path, steDeclaration)
		}
		requireSimplifiedEnglishStyle(t, path, data)
	}

	generatedPaths := make([]string, 0, len(inventory.Generated))
	for _, entry := range inventory.Generated {
		generatedPaths = append(generatedPaths, entry.Path)
		classifyDocumentationPath(t, root, classified, entry.Path, "generated")
		requireDocumentationFile(t, root, entry.Source)
		data := mustReadContractFile(t, root, entry.Path)
		if !strings.Contains(data, steDeclaration) {
			t.Errorf("%s does not declare %s", entry.Path, steDeclaration)
		}
		requireSimplifiedEnglishStyle(t, entry.Path, data)
	}
	requireSortedUniquePaths(t, "generated", generatedPaths)

	validateExcludedDocumentation(t, root, classified, "structured", inventory.Structured)
	validateExcludedDocumentation(t, root, classified, "verbatim", inventory.Verbatim)

	sectionPaths := make([]string, 0, len(inventory.SectionExclusions))
	for _, entry := range inventory.SectionExclusions {
		sectionPaths = append(sectionPaths, entry.Path+":"+entry.Section)
		requireDocumentationFile(t, root, entry.Path)
		if strings.TrimSpace(entry.Section) == "" || strings.TrimSpace(entry.Reason) == "" {
			t.Errorf("section exclusion for %s must have a section and a reason", entry.Path)
		}
	}
	requireSortedUniquePaths(t, "section_exclusions", sectionPaths)

	discovered, err := discoverDocumentation(root)
	if err != nil {
		t.Fatal(err)
	}
	classifiedPaths := make([]string, 0, len(classified))
	for path := range classified {
		classifiedPaths = append(classifiedPaths, path)
	}
	slices.Sort(classifiedPaths)
	if !slices.Equal(classifiedPaths, discovered) {
		t.Errorf("documentation inventory does not match live surfaces\nclassified: %v\ndiscovered: %v",
			classifiedPaths, discovered)
	}
}

func TestDocumentationPathUsesRepositorySlashSemantics(t *testing.T) {
	t.Parallel()

	for _, candidate := range []string{
		"",
		".",
		"../docs/FUNCTIONS.md",
		"./docs/FUNCTIONS.md",
		"/docs/FUNCTIONS.md",
		"C:/docs/FUNCTIONS.md",
		`docs\FUNCTIONS.md`,
		"docs/../FUNCTIONS.md",
	} {
		if isCleanDocumentationPath(candidate) {
			t.Errorf("isCleanDocumentationPath(%q) = true, want false", candidate)
		}
	}

	if !isCleanDocumentationPath("docs/FUNCTIONS.md") {
		t.Error(`isCleanDocumentationPath("docs/FUNCTIONS.md") = false, want true`)
	}
}

func TestSimplifiedEnglishGrammarFindings(t *testing.T) {
	t.Parallel()

	long := strings.TrimSpace(strings.Repeat("word ", 26)) + "."
	technical := "Use `the package is signed by a key with more than twenty-five technical words " +
		"one two three four five six seven eight nine ten eleven twelve thirteen fourteen`.\n\n" +
		"```text\nThe package is signed by the maintainer.\n```\n" +
		"~~~text\nThe package is signed by the maintainer.\n~~~\n"
	for _, test := range []struct {
		name string
		data string
		want string
	}{
		{"passive voice", "The release is signed by the maintainer.", "passive voice"},
		{"long sentence", long, "26 words"},
		{"wrapped list", "- " + strings.Repeat("word ", 13) + "\n  " + long[13*5:], "26 words"},
		{"technical syntax", technical, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			findings := simplifiedEnglishGrammarFindings(test.data)
			if test.want == "" && len(findings) != 0 {
				t.Fatalf("findings = %v, want none", findings)
			}
			if test.want != "" && !slices.ContainsFunc(findings, func(finding string) bool {
				return strings.Contains(finding, test.want)
			}) {
				t.Fatalf("findings = %v, want %q", findings, test.want)
			}
		})
	}
}

func simplifiedEnglishGrammarFindings(data string) []string {
	var findings []string
	for _, sentence := range simplifiedEnglishSentences(data) {
		words := wordPattern.FindAllString(sentence, -1)
		if len(words) > maximumSimplifiedEnglishSentenceWords {
			findings = append(findings,
				fmt.Sprintf("sentence has %d words: %q", len(words), sentence))
		}
		if construction := passivePattern.FindString(sentence); construction != "" {
			findings = append(findings,
				fmt.Sprintf("passive voice %q in %q", construction, sentence))
		}
	}
	return findings
}

func simplifiedEnglishSentences(data string) []string {
	data = strings.ReplaceAll(documentationProse(data), `\n`, "\n")
	var prose strings.Builder

	for line := range strings.SplitSeq(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "---" {
			prose.WriteByte('\n')
			continue
		}
		if strings.HasPrefix(trimmed, ".") {
			if strings.HasPrefix(trimmed, ".SH ") || trimmed == ".TP" ||
				trimmed == ".P" || strings.HasPrefix(trimmed, ".TH ") {
				prose.WriteByte('\n')
			}
			continue
		}

		startNew := strings.HasPrefix(trimmed, "- ") ||
			strings.HasPrefix(trimmed, "* ") ||
			strings.HasPrefix(trimmed, "|") ||
			orderedListPattern.MatchString(trimmed) ||
			yamlKeyPattern.MatchString(trimmed) ||
			tomlKeyPattern.MatchString(trimmed)
		switch {
		case strings.HasPrefix(trimmed, "#"):
			prose.WriteByte('\n')
			continue
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* "):
			trimmed = strings.TrimSpace(trimmed[2:])
		case strings.HasPrefix(trimmed, "|"):
			trimmed = strings.Trim(trimmed, "| ")
		case orderedListPattern.MatchString(trimmed):
			trimmed = orderedListPattern.ReplaceAllString(trimmed, "")
		}
		trimmed = yamlKeyPattern.ReplaceAllString(trimmed, "")
		trimmed = tomlKeyPattern.ReplaceAllString(trimmed, "")

		trimmed = sanitizeSimplifiedEnglishText(trimmed)
		if startNew {
			prose.WriteByte('\n')
		}
		if trimmed == "" || tableRulePattern.MatchString(trimmed) {
			continue
		}
		prose.WriteByte(' ')
		prose.WriteString(trimmed)
	}

	var sentences []string
	for paragraph := range strings.SplitSeq(prose.String(), "\n") {
		for _, match := range sentencePattern.FindAllString(paragraph, -1) {
			if sentence := strings.TrimSpace(match); sentence != "" {
				sentences = append(sentences, sentence)
			}
		}
	}
	return sentences
}

func sanitizeSimplifiedEnglishText(value string) string {
	value = linkPattern.ReplaceAllString(value, "$1")
	value = inlineCodePattern.ReplaceAllString(value, " ")
	value = urlPattern.ReplaceAllString(value, " ")
	value = roffFontPattern.ReplaceAllString(value, "")
	value = strings.Trim(value, `"'`)
	value = strings.NewReplacer("**", "", "__", "", "~~", "").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func requireSimplifiedEnglishStyle(t *testing.T, path string, data string) {
	t.Helper()

	prose := strings.Join(simplifiedEnglishSentences(data), "\n")
	if strings.Contains(prose, ";") {
		t.Errorf("%s contains a semicolon", path)
	}

	lower := strings.ToLower(prose)
	for _, term := range []string{
		"can't",
		"couldn't",
		"didn't",
		"doesn't",
		"don't",
		"hasn't",
		"haven't",
		"isn't",
		"shouldn't",
		"they're",
		"we're",
		"won't",
		"wouldn't",
		"you're",
	} {
		if strings.Contains(lower, term) {
			t.Errorf("%s contains contraction %q", path, term)
		}
	}
	for _, term := range []string{" please ", " shall ", " should "} {
		if strings.Contains(" "+lower+" ", term) {
			t.Errorf("%s contains non-imperative term %q", path, strings.TrimSpace(term))
		}
	}
	for _, finding := range simplifiedEnglishGrammarFindings(data) {
		t.Errorf("%s contains %s", path, finding)
	}
}

func documentationProse(data string) string {
	var prose strings.Builder
	inFence := false
	for line := range strings.SplitSeq(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence {
			prose.WriteString(line)
			prose.WriteByte('\n')
		}
	}
	return prose.String()
}

func readDocumentationInventory(t *testing.T, root string) documentationInventory {
	t.Helper()

	file, err := os.Open(filepath.Join(root, "docs", "documentation-inventory.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = file.Close()
	}()

	var inventory documentationInventory
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&inventory); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("documentation inventory must contain one JSON value: %v", err)
	}
	return inventory
}

func validateExcludedDocumentation(
	t *testing.T,
	root string,
	classified map[string]string,
	category string,
	entries []excludedDocumentation,
) {
	t.Helper()

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
		classifyDocumentationPath(t, root, classified, entry.Path, category)
		if strings.TrimSpace(entry.Reason) == "" {
			t.Errorf("%s entry %s must have a reason", category, entry.Path)
		}
	}
	requireSortedUniquePaths(t, category, paths)
}

func classifyDocumentationPath(
	t *testing.T,
	root string,
	classified map[string]string,
	path string,
	category string,
) {
	t.Helper()

	requireDocumentationFile(t, root, path)
	if previous, ok := classified[path]; ok {
		t.Errorf("%s is in both %s and %s", path, previous, category)
		return
	}
	classified[path] = category
}

func requireDocumentationFile(t *testing.T, root string, path string) {
	t.Helper()

	if !isCleanDocumentationPath(path) {
		t.Errorf("documentation path %q is not a clean relative path", path)
		return
	}
	info, err := os.Stat(filepath.Join(root, path))
	if err != nil {
		t.Errorf("documentation path %s: %v", path, err)
		return
	}
	if !info.Mode().IsRegular() {
		t.Errorf("documentation path %s is not a regular file", path)
	}
}

func isCleanDocumentationPath(value string) bool {
	if value == "" || value == "." || strings.Contains(value, `\`) ||
		pathpkg.IsAbs(value) || isWindowsSlashAbsolutePath(value) ||
		value == ".." || strings.HasPrefix(value, "../") {
		return false
	}
	return pathpkg.Clean(value) == value
}

func isWindowsSlashAbsolutePath(value string) bool {
	if len(value) < 3 || value[1] != ':' || value[2] != '/' {
		return false
	}
	return value[0] >= 'A' && value[0] <= 'Z' || value[0] >= 'a' && value[0] <= 'z'
}

func requireSortedUniquePaths(t *testing.T, category string, paths []string) {
	t.Helper()

	if !slices.IsSorted(paths) {
		t.Errorf("%s paths are not sorted: %v", category, paths)
	}
	for index := 1; index < len(paths); index++ {
		if paths[index] == paths[index-1] {
			t.Errorf("%s has duplicate path %s", category, paths[index])
		}
	}
}

func discoverDocumentation(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if relative == ".git" || relative == "out" {
				return filepath.SkipDir
			}
			return nil
		}
		if isDocumentationSurface(relative) {
			paths = append(paths, relative)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover documentation: %w", err)
	}
	slices.Sort(paths)
	return paths, nil
}

func isDocumentationSurface(path string) bool {
	extension := filepath.Ext(path)
	if extension == ".md" || extension == ".markdown" || extension == ".rst" ||
		extension == ".adoc" || extension == ".txt" || extension == ".1" {
		return true
	}
	if strings.HasPrefix(filepath.Base(path), "README") {
		return true
	}
	if path == "LICENSE" || path == "debian/changelog" ||
		path == "cloudflare.token.example" ||
		path == "config.example.json" ||
		path == "debian/copyright" {
		return true
	}
	if strings.HasPrefix(path, "docs/") && extension == ".json" {
		return true
	}
	if strings.HasPrefix(path, ".github/ISSUE_TEMPLATE/") &&
		(extension == ".yml" || extension == ".yaml") {
		return true
	}
	return strings.HasPrefix(path, ".gemini/commands/") && extension == ".toml"
}
