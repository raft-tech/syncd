package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/api"
	"github.com/raft-tech/syncd/pkg/graph"
	"go.uber.org/zap"
	"strings"
)

type ConnectionConfig struct {
	ConnectionString string
	SyncTable        string
	SequenceTable    string
}

var syncTableVersion = "v1.0.0"

var syncTableSQL = `create table %%TABLE_NAME%% (
    peer varchar not null,
    model varchar not null,
    id varchar not null,
    version varchar,
    status int,
    status_message varchar,
    primary key (peer, model, id)
) partition by list (model)
`

var syncColumns = []string{"peer", "model", "id", "version", "status", "status_message"}

var seqTableSQL = `create table %%TABLE_NAME%% (
	peer varchar not null,
	model varchar not null,
	id varchar not null,
	child varchar not null,
	sequence int,
	primary key (peer, model, id, child)
) partition by list (model)
`

func New(ctx context.Context, cfg ConnectionConfig) (graph.Factory, error) {

	logger := log.FromContext(ctx)

	var pool *pgxpool.Pool
	if c, err := pgxpool.ParseConfig(cfg.ConnectionString); err == nil {
		if pool, err = pgxpool.NewWithConfig(ctx, c); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}

	table := "public.syncd_sync"
	if t := cfg.SyncTable; t != "" {
		table = t
	}

	seqTable := "public.syncd_seq"
	if t := cfg.SequenceTable; t != "" {
		seqTable = t
	}

	logger.Debug("determining current syncd table version")
	if conn, err := pool.Acquire(ctx); err == nil {
		defer conn.Release()

		var rows pgx.Rows
		if rows, err = conn.Query(ctx, fmt.Sprintf("SELECT obj_description('%s'::regclass)", table)); err != nil {
			perr := &pgconn.PgError{}
			if !errors.As(err, &perr) || perr.Code != "42P01" {
				logger.Error("error reading sync table comment", zap.Error(err))
				return nil, errors.New("error generating factory")
			}
		}

		var comment string
		if rows.Next() {
			if err = rows.Scan(&comment); err != nil {
				logger.Error("error reading sync table comment", zap.Error(err))
				return nil, errors.New("error generating factory")
			}
			rows.Close()
		}

		if comment == syncTableVersion {
			logger.Info("sync tables already exists")
		} else if comment == "" {

			logger.Debug("creating sync tables")

			if _, err = conn.Exec(ctx, strings.Replace(syncTableSQL, "%%TABLE_NAME%%", table, -1)); err == nil {
				logger.Debug("created sync table", zap.String("name", table))
			} else {
				logger.Error("error creating sync table", zap.Error(err))
				return nil, errors.New("error generating factory")
			}

			if _, err = conn.Exec(ctx, strings.Replace(seqTableSQL, "%%TABLE_NAME%%", seqTable, -1)); err == nil {
				logger.Debug("created sequence table", zap.String("name", table))
			} else {
				logger.Error("error creating sequence table", zap.Error(err))
				return nil, errors.New("error generating factory")
			}

			if _, err = conn.Exec(ctx, fmt.Sprintf("comment on table %s is '%s'", table, syncTableVersion)); err != nil {
				logger.Error("error creating sync table", zap.Error(err))
				return nil, errors.New("error generating factory")
			}
			logger.Info("successfully created sync tables", zap.String("table", table), zap.String("version", syncTableVersion))
		} else {
			logger.Error("unrecognized sync table comment", zap.String("comment", comment))
			return nil, errors.New("unrecognized sync table comment")
		}

	} else {
		return nil, err
	}

	return &pgxFactory{
		Pool:          pool,
		SyncTable:     table,
		SequenceTable: seqTable,
	}, nil
}

type pgxFactory struct {
	*pgxpool.Pool
	SyncTable     string
	SequenceTable string
}

// Build generates a graph based on the provided model and the connected
// database. If a database connection error occurs while
func (p *pgxFactory) Build(ctx context.Context, m *graph.Model) (graph.Graph, error) {
	pgraph := &pgxGraph{
		Pool:  p.Pool,
		model: model{Model: *m},
	}
	return pgraph, pgraph.init(ctx, p.SyncTable, p.SequenceTable)
}

