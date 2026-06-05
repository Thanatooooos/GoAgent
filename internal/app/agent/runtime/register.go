package runtime

import "github.com/cloudwego/eino/schema"

func init() {
	schema.RegisterName[*RuntimeSession]("agent_runtime_RuntimeSession")
	schema.RegisterName[RequestEnvelope]("agent_runtime_RequestEnvelope")
	schema.RegisterName[CheckpointRef]("agent_runtime_CheckpointRef")
	schema.RegisterName[SessionMetadata]("agent_runtime_SessionMetadata")
	schema.RegisterName[DecisionArtifact]("agent_runtime_DecisionArtifact")
	schema.RegisterName[NodeResult]("agent_runtime_NodeResult")
}
