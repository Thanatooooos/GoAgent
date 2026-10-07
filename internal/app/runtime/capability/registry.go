package capability

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Registry struct{ defs map[string]Def }

func NewRegistry() *Registry { return &Registry{defs: make(map[string]Def)} }

func (r *Registry) Register(def Def) error {
	if r == nil {
		return fmt.Errorf("capability registry is required")
	}
	def.ID = strings.TrimSpace(def.ID)
	def.Description = strings.TrimSpace(def.Description)
	if def.ID == "" || def.Description == "" || !jsonSchemaValid(def.JSONSchema) || def.Validate == nil || def.Describe == nil || def.Execute == nil {
		return fmt.Errorf("capability definition %q is incomplete", def.ID)
	}
	if _, exists := r.defs[def.ID]; exists {
		return fmt.Errorf("capability %q already registered", def.ID)
	}
	r.defs[def.ID] = def
	return nil
}

func (r *Registry) Get(id string) (Def, bool) {
	if r == nil {
		return Def{}, false
	}
	def, ok := r.defs[strings.TrimSpace(id)]
	return def, ok
}

func (r *Registry) List() []ModelDefinition {
	if r == nil {
		return []ModelDefinition{}
	}
	items := make([]ModelDefinition, 0, len(r.defs))
	for _, def := range r.defs {
		items = append(items, ModelDefinition{ID: def.ID, Description: def.Description, JSONSchema: append([]byte(nil), def.JSONSchema...)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

// Prepare validates model arguments and describes the operation, but performs
// no external action. Runtime can journal pending before Execute is called.
func (r *Registry) Prepare(id string, args Value, ctx Context) (Def, Operation, error) {
	def, ok := r.Get(id)
	if !ok {
		return Def{}, Operation{}, fmt.Errorf("capability %q is not registered", strings.TrimSpace(id))
	}
	if err := def.Validate(args); err != nil {
		return Def{}, Operation{}, fmt.Errorf("validate capability %q: %w", def.ID, err)
	}
	op, err := def.Describe(args, ctx)
	if err != nil {
		return Def{}, Operation{}, fmt.Errorf("describe capability %q: %w", def.ID, err)
	}
	return def, op, nil
}

func jsonSchemaValid(schema Value) bool {
	return len(schema) > 0 && json.Valid(schema)
}