// Close closes all connections managed by Factory. The function blocks until
// either the underlying connections have all been closed or the provided
// Context is canceled or its deadline exceeded. If Close returns due to the
// provided context, the contexts error is return. Otherwise, nil is returned.
func (p *pgxFactory) Close(ctx context.Context) error {
	done := make(chan interface{})
	go func() {
		p.Pool.Close()
		close(done)
	}()
	select {
	case _ = <-done:
		return nil
	case _ = <-ctx.Done():
		return ctx.Err()
	}
}

type pgxGraph struct {
	*pgxpool.Pool
	model
	statusStatement   string
	sequenceStatement string
	syncQuery         string
	syncSequenceQuery string
}

func (p *pgxGraph) init(ctx context.Context, syncTable string, sequenceTable string) error {

	var conn *pgxpool.Conn
	if c, e := p.Acquire(ctx); e == nil {
		conn = c
		defer c.Release()
	} else {
		return graph.NewDatabaseError(e)
	}

	p.syncQuery = fmt.Sprintf("SELECT model.id, model.version FROM %s AS model LEFT JOIN (SELECT id, version FROM %s WHERE peer = $1 AND model = '%s') AS sync ON sync.id = model.%s WHERE sync.version IS NULL or sync.version != model.%s",
		p.Table.Name,
		syncTable,
		p.Name,
		p.Table.KeyField,
		p.Table.VersionField)
	p.syncSequenceQuery = fmt.Sprintf("SELECT child, sequence FROM %s WHERE model = '%s' AND peer = $1 AND id = $2", sequenceTable, p.Name)
	p.statusStatement = fmt.Sprintf("INSERT INTO %s (peer, model, id, version, status, status_message) VALUES ($1, '%s', $2, $3, $4, $5) ON CONFLICT (peer, model, id) DO UPDATE SET version = EXCLUDED.version, status = EXCLUDED.status, status_message = EXCLUDED.status_message",
		syncTable,
		p.Model.Name)
	p.sequenceStatement = fmt.Sprintf("INSERT INTO %s (peer, model, id, child, sequence) VALUES ($1, '%s', $2, $3, $4) ON CONFLICT (peer, model, id, child) DO UPDATE SET sequence = EXCLUDED.sequence",
		sequenceTable,
		p.model.Name)

	if _, err := conn.Exec(ctx, fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s_%s PARTITION OF %s FOR VALUES IN ('%s')",
		syncTable,
		p.Name,
		syncTable,
		p.Name)); err != nil {
		return err
	}

	// Recursively check all children for a sequenced record, create _seq partition if necessary
	var hasSeq func(m *graph.Model) bool
	hasSeq = func(m *graph.Model) bool {
		if m.Table.SequenceField != "" {
			return true
		}
		for i := range m.Children {
			if hasSeq(&m.Children[i]) {
				return true
			}
		}
		return false
	}
	if hasSeq(&p.Model) {
		if _, err := conn.Exec(ctx, fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s_%s PARTITION OF %s FOR VALUES IN ('%s')",
			sequenceTable,
			p.Name,
			sequenceTable,
			p.Name)); err != nil {
			return err
		}
	}

	return p.model.init(ctx, conn.Conn())
}

func (p *pgxGraph) Source(peer string, filters ...graph.Filter) graph.Source {
	return &source{
		pgxGraph: p,
		peer:     peer,
		filters:  filters,
	}
}

func (p *pgxGraph) Destination() graph.Destination {
	return &destination{
		pgxGraph: p,
	}
}

type source struct {
	*pgxGraph
	peer    string
	filters []graph.Filter
	error   error
}

