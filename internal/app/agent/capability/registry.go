package capability

import (
	"fmt"
	"sort"
	"strings"
)

// Registry is the unified capability catalog used by runtime patterns.
type Registry struct {
	entries  map[string]Handle
	specs    map[string]Spec
	byRole   map[string][]string
	byFamily map[string][]string
}

// NewRegistry creates an empty capability registry.
func NewRegistry() *Registry {
	return &Registry{
		entries:  make(map[string]Handle),
		specs:    make(map[string]Spec),
		byRole:   make(map[string][]string),
		byFamily: make(map[string][]string),
	}
}

// Register adds a named capability handle to the registry.
func (r *Registry) Register(handle Handle) error {
	if r == nil {
		return fmt.Errorf("capability registry is not initialized")
	}
	if handle == nil {
		return fmt.Errorf("capability handle is required")
	}

	spec, name, err := normalizeSpec(handle.Spec())
	if err != nil {
		return err
	}
	if _, exists := r.entries[name]; exists {
		return fmt.Errorf("capability %q is already registered", name)
	}
	for _, dependency := range spec.Dependencies {
		if _, exists := r.entries[dependency]; !exists {
			return fmt.Errorf("capability %q dependency %q is not registered", name, dependency)
		}
	}

	r.entries[name] = handle
	r.specs[name] = spec
	for _, role := range spec.Roles {
		r.byRole[role] = appendUniqueSorted(r.byRole[role], name)
	}
	r.byFamily[spec.Family] = appendUniqueSorted(r.byFamily[spec.Family], name)
	return nil
}

// Handle returns the named capability handle.
func (r *Registry) Handle(name string) (Handle, error) {
	if r == nil {
		return nil, fmt.Errorf("capability registry is not initialized")
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, fmt.Errorf("capability name is required")
	}
	handle, ok := r.entries[trimmed]
	if !ok {
		return nil, fmt.Errorf("capability %q is not registered", trimmed)
	}
	return handle, nil
}

// Spec returns the registered capability spec by name.
func (r *Registry) Spec(name string) (Spec, bool) {
	if r == nil {
		return Spec{}, false
	}
	spec, ok := r.specs[strings.TrimSpace(name)]
	return spec, ok
}

// Specs returns all registered capability specs in stable name order.
func (r *Registry) Specs() []Spec {
	if r == nil || len(r.specs) == 0 {
		return nil
	}
	names := make([]string, 0, len(r.specs))
	for name := range r.specs {
		names = append(names, name)
	}
	sort.Strings(names)

	specs := make([]Spec, 0, len(names))
	for _, name := range names {
		specs = append(specs, r.specs[name])
	}
	return specs
}

// NamesByRole returns registered capability names that implement the given role.
func (r *Registry) NamesByRole(role string) []string {
	if r == nil {
		return nil
	}
	return append([]string(nil), r.byRole[strings.TrimSpace(role)]...)
}

// NamesByFamily returns registered capability names that belong to the given family.
func (r *Registry) NamesByFamily(family string) []string {
	if r == nil {
		return nil
	}
	return append([]string(nil), r.byFamily[strings.TrimSpace(family)]...)
}

func normalizeSpec(spec Spec) (Spec, string, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return Spec{}, "", fmt.Errorf("capability spec name is required")
	}
	spec.Name = name
	spec.Kind = normalizeKind(spec.Kind)
	if spec.Kind == "" {
		return Spec{}, "", fmt.Errorf("capability spec kind is required for %q", name)
	}
	spec.Family = strings.TrimSpace(spec.Family)
	if spec.Family == "" {
		return Spec{}, "", fmt.Errorf("capability spec family is required for %q", name)
	}
	spec.Roles = normalizeNames(spec.Roles)
	if len(spec.Roles) == 0 {
		return Spec{}, "", fmt.Errorf("capability spec roles are required for %q", name)
	}
	spec.Description = strings.TrimSpace(spec.Description)
	if spec.Description == "" {
		return Spec{}, "", fmt.Errorf("capability spec description is required for %q", name)
	}
	if !spec.InputSchema.Valid() {
		return Spec{}, "", fmt.Errorf("capability spec input schema is required for %q", name)
	}
	if !spec.OutputSchema.Valid() {
		return Spec{}, "", fmt.Errorf("capability spec output schema is required for %q", name)
	}
	spec.RiskLevel = normalizeRiskLevel(spec.RiskLevel)
	spec.Dependencies = normalizeNames(spec.Dependencies)
	spec.Preconditions = normalizePreconditions(spec.Preconditions)
	spec.Idempotency = normalizeIdempotency(spec.Idempotency)
	for _, dependency := range spec.Dependencies {
		if dependency == name {
			return Spec{}, "", fmt.Errorf("capability %q cannot depend on itself", name)
		}
	}
	return spec, name, nil
}

func specHasRole(spec Spec, role string) bool {
	trimmedRole := strings.TrimSpace(role)
	for _, declared := range spec.Roles {
		if declared == trimmedRole {
			return true
		}
	}
	return false
}

func normalizeKind(kind string) string {
	switch trimmed := strings.TrimSpace(kind); trimmed {
	case KindTool, KindWorkflow, KindSubAgent:
		return trimmed
	default:
		return ""
	}
}

func normalizeRiskLevel(level string) string {
	switch trimmed := strings.TrimSpace(level); trimmed {
	case "", RiskLevelLow:
		return RiskLevelLow
	case RiskLevelMedium:
		return RiskLevelMedium
	case RiskLevelHigh:
		return RiskLevelHigh
	default:
		return trimmed
	}
}

func normalizeIdempotency(level string) string {
	switch trimmed := strings.TrimSpace(level); trimmed {
	case "", IdempotencyUnknown:
		return IdempotencyUnknown
	case IdempotencyIdempotent:
		return IdempotencyIdempotent
	case IdempotencyBestEffort:
		return IdempotencyBestEffort
	default:
		return trimmed
	}
}

func normalizeNames(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func appendUniqueSorted(existing []string, name string) []string {
	for _, item := range existing {
		if item == name {
			return existing
		}
	}
	existing = append(existing, name)
	sort.Strings(existing)
	return existing
}

func normalizePreconditions(values []Precondition) []Precondition {
	if len(values) == 0 {
		return nil
	}
	result := make([]Precondition, 0, len(values))
	for _, value := range values {
		field := strings.TrimSpace(value.Field)
		requirement := strings.TrimSpace(value.Requirement)
		description := strings.TrimSpace(value.Description)
		if field == "" && requirement == "" && description == "" {
			continue
		}
		result = append(result, Precondition{
			Field:       field,
			Requirement: requirement,
			Description: description,
		})
	}
	return result
}
