package postgres

import (
	"context"
	_ "embed"
	"github.com/jackc/pgx/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/api"
	"github.com/raft-tech/syncd/pkg/graph"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"testing"
)

//go:embed test_data.sql
var testSQL string

var _ = Describe("Factory", func() {

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

	// Create a shared *pgxFactory and reset the database for each test
	var factory *pgxFactory
	BeforeEach(func(ctx context.Context) {

		ctx = log.NewContext(ctx, logger)

		if testing.Short() {
			Skip("Skipping due to -short")
		}

		var connString string
		if c := os.Getenv("SYNCD_POSTGRESQL_CONN"); c != "" {
			connString = c
		} else {
			Skip("Skipping TestPostgreSQL due to undefined SYNCD_POSTRESQL_CONN env variable")
		}

		if c, err := pgx.Connect(ctx, connString); err == nil {
			_, err = c.Exec(ctx, testSQL)
			Expect(err).ToNot(HaveOccurred())
			Expect(c.Close(ctx)).To(Succeed())
		} else {
			logger.Error("failed to connect to database", zap.Error(err))
			Fail("unable to connect to database")
		}

		fixture, err := New(ctx, ConnectionConfig{
			ConnectionString: connString,
			SyncTable:        "syncd.sync",
			SequenceTable:    "syncd.sync_seq",
		})
		if f, ok := fixture.(*pgxFactory); err == nil && ok {
			factory = f
		} else {
			Fail("unexpected ")
		}
	})

	AfterEach(func(ctx context.Context) {
		if factory != nil {
			if e := factory.Close(ctx); e != nil {
				logger.Error("error closing factory", zap.Error(e))
				Fail(e.Error())
			}
		}
	})

	Context("with artists model", func() {

		var artists *pgxGraph
		BeforeEach(func(ctx context.Context) {
			ctx = log.NewContext(ctx, logger)
			fixture, err := factory.Build(ctx, &graph.Model{
				Name: "artists",
				Table: graph.Table{
					Name:         "syncd.artists",
					KeyField:     "id",
					VersionField: "version",
				},
			})
			Expect(fixture, err).ShouldNot(BeNil())
			artists = fixture.(*pgxGraph)
			Expect(artists).ShouldNot(BeNil())
		})

		It("graphs the artists model", func() {

			By("generating a sync query")
			Expect(artists.syncQuery).To(Equal(
				"SELECT model.id, model.version FROM syncd.artists AS model LEFT JOIN (SELECT id, version FROM syncd.sync WHERE peer = $1 AND model = 'artists') AS sync ON sync.id = model.id WHERE sync.version IS NULL or sync.version != model.version",
			))

			By("describing the table")
			Expect(artists.columns).To(ContainElements(
				Column{
					Name:     "id",
					Position: 1,
					Type:     api.StringData{},
				},
				Column{
					Name:     "name",
					Position: 2,
					Type:     api.StringData{},
				},
				Column{
					Name:     "version",
					Position: 3,
					Type:     api.StringData{},
				},
			))

			By("generating queries")
			Expect(artists.model.readQuery).To(Equal("SELECT name, version FROM syncd.artists WHERE id = $1 FOR SHARE"))
			Expect(artists.model.lockQuery).To(Equal("SELECT version FROM syncd.artists WHERE id = $1 FOR UPDATE"))
			Expect(artists.model.updateStatement).To(Equal("UPDATE syncd.artists SET name = $2, version = $3 WHERE id = $1"))
			Expect(artists.model.insertStatement).To(Equal("INSERT INTO syncd.artists (id, name, version) VALUES ($1, $2, $3)"))
			Expect(artists.model.deleteStatement).To(BeEmpty())
		})

		It("fetches", func(ctx context.Context) {
			ctx = log.NewContext(ctx, logger)
			lennon := &api.Data{}
			Expect(artists.model.Fetch(ctx, artists.Pool, "8b9835e6-9928-4f6a-afba-ea3e80cf89d0", lennon)).Should(BeNil())
			Expect(lennon.Records).To(Equal([]*api.Record{
				{
					Fields: map[string]*api.Data{
						"id":      api.StringData{}.From("8b9835e6-9928-4f6a-afba-ea3e80cf89d0"),
						"name":    api.StringData{}.From("John Lennon"),
						"version": api.StringData{}.From("1"),
					},
				},
			}))
		})

		It("it publishes", func(ctx context.Context) {

			ctx = log.NewContext(ctx, logger)
			peer := "postgres_factory"
			source := artists.Source(peer)

			By("retrieving unsynced artists")
			in := source.Fetch(ctx)
			var data []*api.Data
			for done := false; !done; {
				select {
				case a, ok := <-in:
					if ok {
						data = append(data, a)
					} else {
						done = true
					}
				case _ = <-ctx.Done():
					Fail("context canceled before completion")
				}
			}
			Expect(data).To(HaveLen(4))
			Expect(data).To(ContainElements(
				And(
					HaveField("Type", Equal(api.DataType_RECORD)),
					HaveField("IsList", BeFalse()),
					HaveField("Records", And(
						HaveLen(1),
						ContainElement(&api.Record{
							Fields: map[string]*api.Data{
								"id":      api.StringData{}.From("8b9835e6-9928-4f6a-afba-ea3e80cf89d0"),
								"name":    api.StringData{}.From("John Lennon"),
								"version": api.StringData{}.From("1"),
							},
						}),
					)),
				),
				And(
					HaveField("Type", Equal(api.DataType_RECORD)),
					HaveField("IsList", BeFalse()),
					HaveField("Records", And(
						HaveLen(1),
						ContainElement(&api.Record{
							Fields: map[string]*api.Data{
								"id":      api.StringData{}.From("253cedce-05ed-44bc-a49d-6f570a18b2ef"),
								"name":    api.StringData{}.From("Paul McCartney"),
								"version": api.StringData{}.From("1"),
							},
						}),
					)),
				),
				And(
					HaveField("Type", Equal(api.DataType_RECORD)),
					HaveField("IsList", BeFalse()),
					HaveField("Records", And(
						HaveLen(1),
						ContainElement(&api.Record{
							Fields: map[string]*api.Data{
								"id":      api.StringData{}.From("57ecbe75-ba4c-4094-acca-d066b6c6d058"),
								"name":    api.StringData{}.From("Ringo Starr"),
								"version": api.StringData{}.From("1"),
							},
						}),
					)),
				),
				And(
					HaveField("Type", Equal(api.DataType_RECORD)),
					HaveField("IsList", BeFalse()),
					HaveField("Records", And(
						HaveLen(1),
						ContainElement(&api.Record{
							Fields: map[string]*api.Data{
								"id":      api.StringData{}.From("88a6845b-e286-49a0-aa09-d89309d12d33"),
								"name":    api.StringData{}.From("George Harrison"),
								"version": api.StringData{}.From("1"),
							},
						}),
					)),
				),
			))

			By("updating sync status")
			var status []*api.RecordStatus
			for i := range data {
				status = append(status, &api.RecordStatus{
					Id:      data[i].Records[0].Fields["id"].Strings[0],
					Version: data[i].Records[0].Fields["version"].Strings[0],
				})
			}
			Expect(source.SetStatus(ctx, status...)).To(Succeed())

			By("only retrieving unsynced records")
			_, ok := <-source.Fetch(ctx)
			Expect(ok).To(BeFalse())
		})

		It("consumes", func(ctx context.Context) {

			ctx = log.NewContext(ctx, logger)
			dst := artists.Destination()

			data := make(chan *api.Data)
			go func(ctx context.Context, out chan<- *api.Data) {
				defer close(out)
				for _, d := range []*api.Data{
					{
						Type: api.DataType_RECORD,
						Records: []*api.Record{
							{
								Fields: map[string]*api.Data{
									"id":      api.StringData{}.From("384fc100-740c-43b0-bde2-106c796e2b8b"),
									"name":    api.StringData{}.From("Chester Bennington"),
									"version": api.StringData{}.From("1"),
								},
							},
						},
					},
					{
						Type: api.DataType_RECORD,
						Records: []*api.Record{
							{
								Fields: map[string]*api.Data{
									"id":      api.StringData{}.From("a3fc39bc-5144-4442-8f93-22b1e8a6fb7e"),
									"name":    api.StringData{}.From("Mike Shinoda"),
									"version": api.StringData{}.From("1"),
								},
							},
						},
					},
					{
						Type: api.DataType_RECORD,
						Records: []*api.Record{
							{
								Fields: map[string]*api.Data{
									"id":      api.StringData{}.From("99037e22-5c87-483b-97e1-8f54fbd26215"),
									"name":    api.StringData{}.From("Joe Hahn"),
									"version": api.StringData{}.From("1"),
								},
							},
						},
					},
					{
						Type: api.DataType_RECORD,
						Records: []*api.Record{
							{
								Fields: map[string]*api.Data{
									"id":      api.StringData{}.From("e2db93ce-33ce-4e69-abfc-e0d1ccee503d"),
									"name":    api.StringData{}.From("Rob Bourdon"),
									"version": api.StringData{}.From("1"),
								},
							},
						},
					},
					{
						Type: api.DataType_RECORD,
						Records: []*api.Record{
							{
								Fields: map[string]*api.Data{
									"id":      api.StringData{}.From("cd40483b-7828-464c-9373-19241585c12d"),
									"name":    api.StringData{}.From("Dave Farrel"),
									"version": api.StringData{}.From("1"),
								},
							},
						},
					},
					{
						Type: api.DataType_RECORD,
						Records: []*api.Record{
							{
								Fields: map[string]*api.Data{
									"id":      api.StringData{}.From("72b373fa-da8d-42c4-8b4a-c938ff6e6932"),
									"name":    api.StringData{}.From("Brad Delson"),
									"version": api.StringData{}.From("1"),
								},
							},
						},
					},
				} {
					select {
					case out <- d:
					case _ = <-ctx.Done():
						Skip("context canceled")
					}
				}
			}(ctx, data)

			var status []*api.RecordStatus
			for s := range dst.Write(ctx, data) {
				status = append(status, s)
			}

			Expect(status).To(ContainElements(
				And(
					HaveField("Id", "384fc100-740c-43b0-bde2-106c796e2b8b"),
					HaveField("Version", "1"),
					HaveField("Error", api.NoRecordError()),
				),
				And(
					HaveField("Id", "a3fc39bc-5144-4442-8f93-22b1e8a6fb7e"),
					HaveField("Version", "1"),
					HaveField("Error", api.NoRecordError()),
				),
				And(
					HaveField("Id", "99037e22-5c87-483b-97e1-8f54fbd26215"),
					HaveField("Version", "1"),
					HaveField("Error", api.NoRecordError()),
				),
				And(
					HaveField("Id", "e2db93ce-33ce-4e69-abfc-e0d1ccee503d"),
					HaveField("Version", "1"),
					HaveField("Error", api.NoRecordError()),
				),
				And(
					HaveField("Id", "cd40483b-7828-464c-9373-19241585c12d"),
					HaveField("Version", "1"),
					HaveField("Error", api.NoRecordError()),
				),
				And(
					HaveField("Id", "72b373fa-da8d-42c4-8b4a-c938ff6e6932"),
					HaveField("Version", "1"),
					HaveField("Error", api.NoRecordError()),
				),
			))
			Expect(dst.Error()).NotTo(HaveOccurred())
		})
	})

	Context("with performers model", func() {

		var performers *pgxGraph
		BeforeEach(func(ctx context.Context) {
			ctx = log.NewContext(ctx, logger)
			fixture, err := factory.Build(ctx, &graph.Model{
				Name: "performers",
				Table: graph.Table{
					Name:         "syncd.performers",
					KeyField:     "id",
					VersionField: "version",
				},
				Children: []graph.Model{
					{
						Name: "members",
						Table: graph.Table{
							Name:     "syncd.performer_artists",
							KeyField: "performer",
						},
						IsSet: true,
					},
					{
						Name: "performances",
						Table: graph.Table{
							Name:          "syncd.performances",
							KeyField:      "performer",
							SequenceField: "sequence",
						},
						IsSet: true,
					},
				}})
			Expect(fixture, err).ShouldNot(BeNil())
			performers = fixture.(*pgxGraph)
			Expect(performers).ShouldNot(BeNil())
		})

		It("it graphs the performers table", func() {

			By("describing the columns")
			Expect(performers.columns).To(ContainElements(
				Column{
					Name:     "id",
					Position: 1,
					Type:     api.StringData{},
				},
				Column{
					Name:     "name",
					Position: 2,
					Type:     api.StringData{},
				},
				Column{
					Name:     "label",
					Position: 3,
					Type:     api.StringData{},
				},
				Column{
					Name:     "version",
					Position: 4,
					Type:     api.StringData{},
				},
			))

			By("generating model statements")
			Expect(performers.model.readQuery).To(Equal("SELECT name, label, version FROM syncd.performers WHERE id = $1 FOR SHARE"))
			Expect(performers.model.lockQuery).To(Equal("SELECT version FROM syncd.performers WHERE id = $1 FOR UPDATE"))
			Expect(performers.model.updateStatement).To(Equal("UPDATE syncd.performers SET name = $2, label = $3, version = $4 WHERE id = $1"))
			Expect(performers.model.insertStatement).To(Equal("INSERT INTO syncd.performers (id, name, label, version) VALUES ($1, $2, $3, $4)"))
			Expect(performers.model.deleteStatement).To(BeEmpty())
		})

		It("graphs the performer_artists table", func() {

			Expect(performers.children).To(HaveKey("members"))
			members := performers.children["members"]

			By("describing the columns")
			Expect(members.columns).To(ContainElements(
				Column{
					Name:     "performer",
					Position: 1,
					Type:     api.StringData{},
				},
				Column{
					Name:     "artist",
					Position: 2,
					Type:     api.StringData{},
				},
			))

			By("generating a read query")
			Expect(members.readQuery).To(Equal("SELECT artist FROM syncd.performer_artists WHERE performer = $1"))
			Expect(members.lockQuery).To(BeEmpty())
			Expect(members.updateStatement).To(BeEmpty())
			Expect(members.insertStatement).To(Equal("INSERT INTO syncd.performer_artists (performer, artist) VALUES ($1, $2)"))
			Expect(members.deleteStatement).To(Equal("DELETE FROM syncd.performer_artists WHERE performer = $1"))
		})

		It("graphs the performances table", func() {

			Expect(performers.children).To(HaveKey("performances"))
			performances := performers.children["performances"]

			By("generating a sync query")
			Expect(performers.syncQuery).To(Equal(
				"SELECT model.id, model.version FROM syncd.performers AS model LEFT JOIN (SELECT id, version FROM syncd.sync WHERE peer = $1 AND model = 'performers') AS sync ON sync.id = model.id WHERE sync.version IS NULL or sync.version != model.version",
			))

			By("describing the columns")
			Expect(performances.columns).To(ContainElements(
				Column{
					Name:     "performer",
					Position: 1,
					Type:     api.StringData{},
				},
				Column{
					Name:     "sequence",
					Position: 2,
					Type:     api.UintData{},
				},
				Column{
					Name:     "song",
					Position: 3,
					Type:     api.StringData{},
					Nullable: false,
				},
			))

			By("generating a read query")
			Expect(performances.readQuery).To(Equal("SELECT sequence, song FROM syncd.performances WHERE performer = $1"))
			Expect(performances.lockQuery).To(BeEmpty())
			Expect(performances.insertStatement).To(Equal("INSERT INTO syncd.performances (performer, sequence, song) VALUES ($1, $2, $3)"))
			Expect(performances.deleteStatement).To(BeEmpty())
			Expect(performances.updateStatement).To(BeEmpty())
		})

		It("fetches", func(ctx context.Context) {

			ctx = log.NewContext(ctx, logger)

			data := &api.Data{}
			Expect(performers.model.Fetch(ctx, performers.Pool, "16551b42-292f-4316-800c-880c864fcbfd", data, &graph.Filter{
				Key:      "performances.sequence",
				Operator: graph.GreaterThanFilterOperator,
				Value:    1,
			})).To(BeNil())
			Expect(data).ToNot(BeNil())
			Expect(data.Type).To(Equal(api.DataType_RECORD))
			Expect(data.IsList).To(BeFalse())
			Expect(data.Records).To(HaveLen(1))
			Expect(data.Records[0]).To(HaveField("Fields", Not(BeEmpty())))

			theBeatles := data.Records[0].Fields
			Expect(theBeatles).To(HaveKeyWithValue("id", api.StringData{}.From("16551b42-292f-4316-800c-880c864fcbfd")))
			Expect(theBeatles).To(HaveKeyWithValue("label", api.StringData{}.From("VeeJay")))
			Expect(theBeatles).To(HaveKeyWithValue("version", api.StringData{}.From("1")))
			Expect(theBeatles).To(HaveKeyWithValue("members", And(
				HaveField("Type", Equal(api.DataType_STRING)),
				HaveField("IsList", BeTrue()),
				HaveField("Strings", And(
					HaveLen(4),
					ContainElements(
						"8b9835e6-9928-4f6a-afba-ea3e80cf89d0",
						"253cedce-05ed-44bc-a49d-6f570a18b2ef",
						"57ecbe75-ba4c-4094-acca-d066b6c6d058",
						"88a6845b-e286-49a0-aa09-d89309d12d33",
					),
				)),
			)))
			Expect(theBeatles).To(HaveKeyWithValue("performances", And(
				HaveField("Type", Equal(api.DataType_RECORD)),
				HaveField("IsList", BeTrue()),
				HaveField("Records", And(
					HaveLen(1),
					ContainElements(
						And(
							HaveField("Fields", And(
								HaveKeyWithValue("performer", api.StringData{}.From("16551b42-292f-4316-800c-880c864fcbfd")),
								HaveKeyWithValue("sequence", api.UintData{}.From(2)),
								HaveKeyWithValue("song", api.StringData{}.From("3ce42a81-ae99-42b6-b4ed-d0dc1107e651")),
							)),
						)),
				)),
			)))
		})

		Context("performer sync source", func() {

			var src graph.Source
			BeforeEach(func(ctx SpecContext) {
				src = performers.Source(ctx.SpecReport().LeafNodeText)
			})

			It("fetches", func(ctx context.Context) {

				ctx = log.NewContext(ctx, logger)

				By("sending all unsynced data")
				var actual []*api.Data
				data := src.Fetch(ctx)
				for done := false; !done; {
					select {
					case d, ok := <-data:
						if ok {
							actual = append(actual, d)
						} else {
							done = true
						}
					case _ = <-ctx.Done():
						Skip("context canceled")
					}
				}
				Expect(actual).To(ContainElements(
					And(
						Not(BeNil()),
						HaveField("Type", Equal(api.DataType_RECORD)),
						HaveField("IsList", BeFalse()),
						HaveField("Records", And(
							HaveLen(1),
							ContainElement(HaveField("Fields", And(
								HaveKeyWithValue("id", api.StringData{}.From("16551b42-292f-4316-800c-880c864fcbfd")),
								HaveKeyWithValue("label", api.StringData{}.From("VeeJay")),
								HaveKeyWithValue("version", api.StringData{}.From("1")),
								HaveKeyWithValue("members", And(
									HaveField("Type", Equal(api.DataType_STRING)),
									HaveField("IsList", BeTrue()),
									HaveField("Strings", And(
										HaveLen(4),
										ContainElements(
											"8b9835e6-9928-4f6a-afba-ea3e80cf89d0",
											"253cedce-05ed-44bc-a49d-6f570a18b2ef",
											"57ecbe75-ba4c-4094-acca-d066b6c6d058",
											"88a6845b-e286-49a0-aa09-d89309d12d33",
										),
									)))),
								HaveKeyWithValue("performances", And(
									HaveField("Type", Equal(api.DataType_RECORD)),
									HaveField("IsList", BeTrue()),
									HaveField("Records", And(
										HaveLen(2),
										ContainElements(
											And(
												HaveField("Fields", And(
													HaveKeyWithValue("performer", api.StringData{}.From("16551b42-292f-4316-800c-880c864fcbfd")),
													HaveKeyWithValue("sequence", api.UintData{}.From(1)),
													HaveKeyWithValue("song", api.StringData{}.From("ec7f7744-1f8f-4086-bff6-4809ecb948f4")),
												)),
											),
											And(
												HaveField("Fields", And(
													HaveKeyWithValue("performer", api.StringData{}.From("16551b42-292f-4316-800c-880c864fcbfd")),
													HaveKeyWithValue("sequence", api.UintData{}.From(2)),
													HaveKeyWithValue("song", api.StringData{}.From("3ce42a81-ae99-42b6-b4ed-d0dc1107e651")),
												)),
											),
										),
									)),
								)),
							))),
						)),
					)))

				// Update statuses
				By("accepting RecordStatus updates")
				var status []*api.RecordStatus
				for i := range actual {
					Expect(actual[i].Records).To(And(
						HaveLen(1),
						ContainElement(HaveField("Fields",
							HaveKeyWithValue("performances", And(
								HaveField("Type", Equal(api.DataType_RECORD)),
								HaveField("IsList", BeTrue()),
							)),
						)),
					))
					performances := actual[i].Records[0].Fields["performances"].Records
					var sequences map[string]uint64
					if len(performances) > 0 {
						lastPerformance := performances[len(performances)-1].Fields["sequence"].Uints[0]
						sequences = map[string]uint64{
							"performances": lastPerformance,
						}
					}
					status = append(status, &api.RecordStatus{
						Id:        actual[i].Records[0].Fields["id"].Strings[0],
						Version:   actual[i].Records[0].Fields["version"].Strings[0],
						Sequences: sequences,
					})
				}
				Expect(src.SetStatus(ctx, status...)).To(Succeed())

				// Resync
				By("Not resending already-synced data")
				data = src.Fetch(ctx)
				_, ok := <-data
				Expect(ok).To(BeFalse())
			})

			Context("performer sync destination", func() {

				// Create a shared *pgxFactory and reset the database for each test
				var dfactory *pgxFactory
				BeforeEach(func(ctx context.Context) {
					ctx = log.NewContext(ctx, logger)
					fixture, err := New(ctx, ConnectionConfig{
						ConnectionString: os.Getenv("SYNCD_POSTGRESQL_CONN"),
						SyncTable:        "syncd2.sync",
						SequenceTable:    "syncd2.sync_seq",
					})
					if f, ok := fixture.(*pgxFactory); err == nil && ok {
						dfactory = f
					} else {
						Fail("unexpected factory type")
					}
				})

				AfterEach(func(ctx context.Context) {
					if dfactory != nil {
						if e := dfactory.Close(ctx); e != nil {
							logger.Error("error closing factory", zap.Error(e))
							Fail(e.Error())
						}
					}
				})

				var dperformers *pgxGraph
				BeforeEach(func(ctx context.Context) {
					ctx = log.NewContext(ctx, logger)
					fixture, err := dfactory.Build(ctx, &graph.Model{
						Name: "performers",
						Table: graph.Table{
							Name:         "syncd2.performers",
							KeyField:     "id",
							VersionField: "version",
						},
						Children: []graph.Model{
							{
								Name: "members",
								Table: graph.Table{
									Name:     "syncd2.performer_artists",
									KeyField: "performer",
								},
								IsSet: true,
							},
							{
								Name: "performances",
								Table: graph.Table{
									Name:          "syncd2.performances",
									KeyField:      "performer",
									SequenceField: "sequence",
								},
								IsSet: true,
							},
						}})
					Expect(fixture, err).ShouldNot(BeNil())
					dperformers = fixture.(*pgxGraph)
					Expect(dperformers).ShouldNot(BeNil())
				})

				It("writes to data to the destination", func(ctx context.Context) {
					dst := dperformers.Destination()
					for s := range dst.Write(ctx, src.Fetch(ctx)) {
						Expect(s.Error).To(Equal(api.NoRecordError()))
						Expect(src.SetStatus(ctx, s)).To(Succeed())
					}
					Expect(dst.Error()).NotTo(HaveOccurred())
					Expect(src.Error()).NotTo(HaveOccurred())
				})
			})
		})
	})
})