func (f *source) Fetch(ctx context.Context) <-chan *api.Record {

	f.error = nil

	out := make(chan *api.Record)
	go func(ctx context.Context, out chan<- *api.Record) {

		defer close(out)
		logger := log.FromContext(ctx).With(zap.String("peer", f.peer))

		logger.Debug("acquiring database connection")
		var conn *pgxpool.Conn
		if c, e := f.Pool.Acquire(ctx); e == nil {
			defer c.Release()
			conn = c
			logger.Debug("acquired database connection")
		} else {
			if errors.Is(e, context.Canceled) {
				logger.Info("context canceled")
			} else {
				logger.Error("error acquiring database connection", zap.Error(e))
			}
			f.error = e
			return
		}

		logger.Debug("querying for data to sync")
		var rows pgx.Rows
		if r, err := conn.Query(ctx, f.syncQuery, f.peer); err == nil {
			rows = r
		} else {
			if errors.Is(err, context.Canceled) {
				logger.Info("context canceled")
			} else {
				logger.Error("error querying for data to sync", zap.Error(err))
			}
			f.error = err
			return
		}

		// Retrieve available records
		count := 0
		for id, version := "", ""; rows.Next(); {

			if err := rows.Scan(&id, &version); err != nil {
				logger.Error("error reading sync row", zap.Error(err))
				f.error = err
				return
			}
			logger := logger.With(zap.String("recordId", id))

			// Get child sequences
			logger.Debug("acquiring database connection")
			var sconn *pgxpool.Conn
			if c, e := f.Pool.Acquire(ctx); e == nil {
				defer c.Release()
				sconn = c
				logger.Debug("acquired database connection")
			} else {
				if errors.Is(e, context.Canceled) {
					logger.Info("context canceled")
				} else {
					logger.Error("error acquiring database connection", zap.Error(e))
				}
				f.error = e
				return
			}
			var filters []graph.Filter
			if srows, err := sconn.Query(ctx, f.syncSequenceQuery, f.peer, id); err == nil {
				state := sequenceState{}
				defer srows.Close()
				for srows.Next() {
					if e := srows.Scan(&state.model, &state.seq); e != nil {
						logger.Error("error parsing sequence row", zap.Error(err))
						f.error = err
						return
					}
					filters = append(filters, graph.Filter{
						Key:      state.model,
						Operator: graph.GreaterThanFilterOperator,
						Value:    state.seq,
					})
				}
				sconn.Release()
			} else {
				logger.Error("error retrieving sync sequences", zap.Error(err))
				f.error = err
				return
			}

			ctx := log.NewContext(ctx, logger)
			data := &api.Data{}
			if err := f.model.Fetch(ctx, f.Pool, id, data); err == nil {
				logger.Debug("syncing record")
				select {
				case out <- data.Records[0]: // FIXME ensure this always exists
					logger.Debug("record synced")
					count++
				case _ = <-ctx.Done():
					logger.Info("context canceled while waiting to syncd record")
					f.error = context.Canceled
					return
				}
			} else {
				logger.Error("error fetching record", zap.Error(err))
			}
		}

		logger.Info("fetch completed", zap.Int("count", count))
	}(ctx, out)

	return out
}

func (f *source) Error() error {
	return f.error
}

func (f *source) SetStatus(ctx context.Context, status ...*api.RecordStatus) error {

	if status == nil {
		return nil
	}

	logger := log.FromContext(ctx).With(zap.String("peer", f.peer), zap.String("model", f.model.Name))

	logger.Debug("acquiring database connection")
	var conn *pgxpool.Conn
	if c, err := f.Pool.Acquire(ctx); err == nil {
		defer c.Release()
		conn = c
		logger.Debug("acquired database connection")
	} else {
		logger.Error("error acquiring database connection", zap.Error(err))
		return err
	}

	logger.Debug("updating statuses")
	batch := &pgx.Batch{}
	var bmap [][2]string
	for _, s := range status {
		if s != nil {
			batch.Queue(f.statusStatement, f.peer, s.Id, s.Version, s.Error, s.ErrorField)
			bmap = append(bmap, [2]string{s.Id})
			for k, v := range s.Sequences {
				batch.Queue(f.sequenceStatement, f.peer, s.Id, k, v)
				bmap = append(bmap, [2]string{s.Id, k})
			}
		}
	}
	result := conn.SendBatch(ctx, batch)
	defer func() {
		if err := result.Close(); err != nil {
			logger.Error("error closing status batch", zap.Error(err))
		}
	}()

	var err error
	for i, j := 0, batch.Len(); i < j; i++ {
		if _, e := result.Exec(); e != nil {
			id, child := bmap[i][0], bmap[i][1]
			var fields []zap.Field
			fields = append(fields, zap.String("id", id))
			if child != "" {
				fields = append(fields, zap.String("child", child))
			}
			fields = append(fields, zap.Error(e))
			logger.Error("error updating status", fields...)
			if err == nil {
				err = e
			}
		}
	}
	logger.Info("status updated")

	return err
}

type destination struct {
	*pgxGraph
	error error
}

