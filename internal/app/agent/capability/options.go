package capability

import "strings"

// Option mutates capability spec defaults during capability construction.
type Option func(*Spec)

// WithName overrides the default capability name.
func WithName(name string) Option {
	return func(spec *Spec) {
		if spec == nil {
			return
		}
		spec.Name = strings.TrimSpace(name)
	}
}

// WithDescription overrides the default description.
func WithDescription(description string) Option {
	return func(spec *Spec) {
		if spec == nil {
			return
		}
		spec.Description = strings.TrimSpace(description)
	}
}

// WithInputSchema overrides the default input schema for a capability spec.
func WithInputSchema(schema Schema) Option {
	return func(spec *Spec) {
		if spec == nil {
			return
		}
		spec.InputSchema = schema
	}
}

// WithOutputSchema overrides the default output schema for a capability spec.
func WithOutputSchema(schema Schema) Option {
	return func(spec *Spec) {
		if spec == nil {
			return
		}
		spec.OutputSchema = schema
	}
}

// WithRiskLevel overrides the default risk level for a capability spec.
func WithRiskLevel(level string) Option {
	return func(spec *Spec) {
		if spec == nil {
			return
		}
		spec.RiskLevel = strings.TrimSpace(level)
	}
}

// WithRequiresApproval marks a capability spec as approval-gated.
func WithRequiresApproval(required bool) Option {
	return func(spec *Spec) {
		if spec == nil {
			return
		}
		spec.RequiresApproval = required
	}
}

// WithSupportsParallel overrides the default parallel-execution capability flag.
func WithSupportsParallel(enabled bool) Option {
	return func(spec *Spec) {
		if spec == nil {
			return
		}
		spec.SupportsParallel = enabled
	}
}

// WithSupportsResume overrides the default resume capability flag.
func WithSupportsResume(enabled bool) Option {
	return func(spec *Spec) {
		if spec == nil {
			return
		}
		spec.SupportsResume = enabled
	}
}

// WithFamily overrides the default family for a capability spec.
func WithFamily(family string) Option {
	return func(spec *Spec) {
		if spec == nil {
			return
		}
		spec.Family = strings.TrimSpace(family)
	}
}

// WithRoles overrides the default roles for a capability spec.
func WithRoles(roles ...string) Option {
	return func(spec *Spec) {
		if spec == nil {
			return
		}
		spec.Roles = append([]string(nil), roles...)
	}
}

// WithDependencies overrides the declared dependency names for a capability spec.
func WithDependencies(names ...string) Option {
	return func(spec *Spec) {
		if spec == nil {
			return
		}
		spec.Dependencies = append([]string(nil), names...)
	}
}

// WithPreconditions overrides the declared invocation preconditions.
func WithPreconditions(preconditions ...Precondition) Option {
	return func(spec *Spec) {
		if spec == nil {
			return
		}
		spec.Preconditions = append([]Precondition(nil), preconditions...)
	}
}

// WithProducesEvidence overrides the evidence-production hint.
func WithProducesEvidence(enabled bool) Option {
	return func(spec *Spec) {
		if spec == nil {
			return
		}
		spec.ProducesEvidence = enabled
	}
}

// WithIdempotency overrides the idempotency classification for a capability spec.
func WithIdempotency(level string) Option {
	return func(spec *Spec) {
		if spec == nil {
			return
		}
		spec.Idempotency = strings.TrimSpace(level)
	}
}
