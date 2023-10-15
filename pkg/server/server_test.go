package server_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	api "github.com/raft-tech/syncd/internal/api"
	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/client"
	"github.com/raft-tech/syncd/pkg/graph"
	"github.com/raft-tech/syncd/pkg/server"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

var _ = Describe("Client", func() {

	var logger *zap.Logger
	BeforeEach(func() {
		z := zapcore.NewCore(
			zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
			zapcore.AddSync(GinkgoWriter),
			zapcore.DebugLevel)
		logger = zap.New(z, zap.AddCaller())
	})

	AfterEach(func() {
		Expect(logger.Sync()).To(Succeed())
	})

	Context("with connection", func() {

		var listener *bufconn.Listener
		var as string
		var syncd client.Client
		BeforeEach(func(ctx SpecContext) {
			listener = bufconn.Listen(10 * 1024)
			as = ctx.SpecReport().LeafNodeText
			var err error
			syncd, err = client.New(ctx, "buffered", client.WithDialOptions(
				grpc.WithCredentialsBundle(insecure.NewBundle()),
				grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
					return listener.DialContext(ctx)
				}),
			))
			Expect(err).NotTo(HaveOccurred())
		})

		Context("with default server", func() {

			var gserver *grpc.Server
			var serverErr error
			var serverWait sync.WaitGroup
			BeforeEach(func() {
				gserver = grpc.NewServer(
					grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
						ctx = log.NewContext(ctx, logger)
						return handler(ctx, req)
					}),
					grpc.StreamInterceptor(func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
						ctx := log.NewContext(ss.Context(), logger)
						return handler(srv, server.ServerStreamWithContext(ss, ctx))
					}),
				)
				api.RegisterSyncServer(gserver, server.New(server.Options{}))
				serverWait.Add(1)
				go func() {
					defer serverWait.Done()
					serverErr = gserver.Serve(listener)
				}()
			})

			AfterEach(func() {
				gserver.Stop()
				serverWait.Wait()
				Expect(serverErr).ToNot(HaveOccurred())
			})

			var graphh *Graph
			var model = "data"
			BeforeEach(func(ctx SpecContext) {
				Expect(syncd.Connect(ctx)).To(Succeed())
				graphh = new(Graph)
				server.DeregisterGraph(model)
				server.RegisterGraph(model, graphh)
			})

			It("handles check", func(ctx context.Context) {
				ctx = log.NewContext(ctx, logger)
				Expect(syncd.Check(ctx, model, as)).To(BeTrue())
			})

			It("handles push", func(ctx context.Context) {

				ctx = log.NewContext(ctx, logger)
				data := []*api.Record{
					{
						Fields: map[string]*api.Data{
							"id":      api.StringData{}.From("1"),
							"name":    api.StringData{}.From("John Mayer"),
							"version": api.StringData{}.From("a"),
						},
					},
					{
						Fields: map[string]*api.Data{
							"id":      api.StringData{}.From("2"),
							"name":    api.StringData{}.From("Pino Palladino"),
							"version": api.StringData{}.From("b"),
						},
					},
					{
						Fields: map[string]*api.Data{
							"id":      api.StringData{}.From("3"),
							"name":    api.StringData{}.From("Steve Jordan"),
							"version": api.StringData{}.From("c"),
						},
					},
				}

				src := make(chan *api.Record)
				var from <-chan *api.Record = src
				graphh.from.On("Fetch", mock.Anything).Run(func(args mock.Arguments) {
					go func(ctx context.Context) {
						defer GinkgoRecover()
						defer close(src)
						logger := logger.With(zap.String("role", "source"))
						for i := range data {
							select {
							case src <- data[i]:
								logger.Debug("sent record", zap.String("id", data[i].Fields["id"].Strings[0]))
							case <-ctx.Done():
								Fail("context canceled while sending data")
							}
						}
					}(args.Get(0).(context.Context))
				}).Return(from)
				graphh.from.On("Error").Return(nil)

				ack := make(chan *api.RecordStatus)
				var ackFrom <-chan *api.RecordStatus = ack
				graphh.to.On("Write", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					go func(ctx context.Context, src <-chan *api.Record) {
						defer GinkgoRecover()
						defer close(ack)
						logger := logger.With(zap.String("role", "destination"))
						for done := false; !done; {
							select {
							case r, ok := <-src:
								if ok {
									select {
									case ack <- &api.RecordStatus{
										Id:      r.Fields["id"].Strings[0],
										Version: r.Fields["version"].Strings[0],
										Error:   api.NoRecordError(),
									}:
										logger.Debug("acknowledged record", zap.String("id", r.Fields["id"].Strings[0]))
									case <-ctx.Done():
										Fail("context canceled while acknowledging a record")
									}
								} else {
									done = true
									logger.Info("all records received")
								}
							case <-ctx.Done():
								Fail("context canceled waiting for next record")
							}
						}
					}(args.Get(0).(context.Context), args.Get(1).(<-chan *api.Record))
				}).Return(ackFrom)
				graphh.to.On("Error").Return(nil)

				for i := range data {
					match := func(record *api.Record) func(recordStatus []*api.RecordStatus) bool {
						matcher := func(recordStatus []*api.RecordStatus) bool {
							return len(recordStatus) == 1 &&
								recordStatus[0].Id == record.Fields["id"].Strings[0] &&
								recordStatus[0].Version == record.Fields["version"].Strings[0] &&
								*recordStatus[0].Error == 0
						}
						return matcher
					}
					graphh.from.On("SetStatus", mock.Anything, mock.MatchedBy(match(data[i]))).Once().Return(nil)
				}

				Expect(syncd.Push(ctx, model, graphh.Source(model), as)).To(Succeed())
				graphh.from.AssertExpectations(GinkgoT())
				graphh.to.AssertExpectations(GinkgoT())
			})
			//}, SpecTimeout(500*time.Millisecond))
		})
	})
})

func TestServer(t *testing.T) {
	RegisterFailHandler(Fail)
	cfg, rep := GinkgoConfiguration()
	if d, ok := t.Deadline(); ok {
		cfg.Timeout = d.Sub(time.Now())
	}
	RunSpecs(t, "Server", cfg, rep)
}

type Graph struct {
	from From
	to   To
}

func (s *Graph) Source(_ string, _ ...graph.Filter) graph.Source {
	return &s.from
}

func (s *Graph) Destination() graph.Destination {
	return &s.to
}

type From struct {
	mock.Mock
}

func (f *From) Fetch(ctx context.Context) <-chan *api.Record {
	return f.MethodCalled("Fetch", ctx).Get(0).(<-chan *api.Record)
}

func (f *From) Error() error {
	return f.MethodCalled("Error").Error(0)
}

func (f *From) SetStatus(ctx context.Context, status ...*api.RecordStatus) error {
	return f.MethodCalled("SetStatus", ctx, status).Error(0)
}

type To struct {
	mock.Mock
}

func (t *To) Write(ctx context.Context, records <-chan *api.Record) <-chan *api.RecordStatus {
	return t.MethodCalled("Write", ctx, records).Get(0).(<-chan *api.RecordStatus)
}

func (t *To) Error() error {
	return t.MethodCalled("Error").Error(0)
}