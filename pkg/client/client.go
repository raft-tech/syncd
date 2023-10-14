package client

import (
	"context"
	"errors"
	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/api"
	"github.com/raft-tech/syncd/pkg/graph"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"io"
	"sync"
)

var UnhandledError error = errors.New("an unhandled gRPC error occurred")

type Client interface {
	io.Closer
	Connect(ctx context.Context) error
	Check(ctx context.Context, as string) (bool, error)
	Push(ctx context.Context, from graph.Source, as string) error
	Pull(ctx context.Context, to graph.Destination, as string) error
}

func New(ctx context.Context, target string, opt ...ClientOption) (Client, error) {
	c := &client{
		serverAddress: target,
	}
	for i := range opt {
		if err := opt[i](c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

type client struct {
	sync.RWMutex
	serverAddress string
	dialOpts      []grpc.DialOption
	conn          *grpc.ClientConn
	sync          api.SyncClient
}

func (c *client) Connect(ctx context.Context) error {
	c.Lock()
	defer c.Unlock()
	logger := log.FromContext(ctx).With(zap.String("peer", c.serverAddress))
	logger.Debug("initializing connection")
	if cc, err := grpc.DialContext(ctx, c.serverAddress, c.dialOpts...); err == nil {
		logger.Debug("connection successfully established")
		c.conn = cc
		c.sync = api.NewSyncClient(cc)
		return nil
	} else {
		if errors.Is(err, context.Canceled) {
			logger.Info("context canceled while connecting")
		} else {
			logger.Error("error connecting to peer", zap.Error(err))
		}
		return err
	}

}

func (c *client) Check(ctx context.Context, as string) (bool, error) {

	// verify connection
	c.RLock()
	defer c.RUnlock()
	if c.conn == nil {
		panic("Connect() must be called before client operations")
	}

	// perform check
	logger := log.FromContext(ctx).With(zap.String("peer", c.serverAddress), zap.String("as", as), zap.String("call", "check"))
	logger.Debug("checking peer")
	if i, e := c.sync.Check(metadata.AppendToOutgoingContext(ctx, "peer", as), api.CheckInfo()); e == nil {
		logger.Info("peer check successful", zap.Uint32("peerApiVersion", i.ApiVersion))
		return true, nil
	} else {
		return false, parseError(logger, e)
	}
}

func (c *client) Push(ctx context.Context, from graph.Source, as string) error {
	//TODO implement me
	panic("implement me")
}

func (c *client) Pull(ctx context.Context, to graph.Destination, as string) error {
	//TODO implement me
	panic("implement me")
}

func (c *client) Close() (err error) {
	c.Lock()
	defer c.Unlock()
	if c.conn != nil {
		err = c.conn.Close()
		c.conn = nil
		c.sync = nil
	}
	return err
}

func parseError(logger *zap.Logger, err error) error {
	if s, ok := status.FromError(err); ok {
		logger.Error("sync call failed", zap.Uint32("code", uint32(s.Code())), zap.String("msg", s.Message()))
		return UnhandledError
	} else if errors.Is(err, context.Canceled) {
		logger.Info("context canceled")
		return err
	} else if errors.Is(err, context.DeadlineExceeded) {
		logger.Info("context deadline exceeded")
		return err
	} else {
		logger.Error("sync call failed with unknown error", zap.Error(err))
		return err
	}
}
