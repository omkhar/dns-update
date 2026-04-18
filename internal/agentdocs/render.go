package agentdocs

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

type rootTarget string

const (
	rootTargetCodex  rootTarget = "codex"
	rootTargetClaude rootTarget = "claude"
	rootTargetGemini rootTarget = "gemini"
)

// RenderContract converts the canonical contract into provider-specific root docs.
func RenderContract(source Source, skills []Source) []Output {
	return []Output{
		{Path: "AGENTS.md", Content: renderRootMarkdown(source, skills, rootTargetCodex)},
		{Path: "CLAUDE.md", Content: renderRootMarkdown(source, skills, rootTargetClaude)},
		{Path: "GEMINI.md", Content: renderRootMarkdown(source, skills, rootTargetGemini)},
	}
}

// RenderSkill converts one canonical skill into provider-specific projections.
func RenderSkill(source Source) []Output {
	return []Output{
		{Path: path.Join(".agents", "skills", source.Slug, "SKILL.md"), Content: renderSkillMarkdown(source)},
		{Path: path.Join(".claude", "skills", source.Slug, "SKILL.md"), Content: renderSkillMarkdown(source)},
		{Path: geminiCommandPath(source.Slug), Content: renderSkillTOML(source)},
	}
}

func renderRootMarkdown(source Source, skills []Source, target rootTarget) string {
	body := stripLeadingHeading(source.Body)

	var b strings.Builder
	fmt.Fprintf(&b, "<!-- Generated from %s; do not edit directly. -->\n\n", source.Path)
	fmt.Fprintf(&b, "# %s\n\n", source.Title)
	if summary := strings.TrimSpace(source.Summary); summary != "" {
		b.WriteString(summary)
		b.WriteString("\n\n")
	}
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n\n")
	}
	renderRootHelpers(&b, skills, target)
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func renderRootHelpers(b *strings.Builder, skills []Source, target rootTarget) {
	if len(skills) == 0 {
		return
	}

	switch target {
	case rootTargetCodex:
		b.WriteString("## Repo Skills\n\n")
		b.WriteString("Invoke these repo skills when the task matches:\n\n")
	case rootTargetClaude:
		b.WriteString("## Project Skills\n\n")
		b.WriteString("Invoke these project skills when the task matches:\n\n")
	case rootTargetGemini:
		b.WriteString("## Project Commands\n\n")
		b.WriteString("Invoke these project commands when the task matches:\n\n")
	default:
		return
	}

	for _, skill := range skills {
		fmt.Fprintf(b, "- `%s`: %s\n", skillInvocation(skill.Slug, target), skill.Summary)
	}
}

func renderSkillMarkdown(source Source) string {
	body := strings.TrimSpace(source.Body)

	var b strings.Builder
	fmt.Fprintf(&b, "---\nname: %s\ndescription: %s\n---\n", source.Slug, source.Summary)
	fmt.Fprintf(&b, "<!-- Generated from %s; do not edit directly. -->\n\n", source.Path)
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n")
	}
	return b.String()
}

func renderSkillTOML(source Source) string {
	body := strings.TrimSpace(source.Body)

	var b strings.Builder
	fmt.Fprintf(&b, "# Generated from %s; do not edit directly.\n", source.Path)
	fmt.Fprintf(&b, "description = %q\n\n", source.Summary)
	if body != "" {
		body += "\n"
	}
	fmt.Fprintf(&b, "prompt = %s\n", strconv.Quote(body))
	return b.String()
}

func skillInvocation(slug string, target rootTarget) string {
	switch target {
	case rootTargetCodex:
		return "$" + slug
	case rootTargetClaude:
		return "/" + slug
	case rootTargetGemini:
		return "/dns-update:" + geminiCommandLeaf(slug)
	default:
		return slug
	}
}

func geminiCommandPath(slug string) string {
	return path.Join(".gemini", "commands", "dns-update", geminiCommandLeaf(slug)+".toml")
}

func geminiCommandLeaf(slug string) string {
	leaf := strings.TrimPrefix(slug, "dns-update-")
	if leaf == "" {
		return slug
	}
	return leaf
}

func stripLeadingHeading(body string) string {
	body = strings.TrimLeft(body, "\n")
	body = strings.TrimRight(body, "\n")
	if !strings.HasPrefix(body, "# ") {
		return body
	}

	if idx := strings.Index(body, "\n"); idx >= 0 {
		body = strings.TrimLeft(body[idx+1:], "\n")
		return body
	}
	return ""
}
