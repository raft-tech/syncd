package client

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/raft-tech/syncd/internal/api"
	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/graph"
	"github.com/raft-tech/syncd/pkg/metrics"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var UnhandledError error = errors.New("an unhandled gRPC error occurred")

type Client interface {
	io.Closer
	Connect(ctx context.Context) error
	Check(ctx context.Context, model string, as string) (bool, error)
	Push(ctx context.Context, model string, from graph.Source, as string) error
	Pull(ctx context.Context, model string, to graph.Destination, as string) error
}

func New(ctx context.Context, target string, opt ...ClientOption) (Client, error) {
	c := &client{
		serverName:    target,
		serverAddress: target,
		metrics:       metrics.ForClient(nil), // Default to the NOP collector
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
	serverName    string
	serverAddress string
	dialOpts      []grpc.DialOption
	conn          *grpc.ClientConn
	sync          api.SyncClient
	metrics       metrics.Collector
}

func (c *client) collector(model string) metrics.RequestMetricsCollector {
	return c.metrics.WithLabels(prometheus.Labels{
		"peer":  c.serverName,
		"model": model,
	})
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

func (c *client) Check(ctx context.Context, model string, as string) (bool, error) {

	// verify connection
	c.RLock()
	defer c.RUnlock()
	if c.conn == nil {
		panic("Connect() must be called before client operations")
	}

	// perform check
	logger := log.FromContext(ctx).With(zap.String("peer", c.serverAddress), zap.String("model", model), zap.String("as", as), zap.String("call", "check"))
	logger.Debug("checking peer")
	if i, e := c.sync.Check(metadata.AppendToOutgoingContext(ctx, "peer", as, "model", model), api.CheckInfo()); e == nil {
		logger.Info("peer check successful", zap.Uint32("peerApiVersion", i.ApiVersion))
		c.collector(model).Checked()
		return true, nil
	} else {
		c.collector(model).Erred()
		return false, parseError(logger, e)
	}
}

func (c *client) Push(ctx context.Context, model string, from graph.Source, as string) error {

	if from == nil {
		panic("from must not be nil")
	}

	// verify connection
	c.RLock()
	defer c.RUnlock()
	if c.conn == nil {
		panic("Connect() must be called before client operations")
	}

	// set up to push
	rmetrics := c.collector(model)
	logger := log.FromContext(ctx).With(zap.String("peer", c.serverAddress), zap.String("as", as), zap.String("call", "push"))
	logger.Debug("pushing to peer")
	var push api.Sync_PushClient
	if pc, err := c.sync.Push(metadata.AppendToOutgoingContext(ctx, "peer", as, "model", model)); err == nil {
		push = pc
	} else {
		rmetrics.Erred()
		return parseError(logger, err)
	}

	// Receive RecordStatuses
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for done := false; !done; {
			if status, err := push.Recv(); err == nil {
				rmetrics.Acknowledged()
				if err = from.SetStatus(ctx, status); err != nil {
					rmetrics.Erred()
					logger.Error("error setting record status", zap.Error(err))
				}
			} else {
				done = true
				switch {
				case errors.Is(err, io.EOF):
					// done
				case errors.Is(err, context.Canceled):
					fallthrough
				case errors.Is(err, context.DeadlineExceeded):
					logger.Info("context canceled")
				default:
					rmetrics.Erred()
					logger.Error("error receiving record statuses", zap.Error(err))
				}
			}
		}
	}()

	// Send all data
	src := from.Fetch(ctx)
	for done := false; !done; {
		select {
		case d, ok := <-src:
			if ok {
				if e := push.Send(d); e == nil {
					rmetrics.Pushed()
				} else {
					rmetrics.Erred()
					logger.Error("error sending record", zap.Error(e))
				}
			} else {
				done = true
			}
		case <-ctx.Done():
			logger.Info("context canceled")
			done = true
			for range src {
				_ = <-src
			}
		}
	}
	if e := push.CloseSend(); e != nil {
		rmetrics.Erred()
		logger.Error("error closing send", zap.Error(e))
	}
	wg.Wait() // finish receiving record status

	return from.Error()
}

func (c *client) Pull(ctx context.Context, model string, to graph.Destination, as string) error {

	if to == nil {
		panic("to must not be nil")
	}

	// verify connection
	c.RLock()
	defer c.RUnlock()
	if c.conn == nil {
		panic("Connect() must be called before client operations")
	}

	// set up to pull
	rmetrics := c.collector(model)
	logger := log.FromContext(ctx).With(zap.String("peer", c.serverAddress), zap.String("as", as), zap.String("call", "pull"))
	logger.Debug("initiating pull request")
	var pull api.Sync_PullClient
	if pc, err := c.sync.Pull(metadata.AppendToOutgoingContext(ctx, "peer", as, "model", model), &api.PullRequest{}); err == nil {
		pull = pc
		logger.Info("ready to pull")
	} else {
		rmetrics.Erred()
		return parseError(logger, err)
	}

	// set up to acknowledge
	var ack api.Sync_AcknowledgeClient
	if ac, err := c.sync.Acknowledge(metadata.AppendToOutgoingContext(ctx, "peer", as, "model", model)); err == nil {
		ack = ac
		defer func() {
			if e := ac.CloseSend(); e != nil {
				logger.Error("error closing acknowledge send channel", zap.Error(e))
			}
		}()
	} else {
		rmetrics.Erred()
		return parseError(logger, err)
	}

	// Set up record and status channels

	// Send received records to destination
	records := make(chan *api.Record)
	go func(dst chan<- *api.Record) {
		defer close(records)
		for done := false; !done; {
			logger.Debug("awaiting new record")
			rec, err := pull.Recv()
			if err == nil {
				logger.Debug("record received from peer")
				select {
				case records <- rec:
					rmetrics.Pulled()
					logger.Debug("record sent to destination")
				case <-ctx.Done():
					logger.Info("context canceled")
					done = true
				}
			} else {
				done = true
				switch {
				case errors.Is(err, io.EOF):
					logger.Debug("all records received")
				case errors.Is(err, context.Canceled):
					fallthrough
				case errors.Is(err, context.DeadlineExceeded):
					logger.Info("context canceled while receiving")
				default:
					rmetrics.Erred()
					logger.Error("error receiving records", zap.Error(err))
				}
			}
		}
	}(records)
	statuses := to.Write(ctx, records)

	// Acknowledge record statuses
	for done := false; !done; {
		select {
		case s, ok := <-statuses:
			if ok {
				logger := logger.With(zap.String("id", s.Id), zap.String("version", s.Version))
				logger.Debug("sending status")
				if err := ack.Send(s); err == nil {
					rmetrics.Acknowledged()
					logger.Debug("status sent")
				} else {
					rmetrics.Erred()
					logger.Error("error sending record status", zap.Error(err))
				}
			} else {
				done = true
				logger.Debug("completed send")
			}
		case <-ctx.Done():
			logger.Info("context canceled while sending acks")
			done = true
			for range statuses {
				_ = <-statuses
			}
		}
	}
	logger.Debug("closing ack channel")
	if err := ack.CloseSend(); err == nil {
		logger.Debug("ack channel closed")
	} else {
		rmetrics.Erred()
		logger.Error("error closing ack channel", zap.Error(err))
	}

	return to.Error()
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
