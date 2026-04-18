package agentdocs

import (
	"strings"
	"testing"
)

func TestParseDocument(t *testing.T) {
	doc, err := ParseDocument("docs/agents/skills/dns-update-change-gate.md", []byte(strings.TrimLeft(`---
kind: skill
slug: dns-update-change-gate
title: dns-update change gate
summary: Validate a change for correctness, safety, and reviewability before merge.
---

# dns-update change gate

---

Use this playbook before a change lands.
`, "\n")))
	if err != nil {
		t.Fatalf("ParseDocument() error = %v", err)
	}
	if doc.Kind != KindSkill {
		t.Fatalf("Kind = %q, want %q", doc.Kind, KindSkill)
	}
	if doc.Slug != "dns-update-change-gate" {
		t.Fatalf("Slug = %q", doc.Slug)
	}
	if doc.Title != "dns-update change gate" {
		t.Fatalf("Title = %q", doc.Title)
	}
	if doc.Summary != "Validate a change for correctness, safety, and reviewability before merge." {
		t.Fatalf("Summary = %q", doc.Summary)
	}
	if !strings.Contains(doc.Body, "\n---\n") {
		t.Fatalf("Body = %q, want horizontal rule preserved", doc.Body)
	}
	if !strings.Contains(doc.Body, "Use this playbook") {
		t.Fatalf("Body = %q", doc.Body)
	}
}

func TestParseDocumentAcceptsCRLF(t *testing.T) {
	doc, err := ParseDocument("docs/agents/skills/dns-update-change-gate.md", []byte(strings.ReplaceAll(strings.TrimLeft(`---
kind: skill
slug: dns-update-change-gate
title: dns-update change gate
summary: Validate a change for correctness, safety, and reviewability before merge.
---

# dns-update change gate

Use this playbook before a change lands.
`, "\n"), "\n", "\r\n")))
	if err != nil {
		t.Fatalf("ParseDocument() error = %v", err)
	}
	if doc.Title != "dns-update change gate" {
		t.Fatalf("Title = %q", doc.Title)
	}
}

func TestParseDocumentRejectsInvalidFrontMatter(t *testing.T) {
	_, err := ParseDocument("docs/agents/contract.md", []byte("no front matter\n"))
	if err == nil {
		t.Fatal("ParseDocument() error = nil, want error")
	}
}

func TestParseDocumentRejectsUnsupportedKind(t *testing.T) {
	_, err := ParseDocument("docs/agents/contract.md", []byte(strings.TrimLeft(`---
kind: widget
slug: dns-update
title: dns-update repo contract
summary: Keep changes simple.
---

# dns-update repo contract
`, "\n")))
	if err == nil {
		t.Fatal("ParseDocument() error = nil, want error")
	}
}

func TestParseDocumentRejectsMissingTitle(t *testing.T) {
	_, err := ParseDocument("docs/agents/contract.md", []byte(strings.TrimLeft(`---
kind: contract
slug: dns-update
summary: Keep changes simple.
---

# dns-update repo contract
`, "\n")))
	if err == nil {
		t.Fatal("ParseDocument() error = nil, want error")
	}
}

func TestParseDocumentRejectsSkillWithoutSummary(t *testing.T) {
	_, err := ParseDocument("docs/agents/skills/dns-update-change-gate.md", []byte(strings.TrimLeft(`---
kind: skill
slug: dns-update-change-gate
title: dns-update change gate
---

# dns-update change gate
`, "\n")))
	if err == nil {
		t.Fatal("ParseDocument() error = nil, want error")
	}
}
