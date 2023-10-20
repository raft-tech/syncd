package server

import (
	"context"
	"errors"
	"io"

	"github.com/raft-tech/syncd/internal/api"
	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/graph"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func New(opt Options) api.SyncServer {
	srv := &server{}
	opt.apply(srv)
	return srv
}

type server struct {
	api.UnimplementedSyncServer
	lookup        GraphResolver
	peerValidator PeerValidator
}

func (s *server) Check(ctx context.Context, info *api.Info) (*api.Info, error) {
	md, logger := setUp(ctx, "check")
	if md.Model != "" && !s.peerValidator(md.Peer, md.Model) {
		logger.Info("peer requested unknown or unbound model")
		return nil, status.Error(codes.NotFound, "unknown model")
	}
	logger.Debug("check successful")
	return api.CheckInfo(), nil
}

func (s *server) Push(ps api.Sync_PushServer) error {

	ctx := ps.Context()
	md, logger := setUp(ctx, "push")
	if !s.peerValidator(md.Peer, md.Model) {
		return status.Error(codes.NotFound, "unknown model")
	}
	var dst graph.Destination
	if g, ok := s.lookup(md.Model); ok {
		dst = g.Destination()
	} else {
		return status.Error(codes.NotFound, "unknown model")
	}

	in := make(chan *api.Record)
	go func(in chan<- *api.Record) {
		defer close(in)
		for done := false; !done; {
			rec, err := ps.Recv()
			if err == nil {
				select {
				case in <- rec:
				case <-ctx.Done():
					done = true
					logger.Info("context canceled while receiving records")
				}
			} else {
				done = true
				switch {
				case errors.Is(err, io.EOF):
					logger.Info("all records received")
				case errors.Is(err, context.Canceled):
					fallthrough
				case errors.Is(err, context.DeadlineExceeded):
					logger.Info("context canceled while receiving records")
				default:
					logger.Error("error while receiving records", zap.Error(err))
				}
			}
		}
	}(in)

	out := dst.Write(ctx, in)
	for done := false; !done; {
		select {
		case rs, ok := <-out:
			if ok {
				if err := ps.Send(rs); err != nil {
					logger.Error("error sending record status", zap.Error(err))
				}
			} else {
				done = true
			}
		case <-ctx.Done():
			logger.Info("context canceled while waiting for next record status")
			done = true
			for range out {
				<-out
			}
		}
	}

	if err := dst.Error(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			logger.Info("context canceled while handling push")
			return status.Error(codes.DeadlineExceeded, "deadline exceeded")
		} else {
			logger.Error("error while handling push", zap.Error(err))
			return status.Error(codes.Internal, "unhandled error")
		}
	}
	logger.Info("finished handling push")
	return nil
}

func (s *server) Pull(_ *api.PullRequest, ps api.Sync_PullServer) error {

	ctx := ps.Context()
	md, logger := setUp(ctx, "pull")
	if !s.peerValidator(md.Peer, md.Model) {
		return status.Error(codes.NotFound, "unknown model")
	}
	var src graph.Source
	if g, ok := s.lookup(md.Model); ok {
		src = g.Source(md.Peer) // TODO add filters
	} else {
		return status.Error(codes.NotFound, "unknown model")
	}

	in := src.Fetch(ctx)
	for done := false; !done; {
		select {
		case r, ok := <-in:
			if ok {
				if err := ps.Send(r); err != nil {
					logger.Error("error sending record", zap.Error(err))
				}
			} else {
				done = true
			}
		case <-ctx.Done():
			logger.Info("context canceled while handling push")
			for range in {
				<-in
			}
			return status.Error(codes.DeadlineExceeded, "deadline exceeded")
		}
	}

	return nil
}

func (s *server) Acknowledge(as api.Sync_AcknowledgeServer) error {

	ctx := as.Context()
	md, logger := setUp(ctx, "acknowledge")
	if !s.peerValidator(md.Peer, md.Model) {
		return status.Error(codes.NotFound, "unknown model")
	}
	var src graph.Source
	if g, ok := s.lookup(md.Model); ok {
		src = g.Source(md.Peer) // TODO add filters
	} else {
		return status.Error(codes.NotFound, "unknown model")
	}

	for done := false; !done; {
		rec, err := as.Recv()
		if err == nil {
			if err = src.SetStatus(ctx, rec); err != nil {
				logger.Error("error setting record status", zap.Error(err))
				return status.Error(codes.Internal, "unhandled error")
			}
		} else {
			done = true
			switch {
			case errors.Is(err, io.EOF):
				logger.Info("all records received")
			case errors.Is(err, context.Canceled):
				fallthrough
			case errors.Is(err, context.DeadlineExceeded):
				logger.Info("context canceled while receiving records")
			default:
				logger.Error("error while receiving records", zap.Error(err))
			}
		}
	}
	return nil
}

func setUp(ctx context.Context, call string) (api.Metadata, *zap.Logger) {
	logger := log.FromContext(ctx).With(zap.String("call", call))
	md := api.Metadata{}
	if d, ok := api.GetMetadataFromContext(ctx); ok {
		md = d
		logger = logger.With(zap.String("peer", md.Peer), zap.String("model", md.Model))
	}
	return md, logger
}

func ServerStreamWithContext(ss grpc.ServerStream, ctx context.Context) grpc.ServerStream {
	return &serverStreamWithContext{
		ServerStream: ss,
		ctx:          ctx,
	}
}

type serverStreamWithContext struct {
	grpc.ServerStream
	ctx context.Context
}

func (ssc *serverStreamWithContext) Context() context.Context {
	return ssc.ctx
}
