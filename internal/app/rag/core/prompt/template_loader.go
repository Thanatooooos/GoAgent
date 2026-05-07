package prompt

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

type TemplateLoader struct {
	mu        sync.RWMutex
	templates map[string]string
}

func NewTemplateLoader() *TemplateLoader {
	loader := &TemplateLoader{
		templates: map[string]string{},
	}
	loader.Register(DefaultKBTemplate, defaultKBTemplateContent)
	loader.Register(FallbackKBTemplate, fallbackKBTemplateContent)
	return loader
}

func (l *TemplateLoader) Register(key string, content string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.templates[strings.TrimSpace(key)] = content
}

func (l *TemplateLoader) Load(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("prompt template key is required")
	}

	l.mu.RLock()
	if content, ok := l.templates[key]; ok {
		l.mu.RUnlock()
		return content, nil
	}
	l.mu.RUnlock()

	contentBytes, err := os.ReadFile(key)
	if err != nil {
		return "", fmt.Errorf("load prompt template %q: %w", key, err)
	}
	content := string(contentBytes)

	l.mu.Lock()
	l.templates[key] = content
	l.mu.Unlock()
	return content, nil
}

func (l *TemplateLoader) Render(key string, slots map[string]string) (string, error) {
	template, err := l.Load(key)
	if err != nil {
		return "", err
	}

	rendered := template
	for name, value := range slots {
		rendered = strings.ReplaceAll(rendered, "{{"+name+"}}", value)
	}
	return cleanup(rendered), nil
}

func cleanup(content string) string {
	lines := strings.Split(content, "\n")
	trimmed := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed = append(trimmed, strings.TrimRight(line, " \t"))
	}
	return strings.TrimSpace(strings.Join(trimmed, "\n"))
}
