package trace

import (
	"context"
	"errors"
	"testing"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	userdomain "local/rag-project/internal/app/user/domain"
	userport "local/rag-project/internal/app/user/port"
)

type traceRunRepoStub struct {
	countFn      func(ctx context.Context, filter port.RagTraceRunListFilter) (int, error)
	listFn       func(ctx context.Context, filter port.RagTraceRunListFilter) ([]domain.RagTraceRun, error)
	getByTraceFn func(ctx context.Context, traceID string) (domain.RagTraceRun, error)
}

func (s traceRunRepoStub) Create(context.Context, domain.RagTraceRun) (domain.RagTraceRun, error) {
	return domain.RagTraceRun{}, nil
}

func (s traceRunRepoStub) UpdateByTraceID(context.Context, string, domain.RagTraceRun) error {
	return nil
}

func (s traceRunRepoStub) UpdateWhere(context.Context, port.RagTraceRunConditions, port.RagTraceRunPatch) (int64, error) {
	return 0, nil
}

func (s traceRunRepoStub) GetByTraceID(ctx context.Context, traceID string) (domain.RagTraceRun, error) {
	return s.getByTraceFn(ctx, traceID)
}

func (s traceRunRepoStub) Count(ctx context.Context, filter port.RagTraceRunListFilter) (int, error) {
	return s.countFn(ctx, filter)
}

func (s traceRunRepoStub) List(ctx context.Context, filter port.RagTraceRunListFilter) ([]domain.RagTraceRun, error) {
	return s.listFn(ctx, filter)
}

type traceNodeRepoStub struct {
	listFn func(ctx context.Context, traceID string) ([]domain.RagTraceNode, error)
}

func (s traceNodeRepoStub) Create(context.Context, domain.RagTraceNode) (domain.RagTraceNode, error) {
	return domain.RagTraceNode{}, nil
}

func (s traceNodeRepoStub) UpdateByTraceIDAndNodeID(context.Context, string, string, domain.RagTraceNode) error {
	return nil
}

func (s traceNodeRepoStub) UpdateWhere(context.Context, port.RagTraceNodeConditions, port.RagTraceNodePatch) (int64, error) {
	return 0, nil
}

func (s traceNodeRepoStub) ListByTraceID(ctx context.Context, traceID string) ([]domain.RagTraceNode, error) {
	return s.listFn(ctx, traceID)
}

type traceUserRepoStub struct {
	getByIDFn func(ctx context.Context, id string) (userdomain.User, error)
}

func (s traceUserRepoStub) Create(context.Context, userdomain.User) (userdomain.User, error) {
	return userdomain.User{}, nil
}

func (s traceUserRepoStub) Update(context.Context, userdomain.User) (userdomain.User, error) {
	return userdomain.User{}, nil
}

func (s traceUserRepoStub) Delete(context.Context, string) error {
	return nil
}

func (s traceUserRepoStub) GetByID(ctx context.Context, id string) (userdomain.User, error) {
	return s.getByIDFn(ctx, id)
}

func (s traceUserRepoStub) GetByUsername(context.Context, string) (userdomain.User, error) {
	return userdomain.User{}, nil
}

func (s traceUserRepoStub) Count(context.Context, userport.UserListFilter) (int, error) {
	return 0, nil
}

func (s traceUserRepoStub) List(context.Context, userport.UserListFilter) ([]userdomain.User, error) {
	return nil, nil
}

func TestTraceServicePageRunsAppliesDefaultsAndReturnsPage(t *testing.T) {
	var countedFilter port.RagTraceRunListFilter
	var listedFilter port.RagTraceRunListFilter

	service := NewService(
		traceRunRepoStub{
			countFn: func(_ context.Context, filter port.RagTraceRunListFilter) (int, error) {
				countedFilter = filter
				return 3, nil
			},
			listFn: func(_ context.Context, filter port.RagTraceRunListFilter) ([]domain.RagTraceRun, error) {
				listedFilter = filter
				return []domain.RagTraceRun{
					{ID: "1", TraceID: "trace-1", Status: "success"},
				}, nil
			},
			getByTraceFn: func(context.Context, string) (domain.RagTraceRun, error) {
				return domain.RagTraceRun{}, nil
			},
		},
		traceNodeRepoStub{listFn: func(context.Context, string) ([]domain.RagTraceNode, error) { return nil, nil }},
		nil,
	)

	result, err := service.PageRuns(context.Background(), PageTraceRunsInput{
		Page:           0,
		PageSize:       999,
		TraceID:        " trace-1 ",
		ConversationID: " c1 ",
		TaskID:         " t1 ",
		Status:         " success ",
	})
	if err != nil {
		t.Fatalf("PageRuns returned error: %v", err)
	}
	if result.Total != 3 || result.Page != 1 || result.PageSize != 100 {
		t.Fatalf("unexpected page result: %#v", result)
	}
	if len(result.Items) != 1 || result.Items[0].TraceID != "trace-1" {
		t.Fatalf("unexpected items: %#v", result.Items)
	}
	if countedFilter.TraceID != "trace-1" || countedFilter.ConversationID != "c1" || countedFilter.TaskID != "t1" || countedFilter.Status != "success" {
		t.Fatalf("unexpected count filter: %#v", countedFilter)
	}
	if listedFilter.Offset != 0 || listedFilter.Limit != 100 {
		t.Fatalf("unexpected list options: %#v", listedFilter.ListOptions)
	}
}

