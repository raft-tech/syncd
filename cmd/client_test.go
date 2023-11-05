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

package cmd_test

import (
	"context"
	"io"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raft-tech/syncd/cmd"
	"github.com/raft-tech/syncd/cmd/helpers"
	"github.com/raft-tech/syncd/internal/api"
	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/graph"
	"github.com/raft-tech/syncd/pkg/graph/postgres"
	"github.com/raft-tech/syncd/pkg/server"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var _ = Describe("client", func() {

	var logger *zap.Logger
	var artistRecords []*api.Record
	var songRecords []*api.Record
	var performerRecords []*api.Record

	Context("with mocks", func() {

		var factory *GraphFactory
		var artists *Graph
		var songs *Graph
		var performers *Graph

		Context("with server", func() {

			var syncd *Server

			It("pulls once", func(ctx context.Context) {

				// Mock client destinations
				wg := sync.WaitGroup{}

				// Artists
				{
					// Server Pull
					syncd.On("Pull", mock.Anything, mock.MatchedBy(func(server api.Sync_PullServer) bool {
						md, ok := api.GetMetadataFromContext(server.Context())
						return ok && md.Model == "artists"
					})).Run(func(args mock.Arguments) {
						defer GinkgoRecover()
						logger.Info("handling artists pull")
						server := args.Get(1).(api.Sync_PullServer)
						for i := range artistRecords {
							logger.Debug("sending artist", zap.Int("index", i))
							Expect(server.Send(artistRecords[i])).To(Succeed())
							logger.Info("sent artist", zap.Int("index", i))
						}
						logger.Info("finished handling artists pull")
					}).Once().Return(nil)

					// Server Acknowledge
					syncd.On("Acknowledge", mock.MatchedBy(func(server api.Sync_AcknowledgeServer) bool {
						md, ok := api.GetMetadataFromContext(server.Context())
						return ok && md.Model == "artists"
					})).Run(func(args mock.Arguments) {
						defer GinkgoRecover()
						logger.Info("handling artists ack")
						server := args.Get(0).(api.Sync_AcknowledgeServer)
						for i := range artistRecords {
							rs, err := server.Recv()
							Expect(err).NotTo(HaveOccurred())
							Expect(rs).To(And(
								HaveField("Id", artistRecords[i].Fields["id"].Strings[0]),
								HaveField("Version", artistRecords[i].Fields["version"].Strings[0]),
								HaveField("Error", api.NoRecordError()),
							))
						}
						_, err := server.Recv()
						Expect(err).To(MatchError(io.EOF))
						logger.Info("finished handling artists ack")
					}).Once().Return(nil)

					// Client graph
					dest := new(Destination)
					DeferCleanup(dest.AssertExpectations, GinkgoT())
					status := make(chan *api.RecordStatus)
					var ack <-chan *api.RecordStatus = status
					artists.On("Destination").Return(dest)
					dest.On("Error").Return(nil)
					dest.On("Write", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
						defer GinkgoRecover()
						wg.Add(1)
						go func(ctx context.Context, from <-chan *api.Record) {
							defer wg.Done()
							defer GinkgoRecover()
							defer close(status)
							canceled := false
							for i := range artistRecords {
								if canceled {
									break
								}
								select {
								case in, ok := <-from:
									Expect(ok).To(BeTrue())
									Expect(in.Fields).To(And(
										HaveKeyWithValue("id", And(HaveField("Strings", And(
											HaveLen(1),
											ContainElement(artistRecords[i].Fields["id"].Strings[0]),
										)))),
										HaveKeyWithValue("name", And(HaveField("Strings", And(
											HaveLen(1),
											ContainElement(artistRecords[i].Fields["name"].Strings[0]),
										)))),
										HaveKeyWithValue("version", And(HaveField("Strings", And(
											HaveLen(1),
											ContainElement(artistRecords[i].Fields["version"].Strings[0]),
										)))),
									))
									select {
									case status <- &api.RecordStatus{
										Id:      in.Fields["id"].Strings[0],
										Version: in.Fields["version"].Strings[0],
										Error:   api.NoRecordError(),
									}:
									case <-ctx.Done():
										canceled = true
									}
								case <-ctx.Done():
									canceled = true
								}
							}
							_, ok := <-from
							Expect(ok).To(BeFalse())
						}(args.Get(0).(context.Context), args.Get(1).(<-chan *api.Record))
					}).Once().Return(ack)
				}

				// Songs
				{
					// Server Pull
					syncd.On("Pull", mock.Anything, mock.MatchedBy(func(server api.Sync_PullServer) bool {
						md, ok := api.GetMetadataFromContext(server.Context())
						return ok && md.Model == "songs"
					})).Run(func(args mock.Arguments) {
						defer GinkgoRecover()
						logger.Info("handling songs pull")
						server := args.Get(1).(api.Sync_PullServer)
						for i := range songRecords {
							logger.Debug("sending songs", zap.Int("index", i))
							Expect(server.Send(songRecords[i])).To(Succeed())
							logger.Info("sent song", zap.Int("index", i))
						}
						logger.Info("finished handling songs pull")
					}).Once().Return(nil)

					// Server Acknowledge
					syncd.On("Acknowledge", mock.MatchedBy(func(server api.Sync_AcknowledgeServer) bool {
						md, ok := api.GetMetadataFromContext(server.Context())
						return ok && md.Model == "songs"
					})).Run(func(args mock.Arguments) {
						defer GinkgoRecover()
						logger.Info("handling songs ack")
						server := args.Get(0).(api.Sync_AcknowledgeServer)
						for i := range songRecords {
							rs, err := server.Recv()
							Expect(err).NotTo(HaveOccurred())
							Expect(rs).To(And(
								HaveField("Id", songRecords[i].Fields["id"].Strings[0]),
								HaveField("Version", songRecords[i].Fields["version"].Strings[0]),
								HaveField("Error", api.NoRecordError()),
							))
						}
						_, err := server.Recv()
						Expect(err).To(MatchError(io.EOF))
						logger.Info("finished handling songs ack")
					}).Once().Return(nil)

					// Client graph
					dest := new(Destination)
					DeferCleanup(dest.AssertExpectations, GinkgoT())
					status := make(chan *api.RecordStatus)
					var ack <-chan *api.RecordStatus = status
					songs.On("Destination").Return(dest)
					dest.On("Error").Return(nil)
					dest.On("Write", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
						defer GinkgoRecover()
						wg.Add(1)
						go func(ctx context.Context, from <-chan *api.Record) {
							defer wg.Done()
							defer GinkgoRecover()
							defer close(status)
							canceled := false
							for i := range songRecords {
								if canceled {
									break
								}
								select {
								case in, ok := <-from:
									Expect(ok).To(BeTrue())
									Expect(in.Fields).To(And(
										HaveKeyWithValue("id", And(HaveField("Strings", And(
											HaveLen(1),
											ContainElement(songRecords[i].Fields["id"].Strings[0]),
										)))),
										HaveKeyWithValue("name", And(HaveField("Strings", And(
											HaveLen(1),
											ContainElement(songRecords[i].Fields["name"].Strings[0]),
										)))),
										HaveKeyWithValue("composer", And(HaveField("Strings", And(
											HaveLen(1),
											ContainElement(songRecords[i].Fields["composer"].Strings[0]),
										)))),
										HaveKeyWithValue("publisher", And(HaveField("Strings", And(
											HaveLen(1),
											ContainElement(songRecords[i].Fields["publisher"].Strings[0]),
										)))),
										HaveKeyWithValue("version", And(HaveField("Strings", And(
											HaveLen(1),
											ContainElement(songRecords[i].Fields["version"].Strings[0]),
										)))),
									))
									select {
									case status <- &api.RecordStatus{
										Id:      in.Fields["id"].Strings[0],
										Version: in.Fields["version"].Strings[0],
										Error:   api.NoRecordError(),
									}:
									case <-ctx.Done():
										canceled = true
									}
								case <-ctx.Done():
									canceled = true
								}
							}
							_, ok := <-from
							Expect(ok).To(BeFalse())
						}(args.Get(0).(context.Context), args.Get(1).(<-chan *api.Record))
					}).Once().Return(ack)
				}

				By("executing syncd pull --config ")
				root := cmd.New()
				root.SetOut(GinkgoWriter)
				root.SetArgs([]string{"pull", "--config", "build/config.yaml"})

				Expect(root.ExecuteContext(ctx)).To(Succeed())

				done := false
				go func(b *bool) {
					wg.Wait()
					*b = true
				}(&done)
				Eventually(func() bool { return done }).Should(BeTrue())
			})

			It("pushes once", func(ctx context.Context) {

				// Mock client sources
				wg := sync.WaitGroup{}

				// Artists
				{
					// Server
					syncd.On("Push", mock.MatchedBy(func(server api.Sync_PushServer) bool {
						md, ok := api.GetMetadataFromContext(server.Context())
						return ok && md.Model == "artists"
					})).Run(func(args mock.Arguments) {
						defer GinkgoRecover()
						logger.Info("handling artists push")
						server := args.Get(0).(api.Sync_PushServer)
						for i := range artistRecords {
							rec, err := server.Recv()
							Expect(err).NotTo(HaveOccurred())
							Expect(rec.Fields).To(And(
								HaveKeyWithValue("id", And(HaveField("Strings", And(
									HaveLen(1),
									ContainElement(artistRecords[i].Fields["id"].Strings[0]),
								)))),
								HaveKeyWithValue("name", And(HaveField("Strings", And(
									HaveLen(1),
									ContainElement(artistRecords[i].Fields["name"].Strings[0]),
								)))),
								HaveKeyWithValue("version", And(HaveField("Strings", And(
									HaveLen(1),
									ContainElement(artistRecords[i].Fields["version"].Strings[0]),
								)))),
							))
							Expect(server.Send(&api.RecordStatus{
								Id:      rec.Fields["id"].Strings[0],
								Version: rec.Fields["version"].Strings[0],
								Error:   api.NoRecordError(),
							})).To(Succeed())
						}
						logger.Info("finished handling songs pull")
					}).Once().Return(nil)

					// Clients
					source := new(Source)
					DeferCleanup(source.AssertExpectations, GinkgoT())
					artists.On("Source", "bravo", mock.MatchedBy(func(filters []graph.Filter) bool {
						return len(filters) == 1 &&
							filters[0].Key == "name" &&
							filters[0].Operator == graph.InOperator &&
							reflect.DeepEqual(filters[0].Value, []interface{}{"John Mayer", "Steve Jordan", "Pino Palladino"})
					})).Return(source)
					source.On("Error").Return(nil)
					records := make(chan *api.Record)
					var fetch <-chan *api.Record = records
					source.On("Fetch", mock.Anything).Run(func(args mock.Arguments) {
						wg.Add(1)
						go func(to chan<- *api.Record) {
							defer GinkgoRecover()
							defer wg.Done()
							defer close(records)
							canceled := false
							for i := range artistRecords {
								if canceled {
									break
								}
								select {
								case to <- artistRecords[i]:
								case <-ctx.Done():
									canceled = true
								}
							}
						}(records)
					}).Return(fetch)
					for i := range artistRecords {
						expected := artistRecords[i]
						source.On("SetStatus", mock.Anything, mock.MatchedBy(func(status []*api.RecordStatus) bool {
							return len(status) == 1 &&
								status[0].Id == expected.Fields["id"].Strings[0] &&
								status[0].Version == expected.Fields["version"].Strings[0] &&
								*status[0].Error == *api.NoRecordError()
						})).Once().Return(nil)
					}
				}

				// Songs
				{
					// Server
					syncd.On("Push", mock.MatchedBy(func(server api.Sync_PushServer) bool {
						md, ok := api.GetMetadataFromContext(server.Context())
						return ok && md.Model == "songs"
					})).Run(func(args mock.Arguments) {
						defer GinkgoRecover()
						logger.Info("handling songs push")
						server := args.Get(0).(api.Sync_PushServer)
						for i := range songRecords {
							rec, err := server.Recv()
							Expect(err).NotTo(HaveOccurred())
							Expect(rec.Fields).To(And(
								HaveKeyWithValue("id", And(HaveField("Strings", And(
									HaveLen(1),
									ContainElement(songRecords[i].Fields["id"].Strings[0]),
								)))),
								HaveKeyWithValue("name", And(HaveField("Strings", And(
									HaveLen(1),
									ContainElement(songRecords[i].Fields["name"].Strings[0]),
								)))),
								HaveKeyWithValue("composer", And(HaveField("Strings", And(
									HaveLen(1),
									ContainElement(songRecords[i].Fields["composer"].Strings[0]),
								)))),
								HaveKeyWithValue("publisher", And(HaveField("Strings", And(
									HaveLen(1),
									ContainElement(songRecords[i].Fields["publisher"].Strings[0]),
								)))),
								HaveKeyWithValue("version", And(HaveField("Strings", And(
									HaveLen(1),
									ContainElement(songRecords[i].Fields["version"].Strings[0]),
								)))),
							))
							Expect(server.Send(&api.RecordStatus{
								Id:      rec.Fields["id"].Strings[0],
								Version: rec.Fields["version"].Strings[0],
								Error:   api.NoRecordError(),
							})).To(Succeed())
						}
						logger.Info("finished handling songs pull")
					}).Once().Return(nil)

					// Clients
					source := new(Source)
					DeferCleanup(source.AssertExpectations, GinkgoT())
					songs.On("Source", "bravo", mock.MatchedBy(func(filters []graph.Filter) bool {
						return len(filters) == 1 &&
							filters[0].Key == "publisher" &&
							filters[0].Operator == graph.InOperator &&
							reflect.DeepEqual(filters[0].Value, []interface{}{"Aware", "Bluebird", "Sony Music Entertainment"})
					})).Once().Return(source)
					source.On("Error").Return(nil)
					records := make(chan *api.Record)
					var fetch <-chan *api.Record = records
					source.On("Fetch", mock.Anything).Run(func(args mock.Arguments) {
						wg.Add(1)
						go func(to chan<- *api.Record) {
							defer GinkgoRecover()
							defer wg.Done()
							defer close(records)
							canceled := false
							for i := range songRecords {
								if canceled {
									break
								}
								select {
								case to <- songRecords[i]:
								case <-ctx.Done():
									canceled = true
								}
							}
						}(records)
					}).Return(fetch)
					for i := range songRecords {
						expected := songRecords[i]
						source.On("SetStatus", mock.Anything, mock.MatchedBy(func(status []*api.RecordStatus) bool {
							return len(status) == 1 &&
								status[0].Id == expected.Fields["id"].Strings[0] &&
								status[0].Version == expected.Fields["version"].Strings[0] &&
								*status[0].Error == *api.NoRecordError()
						})).Once().Return(nil)
					}
				}

				By("executing syncd push --config ")
				root := cmd.New()
				root.SetOut(GinkgoWriter)
				root.SetArgs([]string{"push", "--config", "build/config.yaml"})

				Expect(root.ExecuteContext(ctx)).To(Succeed())

				done := false
				go func(b *bool) {
					wg.Wait()
					*b = true
				}(&done)
				Eventually(func() bool { return done }).Should(BeTrue())
			})

			It("pulls continuously", func(ctx context.Context) {

				// Mock client destinations
				wg := sync.WaitGroup{}

				// Artists
				artistsCounter := new(atomic.Uint32)
				{
					logger := logger.With(zap.String("model", "artists"))

					// Server Pull
					syncd.On("Pull", mock.Anything, mock.MatchedBy(func(server api.Sync_PullServer) bool {
						md, ok := api.GetMetadataFromContext(server.Context())
						return ok && md.Model == "artists"
					})).Return(nil)

					// Server Acknowledge
					syncd.On("Acknowledge", mock.MatchedBy(func(server api.Sync_AcknowledgeServer) bool {
						md, ok := api.GetMetadataFromContext(server.Context())
						return ok && md.Model == "artists"
					})).Run(func(args mock.Arguments) {
						defer GinkgoRecover()
						i := artistsCounter.Load() + 1
						server := args.Get(0).(api.Sync_AcknowledgeServer)
						logger.Info("started ack server", zap.Uint32("iteration", i))
						_, err := server.Recv()
						Expect(err).To(MatchError(io.EOF))
						logger.Info("ack server completed", zap.Uint32("iteration", i))
					}).Return(nil)

					// Client graph
					dest := new(Destination)
					DeferCleanup(dest.AssertExpectations, GinkgoT())
					var ack <-chan *api.RecordStatus
					artists.On("Destination").Return(dest)
					dest.On("Error").Return(nil)
					dest.On("Write", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
						defer GinkgoRecover()
						i := artistsCounter.Load() + 1
						logger.Info("creating ack channel", zap.Uint32("iteration", i))
						status := make(chan *api.RecordStatus)
						ack = status
						wg.Add(1)
						go func(ctx context.Context, from <-chan *api.Record) {
							defer GinkgoRecover()
							defer wg.Done()
							defer func() {
								close(status)
								logger.Info("ack channel closed", zap.Uint32("iteration", i))
							}()
							_, ok := <-from
							Expect(ok).To(BeFalse())
							artistsCounter.Add(1)
							logger.Info("finished pulling artists", zap.Uint32("iteration", i))
						}(args.Get(0).(context.Context), args.Get(1).(<-chan *api.Record))
					}).Return(func() <-chan *api.RecordStatus {
						return ack
					})
				}

				// Songs
				songsCounter := new(atomic.Uint32)
				{
					logger := logger.With(zap.String("model", "songs"))
					// Server Pull
					syncd.On("Pull", mock.Anything, mock.MatchedBy(func(server api.Sync_PullServer) bool {
						md, ok := api.GetMetadataFromContext(server.Context())
						return ok && md.Model == "songs"
					})).Return(nil)

					// Server Acknowledge
					syncd.On("Acknowledge", mock.MatchedBy(func(server api.Sync_AcknowledgeServer) bool {
						md, ok := api.GetMetadataFromContext(server.Context())
						return ok && md.Model == "songs"
					})).Run(func(args mock.Arguments) {
						defer GinkgoRecover()
						i := artistsCounter.Load() + 1
						server := args.Get(0).(api.Sync_AcknowledgeServer)
						logger.Info("started ack server", zap.Uint32("iteration", i))
						_, err := server.Recv()
						Expect(err).To(MatchError(io.EOF))
						logger.Info("ack server completed", zap.Uint32("iteration", i))
					}).Return(nil)

					// Client graph
					dest := new(Destination)
					DeferCleanup(dest.AssertExpectations, GinkgoT())
					var ack <-chan *api.RecordStatus
					songs.On("Destination").Return(dest)
					dest.On("Error").Return(nil)
					dest.On("Write", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
						defer GinkgoRecover()
						i := artistsCounter.Load() + 1
						logger.Info("creating ack channel", zap.Uint32("iteration", i))
						status := make(chan *api.RecordStatus)
						ack = status
						wg.Add(1)
						go func(ctx context.Context, from <-chan *api.Record) {
							defer GinkgoRecover()
							defer wg.Done()
							defer func() {
								close(status)
								logger.Info("ack channel closed", zap.Uint32("iteration", i))
							}()
							_, ok := <-from
							Expect(ok).To(BeFalse())
							songsCounter.Add(1)
							logger.Info("finished pulling artists", zap.Uint32("iteration", i))
						}(args.Get(0).(context.Context), args.Get(1).(<-chan *api.Record))
					}).Return(func() <-chan *api.RecordStatus {
						return ack
					})
				}

				By("executing syncd pull --config --continuous 10ms")
				root := cmd.New()
				root.SetOut(GinkgoWriter)
				root.SetArgs([]string{"pull", "--config", "build/config.yaml", "--continuous", "10ms"})

				ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
				defer cancel()
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					Expect(root.ExecuteContext(ctx)).To(Succeed())
				}()
				Eventually(artistsCounter.Load).
					WithTimeout(70 * time.Millisecond).
					WithPolling(5 * time.Millisecond).
					Should(BeNumerically(">=", uint32(5)))
				Eventually(songsCounter.Load).
					WithTimeout(70 * time.Millisecond).
					WithPolling(5 * time.Millisecond).
					Should(BeNumerically(">=", uint32(5)))

				cancel()
				wg.Wait()
			})

			It("pushes continuously", func(ctx context.Context) {

				// Mock client destinations
				wg := sync.WaitGroup{}

				// Artists
				artistsCounter := new(atomic.Uint32)
				{
					logger := logger.With(zap.String("model", "artists"))

					// Server
					syncd.On("Push", mock.MatchedBy(func(server api.Sync_PushServer) bool {
						md, ok := api.GetMetadataFromContext(server.Context())
						return ok && md.Model == "artists"
					})).Run(func(args mock.Arguments) {
						defer GinkgoRecover()
						i := artistsCounter.Load() + 1
						server := args.Get(0).(api.Sync_PushServer)
						logger.Info("started push server", zap.Uint32("iteration", i))
						_, err := server.Recv()
						Expect(err).To(MatchError(io.EOF))
						logger.Info("push server completed", zap.Uint32("iteration", i))
					}).Return(nil)

					// Client graph
					source := new(Source)
					DeferCleanup(source.AssertExpectations, GinkgoT())
					artists.On("Source", "bravo", mock.MatchedBy(func(filters []graph.Filter) bool {
						return len(filters) == 1 &&
							filters[0].Key == "name" &&
							filters[0].Operator == graph.InOperator &&
							reflect.DeepEqual(filters[0].Value, []interface{}{"John Mayer", "Steve Jordan", "Pino Palladino"})
					})).Return(source)
					source.On("Error").Return(nil)
					var fetch <-chan *api.Record
					source.On("Fetch", mock.Anything).Run(func(args mock.Arguments) {
						wg.Add(1)
						records := make(chan *api.Record)
						fetch = records
						go func(to chan<- *api.Record) {
							defer GinkgoRecover()
							defer wg.Done()
							defer close(to)
							artistsCounter.Add(1)
						}(records)
					}).Return(func() <-chan *api.Record { return fetch })
				}

				// Songs
				songsCounter := new(atomic.Uint32)
				{
					logger := logger.With(zap.String("model", "songs"))

					// Server
					syncd.On("Push", mock.MatchedBy(func(server api.Sync_PushServer) bool {
						md, ok := api.GetMetadataFromContext(server.Context())
						return ok && md.Model == "songs"
					})).Run(func(args mock.Arguments) {
						defer GinkgoRecover()
						i := artistsCounter.Load() + 1
						server := args.Get(0).(api.Sync_PushServer)
						logger.Info("started push server", zap.Uint32("iteration", i))
						_, err := server.Recv()
						Expect(err).To(MatchError(io.EOF))
						logger.Info("push server completed", zap.Uint32("iteration", i))
					}).Return(nil)

					// Client graph
					source := new(Source)
					DeferCleanup(source.AssertExpectations, GinkgoT())
					songs.On("Source", "bravo", mock.MatchedBy(func(filters []graph.Filter) bool {
						return len(filters) == 1 &&
							filters[0].Key == "publisher" &&
							filters[0].Operator == graph.InOperator &&
							reflect.DeepEqual(filters[0].Value, []interface{}{"Aware", "Bluebird", "Sony Music Entertainment"})
					})).Return(source)
					source.On("Error").Return(nil)
					var fetch <-chan *api.Record
					source.On("Fetch", mock.Anything).Run(func(args mock.Arguments) {
						wg.Add(1)
						records := make(chan *api.Record)
						fetch = records
						go func(to chan<- *api.Record) {
							defer GinkgoRecover()
							defer wg.Done()
							defer close(to)
							songsCounter.Add(1)
						}(records)
					}).Return(func() <-chan *api.Record { return fetch })
				}

				By("executing syncd push --config --continuous 10ms")
				root := cmd.New()
				root.SetOut(GinkgoWriter)
				root.SetArgs([]string{"push", "--config", "build/config.yaml", "--continuous", "10ms"})

				ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
				defer cancel()
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					Expect(root.ExecuteContext(ctx)).To(Succeed())
				}()
				Eventually(artistsCounter.Load).
					WithTimeout(70 * time.Millisecond).
					WithPolling(5 * time.Millisecond).
					Should(BeNumerically(">=", uint32(5)))
				Eventually(songsCounter.Load).
					WithTimeout(70 * time.Millisecond).
					WithPolling(5 * time.Millisecond).
					Should(BeNumerically(">=", uint32(5)))

				cancel()
				wg.Wait()
			})

			BeforeEach(func() {

				syncd = new(Server)
				DeferCleanup(syncd.AssertExpectations, GinkgoT())

				srv := grpc.NewServer(
					grpc.Creds(credentials.NewTLS(serverTLS)),
					grpc.ChainUnaryInterceptor(
						func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
							ctx = log.NewContext(ctx, logger)
							return handler(ctx, req)
						},
						server.PreSharedKey("cookiecookiecookiecookie").UnaryInterceptor, // defined in configFile
					),
					grpc.ChainStreamInterceptor(
						func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
							ctx := log.NewContext(ss.Context(), logger)
							return handler(srv, server.ServerStreamWithContext(ss, ctx))
						},
						server.PreSharedKey("cookiecookiecookiecookie").StreamInterceptor, // defined in configFile
					),
				)
				api.RegisterSyncServer(srv, syncd)

				listener, err := net.Listen("tcp", ":8080")
				Expect(err).NotTo(HaveOccurred())
				go func() {
					Expect(srv.Serve(listener)).NotTo(HaveOccurred())
				}()
				DeferCleanup(srv.Stop)
			})
		})

		BeforeEach(func() {

			factory = new(GraphFactory)
			artists = new(Graph)
			songs = new(Graph)
			performers = new(Graph)

			DeferCleanup(func(t FullGinkgoTInterface) {
				By("asserting mock expectations")
				factory.AssertExpectations(t)
				artists.AssertExpectations(t)
				songs.AssertExpectations(t)
				performers.AssertExpectations(t)
			}, GinkgoT())

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
			DeferCleanup(helpers.ResetPostgresGraphFactory)
		})
	})

	BeforeEach(func(ctx SpecContext) {

		logger = zap.New(
			zapcore.NewCore(zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()), zapcore.AddSync(GinkgoWriter), zapcore.DebugLevel),
			zap.Fields(zap.String("test", ctx.SpecReport().LeafNodeText)),
		)
		DeferCleanup(logger.Sync)

		artistRecords = []*api.Record{
			{
				Fields: map[string]*api.Data{
					"id":      api.StringData{}.From("1"),
					"name":    api.StringData{}.From("John Mayer"),
					"version": api.StringData{}.From(time.Now().String()),
				},
			},
			{
				Fields: map[string]*api.Data{
					"id":      api.StringData{}.From("2"),
					"name":    api.StringData{}.From("Steve Jordan"),
					"version": api.StringData{}.From(time.Now().String()),
				},
			},
			{
				Fields: map[string]*api.Data{
					"id":      api.StringData{}.From("3"),
					"name":    api.StringData{}.From("Pino Palladino"),
					"version": api.StringData{}.From(time.Now().String()),
				},
			},
		}

		songRecords = []*api.Record{
			{
				Fields: map[string]*api.Data{
					"id":        api.StringData{}.From("1"),
					"name":      api.StringData{}.From("Everyday I Have the Blues"),
					"composer":  api.StringData{}.From("Aaron Sparks"),
					"publisher": api.StringData{}.From("Bluebird"),
					"version":   api.StringData{}.From(time.Now().String()),
				},
			},
			{
				Fields: map[string]*api.Data{
					"id":        api.StringData{}.From("2"),
					"name":      api.StringData{}.From("Wait Until Tomorrow"),
					"composer":  api.StringData{}.From("Jimi Hendrix"),
					"publisher": api.StringData{}.From("Sony Music Entertainment"),
					"version":   api.StringData{}.From(time.Now().String()),
				},
			},
			{
				Fields: map[string]*api.Data{
					"id":        api.StringData{}.From("3"),
					"name":      api.StringData{}.From("Who Did You Think I Was"),
					"composer":  api.StringData{}.From("John Mayer"),
					"publisher": api.StringData{}.From("Aware"),
					"version":   api.StringData{}.From(time.Now().String()),
				},
			},
		}
		_ = songRecords // hack unused var

		performerRecords = []*api.Record{
			{
				Fields: map[string]*api.Data{
					"id":    api.StringData{}.From("1"),
					"name":  api.StringData{}.From("John Mayer Trio"),
					"label": api.StringData{}.From("Sony Music Entertainment"),
					"artists": {
						Type:    api.DataType_STRING,
						IsList:  true,
						Strings: []string{"1", "2", "3"},
					},
					"performances": {
						Type:   api.DataType_RECORD,
						IsList: true,
						Records: []*api.Record{
							{
								Fields: map[string]*api.Data{
									"song":     api.StringData{}.From("1"),
									"sequence": api.IntData{}.From(1),
								},
							},
							{
								Fields: map[string]*api.Data{
									"song":     api.StringData{}.From("2"),
									"sequence": api.IntData{}.From(2),
								},
							},
							{
								Fields: map[string]*api.Data{
									"song":     api.StringData{}.From("3"),
									"sequence": api.IntData{}.From(3),
								},
							},
						},
					},
					"version": api.StringData{}.From(time.Now().String()),
				},
			},
		}
		_ = performerRecords // Hack unused var
	})
})
