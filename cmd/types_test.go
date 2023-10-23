package cmd_test

import (
	"context"

	"github.com/raft-tech/syncd/internal/api"
	"github.com/raft-tech/syncd/pkg/graph"
	"github.com/stretchr/testify/mock"
)

type Server struct {
	mock.Mock
	api.UnimplementedSyncServer
}

func (s *Server) Check(ctx context.Context, info *api.Info) (*api.Info, error) {
	args := s.MethodCalled("Check", ctx, info)
	return args.Get(0).(*api.Info), args.Error(1)
}

func (s *Server) Push(server api.Sync_PushServer) error {
	return s.MethodCalled("Push", server).Error(0)
}

func (s *Server) Pull(request *api.PullRequest, server api.Sync_PullServer) error {
	return s.MethodCalled("Pull", request, server).Error(0)
}

func (s *Server) Acknowledge(server api.Sync_AcknowledgeServer) error {
	return s.MethodCalled("Acknowledge", server).Error(0)
}

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
	res := s.MethodCalled("Fetch", ctx).Get(0)
	if c, ok := res.(<-chan *api.Record); ok {
		return c
	} else if fn, ok := res.(func() <-chan *api.Record); ok {
		return fn()
	} else {
		panic("Fetch(context.Context) not implemented")
	}
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
	res := d.MethodCalled("Write", ctx, records).Get(0)
	if c, ok := res.(<-chan *api.RecordStatus); ok {
		return c
	} else if fn, ok := res.(func() <-chan *api.RecordStatus); ok {
		return fn()
	} else {
		panic("Write(context.Context, <-chan *api.Record) not implemented")
	}
}

func (d *Destination) Error() error {
	return d.MethodCalled("Error").Error(0)
}
