package work

import "local/rag-project/internal/app/runtime/capability"

func (r *Runtime) scopeRetrieval(def capability.Def) capability.Def {
	describe, execute := def.Describe, def.Execute
	scope := func(c capability.Context) (capability.Context, error) {
		if c.Work == nil {
			return c, capability.Deny("Work scope required")
		}
		ids, err := r.Store.SourceScope(c.Context, c.UserID, c.Work.TopicID, c.ConversationID)
		c.KnowledgeBaseIDs = ids
		c.AllowKnowledgeRetrieval = len(ids) > 0
		return c, err
	}
	def.Describe = func(v capability.Value, c capability.Context) (capability.Operation, error) {
		c, err := scope(c)
		if err != nil {
			return capability.Operation{}, err
		}
		return describe(v, c)
	}
	def.Execute = func(v capability.Value, c capability.Context) (capability.Result, error) {
		c, err := scope(c)
		if err != nil {
			return capability.Result{}, err
		}
		return execute(v, c)
	}
	return def
}
