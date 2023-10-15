package client_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/api"
	"github.com/raft-tech/syncd/pkg/client"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
			Expect(syncd.Check(log.NewContext(ctx, logger), as)).To(And(BeTrue()))
		})

		It("Pulls", func(ctx context.Context) {

			wg := sync.WaitGroup{}
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

				// Send Data
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					logger.Debug("starting to send data")
					for i := range data {
						logger := logger.With(zap.String("id", data[i].Fields["id"].Strings[0]))
						logger.Debug("sending data")
						Expect(server.Send(data[i])).To(Succeed())
						logger.Info("sent data")
					}
					logger.Info("finished sending data")
				}()

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

				// gRPC's response channel will not close until this function returns,
				// so we must verify the request channel closes properly in a go routine
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					logger.Debug("waiting for client to close request channel")
					Expect(server.Recv()).Error().To(Equal(status.Error(codes.Canceled, "context canceled")))
					logger.Info("request channel closed")
				}()
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
			Expect(syncd.Close()).To(Succeed())
			wg.Wait()
		})
	})
})

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	cfg, rep := GinkgoConfiguration()
	rep.Verbose = true
	if d, ok := t.Deadline(); ok {
		cfg.Timeout = d.Sub(time.Now())
	}
	RunSpecs(t, "Client", cfg, rep)
}

type Server struct {
	mock.Mock
	api.UnimplementedSyncServer
}

func (m *Server) Check(ctx context.Context, info *api.Info) (*api.Info, error) {
	args := m.MethodCalled("Check", ctx, info)
	return args.Get(0).(*api.Info), args.Error(1)
}

func (m *Server) Pull(server api.Sync_PullServer) error {
	return m.MethodCalled("Pull", server).Error(0)
}

func (m *Server) Push(server api.Sync_PushServer) error {
	return m.MethodCalled("Push", server).Error(0)
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