func TestTraceServicePageRunsUsesRequestedPageWindow(t *testing.T) {
	var listedFilter port.RagTraceRunListFilter

	service := NewService(
		traceRunRepoStub{
			countFn: func(_ context.Context, filter port.RagTraceRunListFilter) (int, error) {
				return 25, nil
			},
			listFn: func(_ context.Context, filter port.RagTraceRunListFilter) ([]domain.RagTraceRun, error) {
				listedFilter = filter
				return []domain.RagTraceRun{}, nil
			},
			getByTraceFn: func(context.Context, string) (domain.RagTraceRun, error) {
				return domain.RagTraceRun{}, nil
			},
		},
		traceNodeRepoStub{listFn: func(context.Context, string) ([]domain.RagTraceNode, error) { return nil, nil }},
		nil,
	)

	result, err := service.PageRuns(context.Background(), PageTraceRunsInput{
		Page:     3,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("PageRuns returned error: %v", err)
	}
	if result.Page != 3 || result.PageSize != 10 {
		t.Fatalf("unexpected page result: %#v", result)
	}
	if listedFilter.Offset != 20 || listedFilter.Limit != 10 {
		t.Fatalf("unexpected paging filter: %#v", listedFilter.ListOptions)
	}
}

func TestTraceServiceGetDetailReturnsRunAndNodes(t *testing.T) {
	now := time.Now()
	service := NewService(
		traceRunRepoStub{
			countFn: func(context.Context, port.RagTraceRunListFilter) (int, error) { return 0, nil },
			listFn:  func(context.Context, port.RagTraceRunListFilter) ([]domain.RagTraceRun, error) { return nil, nil },
			getByTraceFn: func(_ context.Context, traceID string) (domain.RagTraceRun, error) {
				if traceID != "trace-1" {
					t.Fatalf("unexpected trace id: %s", traceID)
				}
				return domain.RagTraceRun{
					ID:        "1",
					TraceID:   "trace-1",
					Status:    "success",
					StartTime: &now,
				}, nil
			},
		},
		traceNodeRepoStub{
			listFn: func(_ context.Context, traceID string) ([]domain.RagTraceNode, error) {
				if traceID != "trace-1" {
					t.Fatalf("unexpected trace id: %s", traceID)
				}
				return []domain.RagTraceNode{{ID: "n1", TraceID: traceID, NodeID: "retrieve"}}, nil
			},
		},
		nil,
	)

	detail, err := service.GetDetail(context.Background(), " trace-1 ")
	if err != nil {
		t.Fatalf("GetDetail returned error: %v", err)
	}
	if detail.Run.TraceID != "trace-1" || len(detail.Nodes) != 1 || detail.Nodes[0].NodeID != "retrieve" {
		t.Fatalf("unexpected detail: %#v", detail)
	}
}

func TestTraceServiceGetDetailReturnsNotFoundWhenRunMissing(t *testing.T) {
	service := NewService(
		traceRunRepoStub{
			countFn: func(context.Context, port.RagTraceRunListFilter) (int, error) { return 0, nil },
			listFn:  func(context.Context, port.RagTraceRunListFilter) ([]domain.RagTraceRun, error) { return nil, nil },
			getByTraceFn: func(context.Context, string) (domain.RagTraceRun, error) {
				return domain.RagTraceRun{}, nil
			},
		},
		traceNodeRepoStub{listFn: func(context.Context, string) ([]domain.RagTraceNode, error) { return nil, nil }},
		nil,
	)

	if _, err := service.GetDetail(context.Background(), "trace-1"); err == nil {
		t.Fatal("expected not found error")
	}
}

func TestTraceServicePageRunsWrapsRepoErrors(t *testing.T) {
	service := NewService(
		traceRunRepoStub{
			countFn: func(context.Context, port.RagTraceRunListFilter) (int, error) {
				return 0, errors.New("db down")
			},
			listFn: func(context.Context, port.RagTraceRunListFilter) ([]domain.RagTraceRun, error) {
				return nil, nil
			},
			getByTraceFn: func(context.Context, string) (domain.RagTraceRun, error) {
				return domain.RagTraceRun{}, nil
			},
		},
		traceNodeRepoStub{listFn: func(context.Context, string) ([]domain.RagTraceNode, error) { return nil, nil }},
		nil,
	)

	if _, err := service.PageRuns(context.Background(), PageTraceRunsInput{Page: 1, PageSize: 10}); err == nil {
		t.Fatal("expected error")
	}
}

func TestTraceServiceResolveUserNameReturnsUsername(t *testing.T) {
	service := NewService(
		traceRunRepoStub{},
		traceNodeRepoStub{},
		traceUserRepoStub{
			getByIDFn: func(_ context.Context, id string) (userdomain.User, error) {
				if id != "u1" {
					t.Fatalf("unexpected user id: %s", id)
				}
				return userdomain.User{ID: "u1", Username: "alice"}, nil
			},
		},
	)

	username, err := service.ResolveUserName(context.Background(), " u1 ")
	if err != nil {
		t.Fatalf("ResolveUserName returned error: %v", err)
	}
	if username != "alice" {
		t.Fatalf("expected alice, got %q", username)
	}
}
