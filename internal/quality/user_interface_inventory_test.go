package quality_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

type userInterfaceInventory struct {
	Scope                userInterfaceScope      `json:"scope"`
	CLIFlags             []mappedSurface         `json:"cli_flags"`
	EnvironmentVariables []mappedSurface         `json:"environment_variables"`
	ConfigFields         []mappedSurface         `json:"config_fields"`
	Modes                []mappedSurface         `json:"modes"`
	Behaviors            []mappedSurface         `json:"behaviors"`
	ExitCodes            []mappedSurface         `json:"exit_codes"`
	Limitations          []mappedSurface         `json:"limitations"`
	SupportedHelpers     []mappedSurface         `json:"supported_helpers"`
	InternalHelpers      []excludedDocumentation `json:"internal_helpers"`
}

type userInterfaceScope struct {
	Documentation     string `json:"documentation"`
	SupportedSurface  string `json:"supported_surface"`
	InternalExclusion string `json:"internal_exclusion"`
}

type mappedSurface struct {
	ID                 string `json:"id"`
	Source             string `json:"source"`
	SourceToken        string `json:"source_token"`
	Documentation      string `json:"documentation"`
	DocumentationToken string `json:"documentation_token"`
}

func TestUserInterfaceInventoryMatchesCodeAndDocumentation(t *testing.T) {
	root := repoRoot(t)
	inventory := readJSONContract[userInterfaceInventory](t, root, "docs/user-interface.json")
	requireDocumentationFile(t, root, inventory.Scope.Documentation)
	if strings.TrimSpace(inventory.Scope.SupportedSurface) == "" ||
		strings.TrimSpace(inventory.Scope.InternalExclusion) == "" {
		t.Error("user-interface scope must define the supported surface and the internal exclusion")
	}
	validateMappedSurfaces(t, root, "cli_flags", inventory.CLIFlags)
	validateMappedSurfaces(t, root, "environment_variables", inventory.EnvironmentVariables)
	validateMappedSurfaces(t, root, "config_fields", inventory.ConfigFields)
	validateMappedSurfaces(t, root, "modes", inventory.Modes)
	validateMappedSurfaces(t, root, "behaviors", inventory.Behaviors)
	validateMappedSurfaces(t, root, "exit_codes", inventory.ExitCodes)
	validateMappedSurfaces(t, root, "limitations", inventory.Limitations)
	validateMappedSurfaces(t, root, "supported_helpers", inventory.SupportedHelpers)
	requireExactIDs(t, "cli_flags", mappedIDs(inventory.CLIFlags), actualCLIFlags(t, root))
	requireExactIDs(t, "environment_variables", mappedIDs(inventory.EnvironmentVariables), actualEnvironmentVariables(t, root))
	requireExactIDs(t, "config_fields", mappedIDs(inventory.ConfigFields), actualConfigFields(t, root))
	requireExactIDs(t, "modes", mappedIDs(inventory.Modes), []string{
		"delete-a", "delete-aaaa", "delete-both", "delete-disabled", "dry-run", "force-push", "help", "print-effective-config", "reconcile", "validate-config", "version",
	})
	requireExactIDs(t, "behaviors", mappedIDs(inventory.Behaviors), []string{
		"bounded-retries", "cloudflare-scope", "config-precedence", "deployment-security", "package-model", "partial-probe-failure", "post-apply-verification", "probe-and-parse", "reconciliation-plan", "record-scope", "runtime-override-precedence", "scheduler-model", "signal-and-timeout", "token-file-protection", "token-path-override",
	})
	requireExactIDs(t, "exit_codes", mappedIDs(inventory.ExitCodes), []string{"exit-0", "exit-1", "exit-2"})
	requireExactIDs(t, "limitations", mappedIDs(inventory.Limitations), []string{
		"a-and-aaaa-only", "cloudflare-only", "credential-validation", "linux-native-packages", "local-test-doubles", "no-distributed-lock", "no-internal-scheduler", "probe-failure", "workflow-contract-parser",
	})
	internalHelperPaths := make([]string, 0, len(inventory.InternalHelpers))
	for _, entry := range inventory.InternalHelpers {
		internalHelperPaths = append(internalHelperPaths, entry.Path)
		requireDocumentationFile(t, root, entry.Path)
		if strings.TrimSpace(entry.Reason) == "" {
			t.Errorf("internal helper %s must have a reason", entry.Path)
		}
	}
	requireSortedUniquePaths(t, "internal_helpers", internalHelperPaths)
	allHelpers := append(mappedIDs(inventory.SupportedHelpers), internalHelperPaths...)
	slices.Sort(allHelpers)
	requireExactIDs(t, "helper classification", allHelpers, actualHelperSurfaces(t, root))
}

