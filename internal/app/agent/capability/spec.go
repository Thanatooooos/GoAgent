package capability

import (
	"reflect"
	"strings"
)

const (
	KindTool     = "tool"
	KindWorkflow = "workflow"
	KindSubAgent = "sub_agent"

	NameWebSearch               = "web_search"
	NameWebFetch                = "web_fetch"
	NameExternalEvidenceCollect = "external_evidence_collect"
	NameDocumentInvestigation   = "document_investigation_collect"

	FamilyExternalEvidence      = "external_evidence"
	FamilyDocumentInvestigation = "document_investigation"
	FamilyTraceInvestigation    = "trace_investigation"
	FamilyDiscovery             = "discovery"
	FamilyMeta                  = "meta"

	RoleSearch                  = "search"
	RoleFetch                   = "fetch"
	RoleInvestigateDocument     = "investigate_document"
	RoleInvestigateTrace        = "investigate_trace"
	RoleDiscover                = "discover"
	RoleCollectExternalEvidence = "collect_external_evidence"

	RiskLevelLow    = "low"
	RiskLevelMedium = "medium"
	RiskLevelHigh   = "high"

	IdempotencyUnknown    = "unknown"
	IdempotencyIdempotent = "idempotent"
	IdempotencyBestEffort = "best_effort"

	PreconditionRequirementNonEmpty = "non_empty"
)

// Schema is the minimal Go-runtime-verifiable schema descriptor carried by a capability spec.
type Schema struct {
	GoType string `json:"go_type,omitempty"`
}

// NewSchema describes the supplied Go type in a stable string form.
func NewSchema(sample any) Schema {
	return Schema{GoType: normalizeTypeName(sample)}
}

// Valid reports whether the schema contains a usable runtime type description.
func (s Schema) Valid() bool {
	return strings.TrimSpace(s.GoType) != ""
}

// Precondition describes a caller-visible input/runtime requirement for invoking a capability.
type Precondition struct {
	Field       string `json:"field,omitempty"`
	Requirement string `json:"requirement,omitempty"`
	Description string `json:"description,omitempty"`
}

// Spec is the runtime-managed capability descriptor used by registry, patterns, and handoff projection.
type Spec struct {
	Name             string         `json:"name"`
	Kind             string         `json:"kind"`
	Family           string         `json:"family"`
	Roles            []string       `json:"roles"`
	Description      string         `json:"description,omitempty"`
	InputSchema      Schema         `json:"input_schema"`
	OutputSchema     Schema         `json:"output_schema"`
	RiskLevel        string         `json:"risk_level,omitempty"`
	RequiresApproval bool           `json:"requires_approval,omitempty"`
	SupportsParallel bool           `json:"supports_parallel,omitempty"`
	SupportsResume   bool           `json:"supports_resume,omitempty"`
	Dependencies     []string       `json:"dependencies,omitempty"`
	Preconditions    []Precondition `json:"preconditions,omitempty"`
	ProducesEvidence bool           `json:"produces_evidence,omitempty"`
	Idempotency      string         `json:"idempotency,omitempty"`
}

func normalizeTypeName(sample any) string {
	if sample == nil {
		return ""
	}
	typ := reflect.TypeOf(sample)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.PkgPath() == "" {
		return typ.String()
	}
	return typ.PkgPath() + "." + typ.Name()
}

func isKnownFamily(family string) bool {
	switch strings.TrimSpace(family) {
	case FamilyExternalEvidence, FamilyDocumentInvestigation, FamilyTraceInvestigation, FamilyDiscovery, FamilyMeta:
		return true
	default:
		return false
	}
}

func isKnownRole(role string) bool {
	switch strings.TrimSpace(role) {
	case RoleSearch, RoleFetch, RoleInvestigateDocument, RoleInvestigateTrace, RoleDiscover, RoleCollectExternalEvidence:
		return true
	default:
		return false
	}
}

func isSupportedPreconditionRequirement(requirement string) bool {
	switch strings.TrimSpace(requirement) {
	case PreconditionRequirementNonEmpty:
		return true
	default:
		return false
	}
}
