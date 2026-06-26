package history

import ragtoken "local/rag-project/internal/app/rag/core/tokenbudget"

type TokenEstimator = ragtoken.Estimator

func NewTokenEstimateAdapter() TokenEstimator {
	return ragtoken.NewDefaultEstimator()
}
