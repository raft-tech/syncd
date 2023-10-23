/*
 *     Copyright (c) 2023. Raft LLC
 *
 *     This program is free software: you can redistribute it and/or modify
 *     it under the terms of the GNU General Public License as published by
 *     the Free Software Foundation, either version 3 of the License, or
 *     (at your option) any later version.
 *
 *     This program is distributed in the hope that it will be useful,
 *     but WITHOUT ANY WARRANTY; without even the implied warranty of
 *     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *     GNU General Public License for more details.
 *
 *     You should have received a copy of the GNU General Public License
 *     along with this program.  If not, see <https://www.gnu.org/licenses/>.
 *
 */

package server

import (
	"context"
	"errors"
	"io"

	"github.com/raft-tech/syncd/internal/api"
	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/graph"
	"github.com/raft-tech/syncd/pkg/metrics"
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
	filters       Filters
	metrics       metrics.ServerCollector
}

func (s *server) unknownModel() error {
	s.metrics.InvalidModel()
	return status.Error(codes.NotFound, "unknown model")
}

func (s *server) invalidModelForPeer(peer string) error {
	s.metrics.InvalidPeerModel(peer)
	return status.Error(codes.NotFound, "unknown model")
}

func (s *server) Check(ctx context.Context, info *api.Info) (*api.Info, error) {
	md, logger := setUp(ctx, "check")
	if md.Model != "" && !s.peerValidator(md.Peer, md.Model) {
		logger.Info("peer requested unknown or unbound model")
		return nil, s.invalidModelForPeer(md.Peer)
	}
	logger.Debug("check successful")
	s.metrics.WithLabels(md.MetricLabels()).Checked()
	return api.CheckInfo(), nil
}

func (s *server) Push(ps api.Sync_PushServer) error {

	ctx := ps.Context()
	md, logger := setUp(ctx, "push")
	if !s.peerValidator(md.Peer, md.Model) {
		return s.invalidModelForPeer(md.Peer)
	}
	var dst graph.Destination
	if g, ok := s.lookup(md.Model); ok {
		dst = g.Destination()
	} else {
		return s.unknownModel()
	}

	rmetrics := s.metrics.WithLabels(md.MetricLabels())

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
					rmetrics.Erred()
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
				if err := ps.Send(rs); err == nil {
					rmetrics.Pushed()
				} else {
					rmetrics.Erred()
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
			rmetrics.Erred()
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
		return s.invalidModelForPeer(md.Peer)
	}
	var src graph.Source
	if g, ok := s.lookup(md.Model); ok {
		src = g.Source(md.Peer, s.filters[md.Model]...)
	} else {
		return s.unknownModel()
	}

	rmetrics := s.metrics.WithLabels(md.MetricLabels())

	in := src.Fetch(ctx)
	for done := false; !done; {
		select {
		case r, ok := <-in:
			if ok {
				if err := ps.Send(r); err == nil {
					rmetrics.Pulled()
				} else {
					rmetrics.Erred()
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
		return s.invalidModelForPeer(md.Peer)
	}
	var src graph.Source
	if g, ok := s.lookup(md.Model); ok {
		src = g.Source(md.Peer) // TODO add filters
	} else {
		return s.unknownModel()
	}

	rmetrics := s.metrics.WithLabels(md.MetricLabels())
	for done := false; !done; {
		rec, err := as.Recv()
		if err == nil {
			if err = src.SetStatus(ctx, rec); err == nil {
				rmetrics.Acknowledged()
			} else {
				rmetrics.Erred()
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
				rmetrics.Erred()
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
