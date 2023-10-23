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

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raft-tech/syncd/internal/api"
	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/graph"
	"go.uber.org/zap"
)

type model struct {
	graph.Model

	parent  *model
	columns []Column

	childKeyIdx int
	keyIdx      int
	sequenceIdx int
	versionIdx  int

	readQuery       string
	lockQuery       string
	updateStatement string
	insertStatement string
	deleteStatement string

	children map[string]*model
}

func (m *model) init(ctx context.Context, conn *pgx.Conn) error {

	// Validate the model
	if m.IsSet && m.parent == nil {
		return graph.NewModelError(errors.New("top-level models cannot be sets"))
	}

	// Reset index fields
	m.childKeyIdx = -1
	m.keyIdx = -1
	m.sequenceIdx = -1
	m.versionIdx = -1

	// Query information schema for columns
	table := m.Table.Name
	var schema = "public"
	if s := strings.SplitN(table, ".", 2); len(s) == 2 {
		schema = s[0]
		table = s[1]
	}
	if r, err := conn.Query(ctx, DescribeTableSQL, schema, table); err == nil {
		for i := 0; r.Next(); i++ {
			c := Column{}
			if e := r.Scan(&c); e == nil {
				if c.Name == m.Table.KeyField {
					m.keyIdx = i
				} else if c.Name == m.Table.VersionField {
					m.versionIdx = i
				} else if c.Name == m.ChildKey {
					m.childKeyIdx = i
				} else if c.Name == m.Table.SequenceField {
					m.sequenceIdx = i
				}
				m.columns = append(m.columns, c)
			} else {
				r.Close()
				return e
			}
		}
	} else {
		return err
	}

	// Validate required fields are present and valid
	if m.keyIdx == -1 {
		return graph.NewModelError(errors.New("specified key field not found in table columns: " + m.Table.KeyField))
	} else if _, ok := m.columns[m.keyIdx].Type.NewValue().(*string); !ok {
		return graph.NewModelError(errors.New("id field must be a string-compatible data type"))
	}
	if m.parent == nil {
		if len(m.columns) < 3 {
			return graph.NewModelError(errors.New("top-level models require a minimum of three fields"))
		} else if m.Table.VersionField == "" {
			return graph.NewModelError(errors.New("version fields are required on non-child models"))
		} else if m.versionIdx == -1 {
			return graph.NewModelError(errors.New("specified version field not found in table columns: " + m.Table.VersionField))
		} else if _, ok := m.columns[m.versionIdx].Type.(api.StringData); !ok {
			return graph.NewModelError(errors.New("version field must be a string-compatible data type"))
		}
	} else if len(m.columns) < 2 {
		return graph.NewModelError(errors.New("child models require a minimum of two fields"))
	}

	if m.Table.SequenceField != "" {
		if m.sequenceIdx == -1 {
			return graph.NewModelError(errors.New("specified sequence field not found in table columns: " + m.ChildKey))
		} else if _, ok := m.columns[m.sequenceIdx].Type.(api.IntData); ok {
			m.columns[m.sequenceIdx].Type = api.UintData{}
		} else {
			return graph.NewModelError(errors.New("sequence field must be an integer-compatible data type"))
		}
	}
	if m.ChildKey != "" {
		if m.childKeyIdx == -1 {
			return graph.NewModelError(errors.New("specified child-key field not found in table columns: " + m.ChildKey))
		} else if _, ok := m.columns[m.childKeyIdx].Type.(api.StringData); !ok {
			return graph.NewModelError(errors.New("child-key field must be a string-compatible data type"))
		}
	}

	// Generate the read query and validate column types
	{
		insertArgs := make([]string, 0, len(m.columns))
		queryFields := make([]string, 0, len(m.columns)-1) // all fields except the id field
		updates := make([]string, 0, len(m.columns)-1)     // all fields except the id field
		for i := range m.columns {
			insertArgs = append(insertArgs, "$"+strconv.Itoa(i+1))
			if i != m.keyIdx {
				queryFields = append(queryFields, m.columns[i].Name)
				updates = append(updates, m.columns[i].Name+" = $"+strconv.Itoa(i+1))
			}
		}
		m.readQuery = fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1", strings.Join(queryFields, ", "), m.Table.Name, m.Table.KeyField)
		m.insertStatement = fmt.Sprintf("INSERT INTO %s (%s, %s) VALUES (%s)",
			m.Table.Name,
			m.Table.KeyField,
			strings.Join(queryFields, ", "),
			strings.Join(insertArgs, ", "))
		if m.Table.SequenceField == "" {
			if m.IsSet {
				m.deleteStatement = fmt.Sprintf("DELETE FROM %s WHERE %s = $1", m.Table.Name, m.Table.KeyField)
			} else {
				m.updateStatement = fmt.Sprintf("UPDATE %s SET %s WHERE %s = $1",
					m.Table.Name,
					strings.Join(updates, ", "),
					m.Table.KeyField)
			}
		}

		// Top-level models require locking
		if m.parent == nil {
			m.readQuery += " FOR SHARE"
			m.lockQuery = fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1 FOR UPDATE", m.Table.VersionField, m.Table.Name, m.Table.KeyField)
		}
	}

	// Initialize child models
	if len(m.Model.Children) > 0 {
		m.children = make(map[string]*model)
		for i := range m.Model.Children {
			m.children[m.Model.Children[i].Name] = &model{
				Model:  m.Model.Children[i],
				parent: m,
			}
			if err := m.children[m.Model.Children[i].Name].init(ctx, conn); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *model) Fetch(ctx context.Context, pool *pgxpool.Pool, key string, data *api.Data, filters ...*graph.Filter) error {

	logger := log.FromContext(ctx).With(zap.String("model", m.Name), zap.String("key", key))

	if data != nil {
		data.Reset()
		data.Type = api.DataType_RECORD
	} else {
		panic("data must not be nil")
	}

	// Add filters to query; map child filters
	childFilters := make(map[string][]*graph.Filter)
	query := m.readQuery
	if len(filters) > 0 {
		qb := strings.Builder{}
		qb.WriteString(m.readQuery)
		for i := range filters {
			if p := strings.IndexRune(filters[i].Key, '.'); p >= 0 {
				if p == 0 {
					return errors.New("invalid filter key: keys must not start with '.'")
				} else if len(filters[i].Key)-p == 1 {
					return errors.New("invalid filter key: keys must not end with '.'")
				} else {
					k := filters[i].Key[:p]
					filters[i].Key = filters[i].Key[p+1:]
					if cf, ok := childFilters[k]; ok {
						childFilters[k] = append(cf, filters[i])
					} else {
						childFilters[k] = []*graph.Filter{filters[i]}
					}
				}

			} else if stmt, ok := filterStatement(filters[i]); ok {
				qb.WriteString(" " + stmt)
			} else {
				return errors.New(fmt.Sprintf("unknown operator '%s' on key '%s'", filters[i].Operator, filters[i].Key))
			}
		}
		query = qb.String()
	}

	// Connect to the DB
	var conn *pgxpool.Conn
	logger.Debug("acquiring database connection")
	if c, e := pool.Acquire(ctx); e == nil {
		conn = c
		logger.Debug("acquired database connection")
		defer func() {
			logger.Debug("release database connection")
			c.Release()
			logger.Debug("released database connection")
		}()
	} else {
		return graph.NewDatabaseError(e)
	}

	// Determine what to sync
	logger.Debug("querying for record(s)")
	var rows pgx.Rows
	if r, err := conn.Query(ctx, query, key); err == nil {
		defer r.Close()
		rows = r
	} else {
		logger.Error("error querying for model", zap.String("query", m.readQuery), zap.String("key", key), zap.Error(err))
		return graph.NewModelError(err)
	}

	count := len(m.columns) - 1
	cols := make([]*Column, 0, count)
	values := make([]interface{}, 0, count)
	for i := range m.columns {
		if i != m.keyIdx {
			cols = append(cols, &m.columns[i])
			values = append(values, m.columns[i].Type.NewValue())
		}
	}

	if !m.IsSet {

		// Fetching a single record and its children
		data.Type = api.DataType_RECORD
		data.Records = []*api.Record{
			{Fields: make(map[string]*api.Data)},
		}
		fields := data.Records[0].Fields

		if !rows.Next() {
			if e := rows.Err(); e == nil {
				logger.Error("no matching record found")
				return graph.NoDataError()
			} else {
				logger.Error("database error", zap.Error(e))
				return graph.NewModelError(e)
			}
		} else if err := rows.Scan(values...); err == nil {

			// Complex models with 1:N:N relationships may have a child key
			ckey := key

			// Get scalar values
			fields[m.Table.KeyField] = api.StringData{}.From(key)
			for i := range cols {
				fields[cols[i].Name] = &api.Data{}
				if e := cols[i].Type.Encode(values[i], fields[cols[i].Name]); e != nil {
					logger.Error("unable to encode value", zap.Error(e))
					return graph.NewModelError(e)
				}
				if m.ChildKey != "" && i == m.childKeyIdx {
					panic("not implemented")
				}
				if cols[i].Name == m.ChildKey {
					if k, ok := values[i].(string); ok {
						ckey = k
					} else if k, ok := values[i].(*string); ok {
						if k != nil {
							ckey = *k
						} else {
							ckey = ""
						}
					} else {
						panic("invalid data type for child key")
					}
				}
			}
			if rows.Next() {
				rows.Close()
				logger.Error("expected one row; query match multiple rows")
				return graph.NewModelError(errors.New("unexpected row count: more than one row"))
			}

			// Get children
			cctx := log.NewContext(ctx, logger.With(zap.String("parent", m.Name+"/"+key)))
			for i := range m.children {
				child := &api.Data{}
				data.Records[0].Fields[m.children[i].Name] = child
				if err := m.children[i].Fetch(cctx, pool, ckey, child, childFilters[m.children[i].Name]...); err != nil {
					return graph.NewModelError(err)
				}
			}
		} else {
			return graph.NewModelError(err)
		}

	} else if count == 1 && len(m.children) == 0 {

		// Fetching a set of scalar values
		dtype := cols[0].Type
		data.Type = dtype.Type()
		data.IsList = true

		for rows.Next() {
			if err := rows.Scan(values...); err == nil {
				if err = dtype.Append(values[0], data); err != nil {
					logger.Error("error encoding value")
					return graph.NewModelError(err)
				}
			}
		}

	} else {

		// Fetching a set of records
		data.Type = api.DataType_RECORD
		data.IsList = true
		data.Records = make([]*api.Record, 0)

		// Get the rows!
		for rows.Next() {
			if err := rows.Scan(values...); err == nil {

				fields := make(map[string]*api.Data)
				data.Records = append(data.Records, &api.Record{Fields: fields})
				ckey := key

				// Get scalar values & determine the key to use when querying children
				fields[m.Table.KeyField] = api.StringData{}.From(key)
				for i := range cols {
					fields[cols[i].Name] = &api.Data{}
					if e := cols[i].Type.Encode(values[i], fields[cols[i].Name]); e != nil {
						logger.Error("unable to encode value", zap.Error(e))
						return graph.NewModelError(e)
					}
					if cols[i].Name == m.ChildKey {
						if k, ok := values[i].(string); ok {
							ckey = k
						} else if k, ok := values[i].(*string); ok {
							if k != nil {
								ckey = *k
							} else {
								ckey = ""
							}
						} else {
							panic("invalid data type for child key")
						}
					}
				}

				// Query children
				if len(m.children) > 0 {
					cctx := log.NewContext(ctx, logger.With(zap.String("parent", m.Name+"/"+key)))
					for i := range m.children {
						child := &api.Data{}
						fields[m.children[i].Name] = child
						if err := m.children[i].Fetch(cctx, pool, ckey, child, childFilters[m.children[i].Name]...); err != nil {
							return graph.NewModelError(err)
						}
					}
				}

			} else {
				return graph.NewModelError(err)
			}
		}
	}

	return nil
}

type sequenceState struct {
	model string
	seq   uint64
}

func (m *model) Insert(ctx context.Context, tx pgx.Tx, data *api.Data) ([]sequenceState, error) {

	if tx == nil {
		panic("tx must not be nil")
	} else if data == nil {
		panic("data must not be nil")
	}

	var seq []sequenceState
	if data.Type == api.DataType_RECORD {

		if data.IsList {
			for i := range data.Records {
				if s, err := m.Insert(ctx, tx, &api.Data{
					Type:    api.DataType_RECORD,
					Records: []*api.Record{data.Records[i]},
				}); err == nil {
					seq = append(seq, s...)
				} else {
					return nil, err
				}
			}
			return seq, nil
		}

		logger := log.FromContext(ctx).With(zap.String("model", m.Name))

		// Prepare args and determine child key for future use
		var childKey *api.Data
		fields := data.Records[0].Fields
		args := make([]interface{}, 1, len(m.columns))
		for i := range m.columns {
			dtype := m.columns[i].Type
			value := dtype.NewValue()
			if val, ok := fields[m.columns[i].Name]; ok {
				if err := m.columns[i].Type.Decode(val, value); err == nil {
					if i == m.keyIdx {
						args[0] = value
						key := *(value.(*string))
						logger = logger.With(zap.String("key", key))
						if m.childKeyIdx == -1 {
							childKey = api.StringData{}.From(key)
						}
					} else {
						args = append(args, value)
						if i == m.sequenceIdx {
							seq = append(seq, sequenceState{model: m.Name, seq: *(value.(*uint64))})
						} else if i == m.childKeyIdx {
							ck := *(value.(*string))
							childKey = api.StringData{}.From(ck)
							logger = logger.With(zap.String("childKey", ck))
						}
					}
				} else {
					return nil, graph.NewDataError(fmt.Sprintf("invalid value for %s", m.columns[i].Name), err)
				}
			} else if m.columns[i].Name == m.Table.KeyField {
				return nil, graph.NewDataError("key field is required")
			} else if m.parent == nil && m.columns[i].Name == m.Table.VersionField {
				return nil, graph.NewDataError("version field is required on top-level models")
			}
		}

		logger.Debug("inserting record")
		if _, err := tx.Exec(ctx, m.insertStatement, args...); err != nil {
			logger.Error("error inserting record", zap.Error(err))
			return nil, graph.NewDatabaseError(err)
		}

		// Add children
		for k, child := range m.children {
			if d, ok := fields[k]; ok {

				// Scalar types need to be converted to records with IDs added
				var cdata *api.Data
				if d.Type == api.DataType_RECORD {
					cdata = d
				} else if len(child.columns) == 2 {

					// The value field is probably the second column
					vfield := child.columns[1].Name
					if child.keyIdx == 1 {
						vfield = child.columns[0].Name
					}

					// Type-specific record
					cdata = &api.Data{
						Type:   api.DataType_RECORD,
						IsList: d.IsList,
					}
					switch d.Type {

					case api.DataType_UINT:
						cdata.Records = make([]*api.Record, 0, len(d.Uints))
						for i := range d.Uints {
							cdata.Records = append(cdata.Records, &api.Record{Fields: map[string]*api.Data{
								child.Table.KeyField: childKey,
								vfield: &api.Data{
									Type:  api.DataType_UINT,
									Uints: []uint64{d.Uints[i]},
								},
							}})
						}

					case api.DataType_INT:
						cdata.Records = make([]*api.Record, 0, len(d.Uints))
						for i := range d.Uints {
							cdata.Records = append(cdata.Records, &api.Record{Fields: map[string]*api.Data{
								child.Table.KeyField: childKey,
								vfield: &api.Data{
									Type:  api.DataType_INT,
									Uints: []uint64{d.Uints[i]},
								},
							}})
						}

					case api.DataType_FLOAT:
						cdata.Records = make([]*api.Record, 0, len(d.Floats))
						for i := range d.Floats {
							cdata.Records = append(cdata.Records, &api.Record{Fields: map[string]*api.Data{
								child.Table.KeyField: childKey,
								vfield: &api.Data{
									Type:   api.DataType_FLOAT,
									Floats: []float64{d.Floats[i]},
								},
							}})
						}

					case api.DataType_BOOL:
						cdata.Records = make([]*api.Record, 0, len(d.Bools))
						for i := range d.Bools {
							cdata.Records = append(cdata.Records, &api.Record{Fields: map[string]*api.Data{
								child.Table.KeyField: childKey,
								vfield: &api.Data{
									Type:  api.DataType_BOOL,
									Bools: []bool{d.Bools[i]},
								},
							}})
						}

					case api.DataType_STRING:
						cdata.Records = make([]*api.Record, 0, len(d.Strings))
						for i := range d.Strings {
							cdata.Records = append(cdata.Records, &api.Record{Fields: map[string]*api.Data{
								child.Table.KeyField: childKey,
								vfield: &api.Data{
									Type:    api.DataType_STRING,
									Strings: []string{d.Strings[i]},
								},
							}})
						}

					case api.DataType_BYTES:
						cdata.Records = make([]*api.Record, 0, len(d.Bytes))
						for i := range d.Bytes {
							cdata.Records = append(cdata.Records, &api.Record{Fields: map[string]*api.Data{
								child.Table.KeyField: childKey,
								vfield: &api.Data{
									Type:  api.DataType_BYTES,
									Bytes: [][]byte{d.Bytes[i]},
								},
							}})
						}

					default:
						logger.Error("unrecognized data type", zap.String("type", d.Type.String()))
						return nil, graph.NewDataError("unrecognized data type")
					}
				} else {
					logger.Error("data type mismatch", zap.String("description", "scalar values require a 2-column table"))
					return nil, graph.MalformedDataError()
				}

				if s, err := m.children[k].Insert(ctx, tx, cdata); err == nil {
					seq = append(seq, s...)
				} else {
					return nil, err
				}
			}
		}
		logger.Info("inserted record")

	} else {
		return nil, graph.MalformedDataError()
	}

	return seq, nil
}

func (m *model) Delete(ctx context.Context, tx pgx.Tx, id string) error {

	if tx == nil {
		panic("tx must not be nil")
	}

	logger := log.FromContext(ctx).With(zap.String("model", m.Name)).With(zap.String("id", id))

	ctx = log.NewContext(ctx, logger)
	for i := range m.children {
		if err := m.children[i].Delete(ctx, tx, id); err != nil {
			return nil
		}
	}

	logger.Debug("deleting record(s)")
	if t, err := tx.Exec(ctx, m.deleteStatement, id); err == nil {
		logger.Info("deleted record(s)", zap.Int64("count", t.RowsAffected()))
		return nil
	} else {
		logger.Error("error deleting record", zap.Error(err))
		return graph.NewDatabaseError(err)
	}
}
