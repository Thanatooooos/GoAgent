package sqlc

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type DBTX interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Queries struct {
	db DBTX
}

func New(db DBTX) *Queries {
	return &Queries{db: db}
}
