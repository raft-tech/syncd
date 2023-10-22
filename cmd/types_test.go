package cmd_test

import (
	"context"

	"github.com/raft-tech/syncd/internal/api"
	"github.com/raft-tech/syncd/pkg/graph"
	"github.com/stretchr/testify/mock"
)

type GraphFactory struct {
	mock.Mock
}

func (f *GraphFactory) Build(ctx context.Context, m *graph.Model) (graph.Graph, error) {
	args := f.MethodCalled("Build", ctx, m)
	return args.Get(0).(graph.Graph), args.Error(1)
}

func (f *GraphFactory) Close(ctx context.Context) error {
	return f.MethodCalled("Close", ctx).Error(0)
}

type Graph struct {
	mock.Mock
}

func (g *Graph) Source(peer string, filters ...graph.Filter) graph.Source {
	return g.MethodCalled("Source", peer, filters).Get(0).(graph.Source)
}

func (g *Graph) Destination() graph.Destination {
	return g.MethodCalled("Destination").Get(0).(graph.Destination)
}

type Source struct {
	mock.Mock
}

func (s *Source) Fetch(ctx context.Context) <-chan *api.Record {
	return s.MethodCalled("Fetch", ctx).Get(0).(<-chan *api.Record)
}

func (s *Source) Error() error {
	return s.MethodCalled("Error").Error(0)
}

func (s *Source) SetStatus(ctx context.Context, status ...*api.RecordStatus) error {
	return s.MethodCalled("SetStatus", ctx, status).Error(0)
}

type Destination struct {
	mock.Mock
}

func (d *Destination) Write(ctx context.Context, records <-chan *api.Record) <-chan *api.RecordStatus {
	return d.MethodCalled("Write", ctx, records).Get(0).(<-chan *api.RecordStatus)
}

func (d *Destination) Error() error {
	return d.MethodCalled("Error").Error(0)
}
