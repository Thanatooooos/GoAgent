package tool

import (
	"context"

	ragcore "local/rag-project/internal/app/rag/tool/core"
)

const (
	ParamTypeString  = ragcore.ParamTypeString
	ParamTypeNumber  = ragcore.ParamTypeNumber
	ParamTypeInteger = ragcore.ParamTypeInteger
	ParamTypeBoolean = ragcore.ParamTypeBoolean
	ParamTypeObject  = ragcore.ParamTypeObject
	ParamTypeArray   = ragcore.ParamTypeArray
)

type ParameterDefinition = ragcore.ParameterDefinition
type Definition = ragcore.Definition
type Call = ragcore.Call
type Result = ragcore.Result
type ResultMeta = ragcore.ResultMeta
type Tool = ragcore.Tool

// Ensure context import stays available to callers that dot-import tool.
var _ context.Context
