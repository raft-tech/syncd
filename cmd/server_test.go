package cmd_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raft-tech/syncd/cmd"
	"github.com/raft-tech/syncd/cmd/helpers"
	"github.com/raft-tech/syncd/pkg/graph"
	"github.com/raft-tech/syncd/pkg/graph/postgres"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("serve", func() {

	args := []string{"serve"}

	Context("with config", func() {

		BeforeEach(func() {
			args = append(args, "--config", "build/config.yaml")
		})

		Context("with mocks", func() {

			factory := new(GraphFactory)
			artists := new(Graph)
			songs := new(Graph)
			performers := new(Graph)
			BeforeEach(func() {

				helpers.OverridePostgresGraphFactory(func(_ context.Context, config postgres.ConnectionConfig) (graph.Factory, error) {
					defer GinkgoRecover()
					Expect(config.ConnectionString).To(Equal("postgres://testing"))
					Expect(config.SyncTable).To(Equal("syncd.sync"))
					Expect(config.SequenceTable).To(Equal("syncd.sync_seq"))

					factory.On("Close", mock.Anything).Return(nil)
					factory.On("Build", mock.Anything, mock.MatchedBy(func(m *graph.Model) bool {
						return m.Name == "artists"
					})).Once().Return(artists, nil)
					factory.On("Build", mock.Anything, mock.MatchedBy(func(m *graph.Model) bool {
						return m.Name == "songs"
					})).Once().Return(songs, nil)
					factory.On("Build", mock.Anything, mock.MatchedBy(func(m *graph.Model) bool {
						return m.Name == "performers"
					})).Once().Return(performers, nil)

					return factory, nil
				})
			})

			AfterEach(func() {
				helpers.ResetPostgresGraphFactory()
				t := GinkgoT()
				factory.AssertExpectations(t)
				artists.AssertExpectations(t)
				songs.AssertExpectations(t)
				performers.AssertExpectations(t)
			})

			It("starts", func(ctx context.Context) {

				ctx, cancel := context.WithCancel(ctx)
				defer cancel()

				root := cmd.New()
				root.SetArgs(args)
				wg := sync.WaitGroup{}
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					Expect(root.ExecuteContext(ctx)).To(Succeed())
				}()

				Eventually(func() (*http.Response, error) {
					return http.Get("http://localhost:8081/healthz/ready")
				}).Should(HaveHTTPStatus(200))

				cancel()
				wg.Wait()
			})
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