func TestOperatorDocumentationContracts(t *testing.T) {
	root := repoRoot(t)
	readme := mustReadContractFile(t, root, "README.md")
	for _, token := range []string{"- `-version` or `--version` prints", "- `-h`, `--h`, `-help`, or `--help` prints"} {
		if !strings.Contains(readme, token) {
			t.Errorf("README.md introspection list does not contain %q", token)
		}
	}
	aliases := []string{"-version", "--version", "-h", "--h", "-help", "--help"}
	requireDocumentedAliases(t, "docs/FUNCTIONS.md", mustReadContractFile(t, root, "docs/FUNCTIONS.md"), aliases...)

	manual := mustReadContractFile(t, root, "docs/dns-update.1")
	requireDocumentedAliases(t, "docs/dns-update.1 SYNOPSIS", manualSection(t, manual, "SYNOPSIS"), aliases...)
	requireDocumentedAliases(t, "docs/dns-update.1 OPTIONS", manualSection(t, manual, "OPTIONS"), aliases...)
	if !strings.Contains(manualSection(t, manual, "EXIT STATUS"), "requested help") {
		t.Error("docs/dns-update.1 EXIT STATUS does not define the successful help result")
	}
	run := mustReadContractFile(t, root, "internal/app/run.go")
	if normal, force := strings.Index(run, "provider.BuildObservedAddressPlan"), strings.Index(run, "plan = forcePushPlan"); normal < 0 || force < 0 ||
		normal >= force {
		t.Error("force-push does not follow the normal reconciliation plan")
	}
	for _, path := range []string{"README.md", "docs/FUNCTIONS.md", "docs/LIMITATIONS.md", "docs/dns-update.1"} {
		data := mustReadContractFile(t, root, path)
		if !strings.Contains(data, "existing address record") || !strings.Contains(data, "Normal reconciliation creates a missing observed record") {
			t.Errorf("%s does not define the normal-create and force-only boundaries", path)
		}
	}
	script := mustReadContractFile(t, root, "deploy/windows/register-scheduled-task.ps1")
	windows := mustReadContractFile(t, root, "deploy/windows/README.md")
	if !strings.Contains(script, "-Once") || !strings.Contains(script, "-At (Get-Date).AddMinutes(1)") ||
		!strings.Contains(script, "-StartWhenAvailable") || strings.Count(script, "-RepetitionInterval") != 1 ||
		strings.Contains(script, "-AtStartup") || !strings.Contains(windows, "after it misses a scheduled start") ||
		strings.Contains(windows, "when Windows becomes available") {
		t.Error("Windows scheduler documentation does not define the missed-start behavior")
	}
}

func requireDocumentedAliases(t *testing.T, name string, data string, aliases ...string) {
	t.Helper()
	normalized := strings.NewReplacer(`\fB`, " ", `\fI`, " ", `\fR`, " ", `\-`, "-").Replace(data)
	pattern := regexp.MustCompile(`(?:^|[^A-Za-z0-9-])(--?[a-z][a-z0-9-]*)`)
	found := make(map[string]bool)
	for _, match := range pattern.FindAllStringSubmatch(normalized, -1) {
		found[match[1]] = true
	}
	for _, alias := range aliases {
		if !found[alias] {
			t.Errorf("%s does not document %s", name, alias)
		}
	}
}

func manualSection(t *testing.T, data string, name string) string {
	t.Helper()
	data = strings.ReplaceAll(data, "\r\n", "\n")
	parts := strings.SplitN(data, ".SH "+name+"\n", 2)
	if len(parts) != 2 {
		t.Fatalf("manual has no %s section", name)
	}
	return strings.SplitN(parts[1], "\n.SH ", 2)[0]
}

func TestManualSectionHandlesLineEndings(t *testing.T) {
	for _, data := range []string{".SH SYNOPSIS\nbody\n.SH OPTIONS\n", ".SH SYNOPSIS\r\nbody\r\n.SH OPTIONS\r\n"} {
		if got := strings.TrimSpace(manualSection(t, data, "SYNOPSIS")); got != "body" {
			t.Fatalf("manualSection() = %q, want body", got)
		}
	}
}

