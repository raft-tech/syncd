package helpers

import (
	"context"
	"os"

	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/graph"
	"github.com/raft-tech/syncd/pkg/graph/postgres"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type PostgresGraphFactory func(ctx context.Context, config postgres.ConnectionConfig) (graph.Factory, error)

var (
	postgresFactory PostgresGraphFactory
)

func init() {
	ResetPostgresGraphFactory()
}

func OverridePostgresGraphFactory(factory PostgresGraphFactory) {
	postgresFactory = factory
}

func ResetPostgresGraphFactory() {
	postgresFactory = postgres.New
}

func Graph(ctx context.Context, cfg *viper.Viper) (syncd map[string]graph.Graph, closer func(context.Context) error, err error) {

	gcfg := GraphConfig{}
	if err = cfg.Unmarshal(&gcfg); err != nil {
		err = WrapError(err, 2)
		return
	} else if err = gcfg.Validate(); err != nil {
		return
	}

	var factory graph.Factory
	err = NewError("graph source not defined", 2)
	switch src := gcfg.Source; true {
	case src.Postgres != nil:
		if factory, err = gcfg.PostgreSQL(ctx); err == nil {
			closer = factory.Close
		} else {
			return
		}
	default:
		return
	}

	// If build fails, make sure to clean up
	ok := false
	defer func() {
		if !ok {
			if e := factory.Close(ctx); e != nil {
				log.FromContext(ctx).Error("error cleaning up factory", zap.Error(e))
			}
		}
	}()

	// Graph each model
	syncd = make(map[string]graph.Graph)
	for k, m := range gcfg.Models {

		var model graph.Model
		if err = m.GenerateModel(k, &model); err != nil {
			return
		}

		var g graph.Graph
		if g, err = factory.Build(ctx, &model); err != nil {
			syncd = nil
			err = WrapError(err, 2)
			return
		}
		syncd[k] = g
	}
	ok = true

	return
}

type GraphConfig struct {
	Source struct {
		Postgres *struct {
			Connection StringValue
			SyncTable  struct {
				Name string
			}
			SequenceTable struct {
				Name string
			}
		}
	}
	Models map[string]GraphModel
}

func (c *GraphConfig) Validate() error {
	// TODO validate GraphConfigs
	return nil
}

type GraphModel struct {
	Table    string
	Key      string
	Version  string
	Sequence string
	ChildKey string
	Priority string
	IsSet    bool
	Children map[string]GraphModel
}

func (m *GraphModel) GenerateModel(name string, model *graph.Model) error {
	model.Name = name
	model.Table = graph.Table{
		Name:          m.Table,
		KeyField:      m.Key,
		SequenceField: m.Sequence,
		PriorityField: m.Priority,
		VersionField:  m.Version,
	}
	model.IsSet = m.IsSet
	model.ChildKey = m.ChildKey
	for k, v := range m.Children {
		cmodel := graph.Model{}
		if err := v.GenerateModel(k, &cmodel); err != nil {
			return err
		}
		model.Children = append(model.Children, cmodel)
	}
	return nil
}

func (c *GraphConfig) PostgreSQL(ctx context.Context) (graph.Factory, error) {
	cfg := postgres.ConnectionConfig{
		ConnectionString: c.Source.Postgres.Connection.GetValue(),
		SyncTable:        c.Source.Postgres.SyncTable.Name,
		SequenceTable:    c.Source.Postgres.SequenceTable.Name,
	}
	if env := c.Source.Postgres.Connection.FromEnv; cfg.ConnectionString == "" && env != "" {
		cfg.ConnectionString = os.Getenv(env)
	}
	return postgresFactory(ctx, cfg)
}
