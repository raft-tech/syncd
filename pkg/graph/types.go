package graph

import (
	"context"
	"github.com/raft-tech/syncd/pkg/api"
)

type Graph interface {
	Source(peer string, filters ...Filter) Source
	Destination() Destination
}

type Source interface {
	Fetch(ctx context.Context) <-chan *api.Data
	Error() error
	SetStatus(ctx context.Context, status ...*api.RecordStatus) error
}

type Destination interface {
	Write(ctx context.Context, data <-chan *api.Data) <-chan *api.RecordStatus
	Error() error
}

type Factory interface {
	Build(ctx context.Context, m *Model) (Graph, error)
	Close(ctx context.Context) error
}

type Model struct {
	Name     string
	Table    Table
	IsSet    bool
	ChildKey string
	Children []Model
	Filters  []string
}

type Table struct {
	Name          string
	KeyField      string
	SequenceField string
	PriorityField string
	VersionField  string
}

type Filter struct {
	Key      string
	Operator FilterOperator
	Value    interface{}
}

type FilterOperator string

const (
	GreaterThanFilterOperator FilterOperator = "GreaterThan"
)