package client_test

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/api"
	"github.com/raft-tech/syncd/pkg/client"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"net"
	"sync"
	"testing"
	"time"
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
		var syncClient client.Client
		BeforeEach(func(ctx context.Context) {
			var err error
			syncClient, err = client.New(ctx, "buffered", client.WithDialOptions(
				grpc.WithCredentialsBundle(insecure.NewBundle()),
				grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
					return listener.DialContext(ctx)
				}),
			))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(syncClient.Close()).To(Succeed())
		})

		var syncServer *mockServer
		var gServer *grpc.Server
		var serverErr error
		var serverWait sync.WaitGroup
		BeforeEach(func() {
			listener = bufconn.Listen(10 * 1024)
			syncServer = &mockServer{}
			gServer = grpc.NewServer()
			api.RegisterSyncServer(gServer, syncServer)
			serverWait.Add(1)
			go func() {
				serverErr = gServer.Serve(listener)
				serverWait.Done()
			}()
		})

		AfterEach(func() {
			gServer.Stop()
			serverWait.Wait()
			Expect(serverErr).ToNot(HaveOccurred())
		})

		BeforeEach(func(ctx context.Context) {
			Expect(syncClient.Connect(ctx)).To(Succeed())
		})

		It("Checks", func(ctx SpecContext) {
			syncServer.On("Check", mock.Anything, mock.Anything).Return(api.CheckInfo(), nil)
			Expect(syncClient.Check(log.NewContext(ctx, logger), ctx.SpecReport().LeafNodeText)).To(And(BeTrue()))
		})
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

type mockServer struct {
	mock.Mock
	api.UnimplementedSyncServer
}

func (m *mockServer) Check(ctx context.Context, info *api.Info) (*api.Info, error) {
	args := m.MethodCalled("Check", ctx, info)
	return args.Get(0).(*api.Info), args.Error(1)
}

func (m *mockServer) Pull(server api.Sync_PullServer) error {
	return m.MethodCalled("Pull", server).Error(0)
}

func (m *mockServer) Push(server api.Sync_PushServer) error {
	return m.MethodCalled("Push", server).Error(0)
}
