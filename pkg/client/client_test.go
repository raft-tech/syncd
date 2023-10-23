/*
 * Copyright (c) 2023. Raft, LLC
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package client_test

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raft-tech/syncd/internal/api"
	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/client"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
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
		var syncd client.Client
		BeforeEach(func(ctx context.Context) {
			var err error
			syncd, err = client.New(ctx, "buffered", client.WithDialOptions(
				grpc.WithCredentialsBundle(insecure.NewBundle()),
				grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
					return listener.DialContext(ctx)
				}),
			))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(syncd.Close()).To(Succeed())
		})

		var peer *Server
		var server *grpc.Server
		var serverErr error
		var serverWait sync.WaitGroup
		BeforeEach(func() {
			listener = bufconn.Listen(10 * 1024)
			peer = &Server{}
			server = grpc.NewServer()
			api.RegisterSyncServer(server, peer)
			serverWait.Add(1)
			go func() {
				serverErr = server.Serve(listener)
				serverWait.Done()
			}()
		})

		AfterEach(func() {
			server.Stop()
			serverWait.Wait()
			Expect(serverErr).ToNot(HaveOccurred())
		})

		var as string
		BeforeEach(func(ctx SpecContext) {
			as = ctx.SpecReport().LeafNodeText
			Expect(syncd.Connect(ctx)).To(Succeed())
		})

		It("Checks", func(ctx context.Context) {
			peer.On("Check", mock.Anything, mock.Anything).Return(api.CheckInfo(), nil)
			Expect(syncd.Check(log.NewContext(ctx, logger), "data", as)).To(And(BeTrue()))
		})

		It("Pushes", func(ctx context.Context) {

			peer.On("Push", mock.Anything).Run(func(args mock.Arguments) {
				defer GinkgoRecover()

				logger := logger.With(zap.String("role", "server"))
				server := args.Get(0).(api.Sync_PushServer)
				ctx := server.Context()

				var client, model string
				if md, ok := metadata.FromIncomingContext(ctx); ok {
					if s := md.Get("peer"); len(s) == 1 {
						client = s[0]
					}
					if s := md.Get("model"); len(s) == 1 {
						model = s[0]
					}
				}
				Expect(client).To(Equal(as))
				Expect(model).To(Equal("data"))

				for done := false; !done; {
					logger.Debug("waiting on client data")
					if rec, err := server.Recv(); err == nil {
						logger := logger.With(zap.String("id", rec.Fields["id"].Strings[0]))
						logger.Info("received record")
						Expect(server.Send(&api.RecordStatus{
							Id:      rec.Fields["id"].Strings[0],
							Version: rec.Fields["version"].Strings[0],
							Error:   api.NoRecordError(),
						})).To(Succeed())
					} else {
						Expect(err).To(MatchError(io.EOF))
						done = true
					}
				}

			}).Return(nil)

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

			records := make(chan *api.Record)
			var recordsOut <-chan *api.Record = records
			from := &From{}
			from.On("Fetch", mock.Anything).Run(func(args mock.Arguments) {
				defer GinkgoRecover()
				ctx := args.Get(0).(context.Context)
				go func(to chan<- *api.Record) {
					defer GinkgoRecover()
					defer close(records)
					for i := range data {
						select {
						case to <- data[i]:
						case <-ctx.Done():
							break
						}
					}
				}(records)
			}).Return(recordsOut)

			from.On("Error").Return(nil)

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
				from.On("SetStatus", mock.Anything, mock.MatchedBy(match(data[i]))).Once().Return(nil)
			}

			Expect(syncd.Push(log.NewContext(ctx, logger.With(zap.String("role", "client"))), "data", from, as)).To(Succeed())
			from.AssertExpectations(GinkgoT())
		}, SpecTimeout(500*time.Millisecond))

		It("Pulls", func(ctx context.Context) {

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

			peer.On("Pull", mock.Anything).Run(func(args mock.Arguments) {

				defer GinkgoRecover()

				logger := logger.With(zap.String("role", "server"))
				server := args.Get(0).(api.Sync_PullServer)
				ctx := server.Context()

				var client, model string
				if md, ok := metadata.FromIncomingContext(ctx); ok {
					if s := md.Get("peer"); len(s) == 1 {
						client = s[0]
					}
					if s := md.Get("model"); len(s) == 1 {
						model = s[0]
					}
				}
				Expect(client).To(Equal(as))
				Expect(model).To(Equal("data"))

				// Send Data
				logger.Debug("starting to send data")
				for i := range data {
					logger := logger.With(zap.String("id", data[i].Fields["id"].Strings[0]))
					logger.Debug("sending data")
					Expect(server.Send(data[i])).To(Succeed())
					logger.Info("sent data")
				}
				logger.Info("finished sending data")

			}).Return(nil)

			peer.On("Acknowledge", mock.Anything).Run(func(args mock.Arguments) {
				defer GinkgoRecover()

				logger := logger.With(zap.String("role", "ack"))
				server := args.Get(0).(api.Sync_AcknowledgeServer)
				ctx := server.Context()

				var client, model string
				if md, ok := metadata.FromIncomingContext(ctx); ok {
					if s := md.Get("peer"); len(s) == 1 {
						client = s[0]
					}
					if s := md.Get("model"); len(s) == 1 {
						model = s[0]
					}
				}
				Expect(client).To(Equal(as))
				Expect(model).To(Equal("data"))

				// Receive Responses
				for i := range data {
					logger := logger.With(zap.String("id", data[i].Fields["id"].Strings[0]))
					logger.Debug("expecting client data")
					Expect(server.Recv()).To(And(
						HaveField("Id", Equal(data[i].Fields["id"].Strings[0])),
						HaveField("Version", Equal(data[i].Fields["version"].Strings[0])),
						HaveField("Error", Equal(api.NoRecordError())),
					))
					logger.Info("received client data")
				}

				// EOF should be returned after final message received
				Expect(server.Recv()).Error().To(Equal(io.EOF))
			}).Return(nil)

			to := &To{}
			status := make(chan *api.RecordStatus)
			var statusOut <-chan *api.RecordStatus = status
			to.On("Write", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				// The destination work must be done in a go routine to allow the status channel to return
				go func(out chan<- *api.RecordStatus) {
					defer GinkgoRecover()
					defer close(out)
					logger := logger.With(zap.String("role", "destination"))
					ctx := args.Get(0).(context.Context)
					src := args.Get(1).(<-chan *api.Record)
					for done := false; !done; {
						logger.Debug("waiting on server data")
						select {
						case r, ok := <-src:
							if ok {
								logger := logger.With(zap.String("id", r.Fields["id"].Strings[0]))
								logger.Debug("sending record status")
								select {
								case out <- &api.RecordStatus{
									Id:      r.Fields["id"].Strings[0],
									Version: r.Fields["version"].Strings[0],
									Error:   api.NoRecordError(),
								}:
									logger.Info("record status sent")
								case <-ctx.Done():
									Fail("context canceled")
								}
							}
							done = !ok
						case <-ctx.Done():
							done = true
							Fail("context canceled")
						}
					}
				}(status)
			}).Return(statusOut)
			to.On("Error").Return(nil)

			Expect(syncd.Pull(log.NewContext(ctx, logger.With(zap.String("role", "client"))), "data", to, as)).To(Succeed())
			peer.AssertExpectations(GinkgoT())
		}, SpecTimeout(500*time.Millisecond))
	})
})

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	cfg, rep := GinkgoConfiguration()
	if d, ok := t.Deadline(); ok {
		cfg.Timeout = d.Sub(time.Now())
	}
	RunSpecs(t, "Client", cfg, rep)
}

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

func (s *Server) Pull(_ *api.PullRequest, server api.Sync_PullServer) error {
	return s.MethodCalled("Pull", server).Error(0)
}

func (s *Server) Acknowledge(server api.Sync_AcknowledgeServer) error {
	return s.MethodCalled("Acknowledge", server).Error(0)
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
