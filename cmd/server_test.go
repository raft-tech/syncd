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

package cmd_test

import (
	"context"
	"net/http"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raft-tech/syncd/cmd"
	"github.com/raft-tech/syncd/cmd/helpers"
	"github.com/raft-tech/syncd/internal/api"
	"github.com/raft-tech/syncd/pkg/client"
	"github.com/raft-tech/syncd/pkg/graph"
	"github.com/raft-tech/syncd/pkg/graph/postgres"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var _ = Describe("serve", func() {

	args := []string{"serve"}

	Context("with config", func() {

		Context("with mocks", func() {

			var factory *GraphFactory
			var artists *Graph
			var songs *Graph
			var performers *Graph

			Context("with server and client", Serial, func() {

				var dialOpts []grpc.DialOption
				var syncd api.SyncClient

				When("the client is authenticated", func() {

					It("handles checks", func(ctx context.Context) {
						ctx = metadata.AppendToOutgoingContext(ctx, "peer", "client", "model", "artists")
						if info, err := syncd.Check(ctx, &api.Info{ApiVersion: 1}); err == nil {
							Expect(info.ApiVersion).To(Equal(uint32(1)))
						} else {
							Expect(err).NotTo(HaveOccurred())
						}
					})

					BeforeEach(func(ctx context.Context) {
						dialOpts = append(dialOpts, client.WithPreSharedKey("cookiecookiecookiecookie"))
					})
				})

				When("the client is not authenticated", func() {

					It("refuses unauthenticated checks", func(ctx context.Context) {
						ctx = metadata.AppendToOutgoingContext(ctx, "peer", "client", "model", "artists")
						_, err := syncd.Check(ctx, &api.Info{ApiVersion: 1})
						Expect(err).To(MatchError(status.Error(codes.Unauthenticated, "Unauthenticated")))
					})
				})

				JustBeforeEach(func(ctx context.Context) {
					conn, err := grpc.DialContext(ctx, "localhost:8080", dialOpts...)
					DeferCleanup(conn.Close)
					Expect(err).NotTo(HaveOccurred())
					syncd = api.NewSyncClient(conn)
				})

				BeforeEach(func() {

					dialOpts = []grpc.DialOption{
						// As defined in configFile
						grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)),
						grpc.WithAuthority("syncd"),
					}

					root := cmd.New()
					root.SetOut(GinkgoWriter)
					root.SetArgs(args)

					wg := sync.WaitGroup{}
					ctx, cancel := context.WithCancel(context.Background())
					DeferCleanup(func() {
						By("by canceling the command context")
						cancel()
						wg.Wait()
					})
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()
						By("by executing 'syncd serve [ARGS]'")
						Expect(root.ExecuteContext(ctx)).To(Succeed())
					}()

					Eventually(func() (*http.Response, error) {
						return http.Get("http://localhost:8081/healthz")
					}).Should(HaveHTTPStatus(200))

					Eventually(func() (*http.Response, error) {
						return http.Get("http://localhost:8081/healthz/ready")
					}).Should(And(HaveHTTPStatus(200), HaveHTTPBody("READY")))

					Eventually(func() (*http.Response, error) {
						return http.Get("http://localhost:8081/metrics")
					}).Should(HaveHTTPStatus(200))
				})
			})

			BeforeEach(func() {

				factory = new(GraphFactory)
				artists = new(Graph)
				songs = new(Graph)
				performers = new(Graph)

				DeferCleanup(func(t FullGinkgoTInterface) {
					By("Asserting mock expectations")
					factory.AssertExpectations(t)
					artists.AssertExpectations(t)
					songs.AssertExpectations(t)
					performers.AssertExpectations(t)
				}, GinkgoT())

				helpers.OverridePostgresGraphFactory(func(_ context.Context, config postgres.ConnectionConfig) (graph.Factory, error) {
					defer GinkgoRecover()
					By("Overriding the Postgres Graph Factory")
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
				DeferCleanup(helpers.ResetPostgresGraphFactory)
			})
		})

		BeforeEach(func() {
			args = append(args, "--config", "build/config.yaml")
		})
	})
})
