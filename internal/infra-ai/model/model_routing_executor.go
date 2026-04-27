package model

import (
	"fmt"
	"strings"

	"local/rag-project/internal/framework/exception"
	fwlog "local/rag-project/internal/framework/log"
	aienum "local/rag-project/internal/infra-ai/enum"
)

type ModelCaller[C any, T any] func(client C, target ModelTarget) (T, error)

type ModelRoutingExecutor struct {
	healthStore *ModelHealthStore
}

func NewModelRoutingExecutor(healthStore *ModelHealthStore) *ModelRoutingExecutor {
	if healthStore == nil {
		healthStore = NewModelHealthStore()
	}
	return &ModelRoutingExecutor{healthStore: healthStore}
}

func ExecuteWithFallback[C any, T any](
	e *ModelRoutingExecutor,
	capability aienum.ModelCapability,
	targets []ModelTarget,
	clientResolver func(target ModelTarget) (C, error),
	caller ModelCaller[C, T],
) (T, error) {
	var zero T
	if e == nil {
		return zero, exception.NewRemoteException("model routing executor is nil", nil)
	}
	if e.healthStore == nil {
		e.healthStore = NewModelHealthStore()
	}

	label := strings.TrimSpace(capability.DisplayName())
	if label == "" {
		label = strings.TrimSpace(capability.ID())
	}
	if label == "" {
		label = "model"
	}

	if len(targets) == 0 {
		return zero, exception.NewRemoteException(
			fmt.Sprintf("no %s model candidates available", label),
			nil,
		)
	}

	if clientResolver == nil {
		return zero, exception.NewRemoteException(
			fmt.Sprintf("%s client resolver is nil", label),
			nil,
		)
	}

	if caller == nil {
		return zero, exception.NewRemoteException(
			fmt.Sprintf("%s caller is nil", label),
			nil,
		)
	}

	var lastErr error
	for _, target := range targets {
		client, err := clientResolver(target)
		if err != nil {
			lastErr = err
			fwlog.Warnf("%s provider client resolve failed: modelId=%s provider=%s error=%v",
				label, target.Id, target.Candidate.Provider, err)
			continue
		}

		if !e.healthStore.allowCall(target.Id) {
			fwlog.Warnf("%s model skipped by circuit breaker: modelId=%s provider=%s",
				label, target.Id, target.Candidate.Provider)
			continue
		}

		response, err := caller(client, target)
		if err != nil {
			lastErr = err
			e.healthStore.markFailure(target.Id)
			fwlog.Warnf("%s model failed, fallback to next: modelId=%s provider=%s error=%v",
				label, target.Id, target.Candidate.Provider, err)
			continue
		}

		e.healthStore.markSuccess(target.Id)
		return response, nil
	}

	message := fmt.Sprintf("all %s model candidates failed", label)
	if lastErr != nil {
		message = fmt.Sprintf("all %s model candidates failed: %v", label, lastErr)
	}
	return zero, exception.NewRemoteException(message, lastErr)
}
