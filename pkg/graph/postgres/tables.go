package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/raft-tech/syncd/internal/api"
)

const (
	DescribeTableSQL = "SELECT column_name, ordinal_position, column_default, is_nullable, data_type " +
		"FROM information_schema.columns " +
		"WHERE table_schema = $1 AND table_name = $2 " +
		"ORDER BY ordinal_position ASC"
)

type Column struct {
	Name          string
	Position      uint
	Type          api.IDataType
	Default       *string
	Nullable      bool
	MaxCharLength uint
	Precision     uint
}

func (c *Column) ScanRow(rows pgx.Rows) error {
	dtype := ""
	nullable := ""
	args := make([]interface{}, 0, 9)
	for _, d := range rows.FieldDescriptions() {
		switch d.Name {
		case "column_name":
			args = append(args, &c.Name)
		case "ordinal_position":
			args = append(args, &c.Position)
		case "column_default":
			c.Default = nil
			args = append(args, &c.Default)
		case "is_nullable":
			args = append(args, &nullable)
		case "data_type":
			args = append(args, &dtype)
		case "character_maximum_length":
			args = append(args, &c.MaxCharLength)
		case "numeric_precision":
			args = append(args, &c.Precision)
		default:
			return errors.New("unrecognized table description: " + d.Name)
		}
	}
	if e := rows.Scan(args...); e == nil {
		c.Nullable = nullable == "YES"
		if v, ok := lookupValue(dtype); ok {
			c.Type = v
		} else {
			return errors.New("unrecognized data_type: " + dtype)
		}
	} else {
		return e
	}
	return nil
}

func lookupValue(dtype string) (v api.IDataType, ok bool) {
	switch dtype {

	// Character Types
	case "character varying":
		fallthrough
	case "text":
		v = api.StringData{}

	// Integer Types
	case "smallint":
		fallthrough
	case "bigint":
		fallthrough
	case "integer":
		fallthrough
	case "int":
		v = api.IntData{}

	// UUID
	case "uuid":
		v = api.StringData{}
	}
	ok = v != nil
	return
}