func (d *destination) Write(ctx context.Context, data <-chan *api.Record) <-chan *api.RecordStatus {
	d.error = nil
	status := make(chan *api.RecordStatus)
	go func(ctx context.Context, in <-chan *api.Record, out chan<- *api.RecordStatus) {
		defer close(out)
		logger := log.FromContext(ctx).With(zap.String("model", d.model.Name))
		logger.Info("ready to receive data")
		count := 0
		for done := false; !done; {
			select {
			case val, ok := <-in:
				if ok {
					if s, ok := d.write(ctx, &api.Data{
						Type:    api.DataType_RECORD,
						Records: []*api.Record{{Fields: val.Fields}},
					}); ok {
						count++
						out <- s
					} else {
						// Stop on DB errors
						done = true
					}
				} else {
					done = true
				}
			case _ = <-ctx.Done():
				logger.Error("context canceled")
				d.error = context.Canceled
			}
		}
		logger.Info("receive channel closed", zap.Int("count", count))
	}(ctx, data, status)
	return status
}

func (d *destination) write(ctx context.Context, data *api.Data) (*api.RecordStatus, bool) {

	logger := log.FromContext(ctx).With(zap.String("model", d.model.Name))

	// Validate the data
	var id, version string
	if data.Type == api.DataType_RECORD && !data.IsList {

		if len(data.Records) == 0 {
			logger.Error("missing record")
			return nil, false
		}

		// Get the ID
		if rid, ok := data.Records[0].Fields[d.model.Table.KeyField]; ok {
			if rid.Type == api.DataType_STRING && !rid.IsList {
				if len(rid.Strings) == 0 {
					logger.Error("missing ID string")
					return nil, false
				}
				id = rid.Strings[0]
			} else {
				logger.Error("invalid ID type; expected string", zap.String("type", rid.Type.String()), zap.Bool("isList", rid.IsList))
				return nil, false
			}
		}

		// Get the Version
		if rver, ok := data.Records[0].Fields[d.model.Table.VersionField]; ok {
			if rver.Type == api.DataType_STRING && !rver.IsList {
				if len(rver.Strings) == 0 {
					logger.Error("missing Version string")
					return nil, false
				}
				version = rver.Strings[0]
			} else {
				logger.Error("invalid Version type; expected string", zap.String("type", rver.Type.String()), zap.Bool("isList", rver.IsList))
				return nil, false
			}
		}

	} else {
		logger.Error("invalid data type; expected record", zap.String("type", data.Type.String()), zap.Bool("isList", data.IsList))
		return nil, false
	}

	logger = logger.With(zap.String("id", id), zap.String("version", version))

	// Establish a DB connection
	var conn *pgx.Conn
	if c, err := d.Pool.Acquire(ctx); err == nil {
		defer c.Release()
		conn = c.Conn()
	} else {
		logger.Error("error acquiring connection", zap.Error(err))
		d.error = graph.NewDatabaseError(err)
		return nil, false
	}

	// Start a transaction
	var tx pgx.Tx
	if t, err := conn.BeginTx(ctx, pgx.TxOptions{}); err == nil {
		defer func() {
			if e := t.Rollback(ctx); e != nil && !errors.Is(e, pgx.ErrTxClosed) {
				logger.Error("error rolling backing transaction", zap.Error(e))
			}
		}()
		tx = t
	} else {
		logger.Error("error beginning transaction", zap.Error(err))
		d.error = graph.NewDatabaseError(err)
		return nil, false
	}

	// Add the record
	var seq []sequenceState
	if s, err := d.model.Insert(ctx, tx, data); err == nil {
		seq = s
	} else {
		logger.Error("error writing data", zap.Error(err))
		return &api.RecordStatus{
			Id:      id,
			Version: version,
			Error:   api.LocalRecordError(),
		}, true
	}

	// Commit
	if err := tx.Commit(ctx); err != nil {
		logger.Error("error committing insert transaction", zap.Error(err))
		return &api.RecordStatus{
			Id:      id,
			Version: version,
			Error:   api.LocalRecordError(),
		}, true
	}

	logger.Debug("record successfully inserted")
	res := api.RecordStatus{
		Id:      id,
		Version: version,
		Error:   api.NoRecordError(),
	}
	if len(seq) > 0 {
		res.Sequences = make(map[string]uint64)
	}
	for i := range seq {
		res.Sequences[seq[i].model] = seq[i].seq
	}
	return &res, true
}

func (d *destination) Error() error {
	return d.error
}
