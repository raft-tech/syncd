package client

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/api"
	"github.com/raft-tech/syncd/pkg/graph"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var UnhandledError error = errors.New("an unhandled gRPC error occurred")

type Client interface {
	io.Closer
	Connect(ctx context.Context) error
	Check(ctx context.Context, as string) (bool, error)
	Push(ctx context.Context, model string, from graph.Source, as string) error
	Pull(ctx context.Context, model string, to graph.Destination, as string) error
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
	logger := log.FromContext(ctx).With(zap.String("peer", c.serverAddress), zap.String("as", as), zap.String("call", "push"))
	logger.Debug("pushing to peer")
	var push api.Sync_PushClient
	if pc, err := c.sync.Push(metadata.AppendToOutgoingContext(ctx, "peer", as, "model", model)); err == nil {
		push = pc
	} else {
		return parseError(logger, err)
	}

	// Receive RecordStatuses
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		for done := false; !done; {
			if status, err := push.Recv(); err == nil {
				if err = from.SetStatus(ctx, status); err != nil {
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
				if e := push.Send(d); e != nil {
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

	var rcount, scount int

	// set up to pull
	logger := log.FromContext(ctx).With(zap.String("peer", c.serverAddress), zap.String("as", as), zap.String("call", "pull"))
	logger.Debug("initiating pull request")
	var pull api.Sync_PullClient
	if pc, err := c.sync.Pull(metadata.AppendToOutgoingContext(ctx, "peer", as, "model", model)); err == nil {
		pull = pc
		logger.Info("ready to pull")
	} else {
		return parseError(logger, err)
	}

	// Set up record and status channels
	records := make(chan *api.Record)
	defer func() {
		if records != nil {
			close(records)
		}
	}()
	statuses := to.Write(ctx, records)

	// send status updates asynchronously
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func(src <-chan *api.RecordStatus) {
		for done := false; !done; {
			select {
			case s, ok := <-src:
				if ok {
					logger := logger.With(zap.String("id", s.Id), zap.String("version", s.Version))
					logger.Debug("sending status")
					if err := pull.Send(s); err == nil {
						scount++
						logger.Debug("status sent")
					} else {
						logger.Error("error sending record status", zap.Error(err))
					}
				} else {
					done = true
					logger.Debug("completed send")
				}
			case <-ctx.Done():
				logger.Info("context canceled while sending")
				done = true
				for range src {
					_ = <-src
				}
			}
		}
		logger.Debug("closing send channel")
		if err := pull.CloseSend(); err == nil {
			logger.Debug("send channel closed")
		} else {
			logger.Error("error closing send channel", zap.Error(err))
		}
		wg.Done()
	}(statuses)

	// receive records
	for done := false; !done; {
		logger.Debug("awaiting new record")
		rec, err := pull.Recv()
		if err == nil {
			logger.Debug("record received from peer")
			select {
			case records <- rec:
				rcount++
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
				logger.Error("error receiving records", zap.Error(err))
			}
		}
	}
	close(records)
	records = nil

	logger.Debug("waiting for send to complete")
	wg.Wait()
	logger.Debug("pull completed", zap.Int("recordsReceived", rcount), zap.Int("statusesSent", scount))

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
