package agentdocs

import (
	"bytes"
	"fmt"
	"strings"
)

// ParseDocument parses one canonical source file.
func ParseDocument(path string, data []byte) (Source, error) {
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return Source{}, fmt.Errorf("%s: missing front matter start marker", path)
	}

	rest := data[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		return Source{}, fmt.Errorf("%s: missing front matter end marker", path)
	}

	header := string(rest[:end])
	body := rest[end+len("\n---\n"):]
	body = bytes.TrimPrefix(body, []byte("\n"))

	fields := map[string]string{}
	for _, rawLine := range strings.Split(header, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Source{}, fmt.Errorf("%s: malformed front matter line %q", path, line)
		}
		fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	kind := Kind(fields["kind"])
	slug := strings.TrimSpace(fields["slug"])
	title := strings.TrimSpace(fields["title"])
	summary := strings.TrimSpace(fields["summary"])
	if kind == "" {
		return Source{}, fmt.Errorf("%s: missing kind", path)
	}
	if slug == "" {
		return Source{}, fmt.Errorf("%s: missing slug", path)
	}
	if title == "" {
		return Source{}, fmt.Errorf("%s: missing title", path)
	}
	switch kind {
	case KindContract, KindSkill:
	default:
		return Source{}, fmt.Errorf("%s: unsupported kind %q", path, kind)
	}
	if kind == KindSkill && summary == "" {
		return Source{}, fmt.Errorf("%s: missing summary", path)
	}

	return Source{
		Path:    path,
		Kind:    kind,
		Slug:    slug,
		Title:   title,
		Summary: summary,
		Body:    string(body),
	}, nil
}