func validateMappedSurfaces(t *testing.T, root string, category string, entries []mappedSurface) {
	t.Helper()
	ids := mappedIDs(entries)
	requireSortedUniquePaths(t, category, ids)
	for _, entry := range entries {
		requireDocumentationFile(t, root, entry.Source)
		requireDocumentationFile(t, root, entry.Documentation)
		if strings.TrimSpace(entry.SourceToken) == "" || strings.TrimSpace(entry.DocumentationToken) == "" {
			t.Errorf("%s entry %s must have source and documentation tokens", category, entry.ID)
			continue
		}
		source := mustReadContractFile(t, root, entry.Source)
		if !strings.Contains(source, entry.SourceToken) {
			t.Errorf("%s source %s does not contain %q", entry.ID, entry.Source, entry.SourceToken)
		}
		documentation := mustReadContractFile(t, root, entry.Documentation)
		if !strings.Contains(documentation, entry.DocumentationToken) {
			t.Errorf("%s documentation %s does not contain %q",
				entry.ID, entry.Documentation, entry.DocumentationToken)
		}
	}
}

func mappedIDs(entries []mappedSurface) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return ids
}

func requireExactIDs(t *testing.T, category string, got []string, want []string) {
	t.Helper()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("%s mismatch\ngot:  %v\nwant: %v", category, got, want)
	}
}

func actualCLIFlags(t *testing.T, root string) []string {
	t.Helper()
	file := parseGoFile(t, filepath.Join(root, "cmd", "dns-update", "flags.go"))
	var names []string
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel == nil {
			return true
		}
		if selector.Sel.Name != "StringVar" && selector.Sel.Name != "BoolVar" &&
			selector.Sel.Name != "DurationVar" && selector.Sel.Name != "Var" {
			return true
		}
		if len(call.Args) < 2 {
			return true
		}
		literal, ok := call.Args[1].(*ast.BasicLit)
		if ok && literal.Kind == token.STRING {
			names = append(names, strings.Trim(literal.Value, `"`))
		}
		return true
	})
	slices.Sort(names)
	return names
}

func actualEnvironmentVariables(t *testing.T, root string) []string {
	t.Helper()
	pattern := regexp.MustCompile(`DNS_UPDATE_[A-Z0-9_]+`)
	seen := make(map[string]bool)
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
		if !strings.HasSuffix(relative, ".go") || strings.HasSuffix(relative, "_test.go") {
			return nil
		}
		data := mustReadContractFile(t, root, relative)
		for _, name := range pattern.FindAllString(data, -1) {
			seen[name] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func actualConfigFields(t *testing.T, root string) []string {
	t.Helper()
	file := parseGoFile(t, filepath.Join(root, "internal", "config", "types.go"))
	structs := make(map[string]*ast.StructType)
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			typeSpec, ok := specification.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if ok {
				structs[typeSpec.Name.Name] = structType
			}
		}
	}

	var fields []string
	var walk func(string, string)
	walk = func(typeName string, prefix string) {
		structType := structs[typeName]
		for _, field := range structType.Fields.List {
			if field.Tag == nil {
				continue
			}
			tag := strings.Trim(field.Tag.Value, "`")
			jsonName := strings.Split(reflect.StructTag(tag).Get("json"), ",")[0]
			if jsonName == "" || jsonName == "-" {
				continue
			}
			path := jsonName
			if prefix != "" {
				path = prefix + "." + jsonName
			}
			identifier, nested := field.Type.(*ast.Ident)
			if nested && strings.HasPrefix(identifier.Name, "file") {
				walk(identifier.Name, path)
				continue
			}
			fields = append(fields, path)
		}
	}
	walk("fileConfig", "")
	slices.Sort(fields)
	return fields
}

func parseGoFile(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func actualHelperSurfaces(t *testing.T, root string) []string {
	t.Helper()
	paths := []string{"cmd/agentdocgen"}
	for _, directory := range []string{"deploy", "packaging"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			extension := filepath.Ext(path)
			if extension != ".sh" && extension != ".ps1" && extension != ".service" &&
				extension != ".timer" && extension != ".env" {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			paths = append(paths, filepath.ToSlash(relative))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	slices.Sort(paths)
	return paths
}
