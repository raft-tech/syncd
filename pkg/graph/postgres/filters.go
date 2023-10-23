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
	"fmt"
	"strings"

	"github.com/raft-tech/syncd/pkg/graph"
)

func filterStatement(filter *graph.Filter) (stmt string, ok bool) {
	switch filter.Operator {
	case graph.EqualFilterOperator:
		if isString(filter.Value) {
			stmt = fmt.Sprintf("AND %s = %s", filter.Key, filter.Value)
			ok = true
		} else if isInteger(filter.Value) {
			stmt = fmt.Sprintf("AND %s = %d", filter.Key, filter.Value)
			ok = true
		}
	case graph.GreaterThanFilterOperator:
		if isNumeric(filter.Value) {
			stmt = fmt.Sprintf("AND %s > %d", filter.Key, filter.Value)
			ok = true
		}
	case graph.NotInOperator:
		var val []interface{}
		if val, ok = filter.Value.([]interface{}); ok {
			stmt = "NOT IN " + joinValues(val)
		}
	case graph.InOperator:
		var val []interface{}
		if val, ok = filter.Value.([]interface{}); ok {
			stmt = "IN " + joinValues(val)
		}
	}
	return
}

func filterGreaterThan(field string, val int64) string {
	return fmt.Sprintf("AND %s > %d", field, val)
}

func isString(val interface{}) bool {
	_, ok := val.(string)
	return ok
}

func isSlice(val interface{}) bool {
	_, ok := val.([]interface{})
	return ok
}

func isNumeric(val interface{}) bool {
	return isFloat(val) || isInteger(val)
}

func isFloat(val interface{}) bool {
	switch val.(type) {
	case float64:
	case float32:
	default:
		return false
	}
	return true
}

func isInteger(val interface{}) bool {
	switch val.(type) {
	case int:
	case int64:
	case int32:
	case int16:
	case int8:
	case uint:
	case uint64:
	case uint32:
	case uint16:
	case uint8:
	default:
		return false
	}
	return true
}

func joinValues(val []interface{}) (s string) {
	s = "()"
	if l := len(val); l > 0 {
		str := strings.Builder{}
		str.WriteString("(" + encodeFilterValue(val[0]))
		for i := 1; i < l; i++ {
			str.WriteString(", " + encodeFilterValue(i))
		}
		str.WriteString(")")
		s = str.String()
	}
	return s
}

func encodeFilterValue(v interface{}) string {
	if s, ok := v.(string); ok {
		return "'" + s + "'"
	} else if isFloat(v) {
		return fmt.Sprintf("%f", v)
	} else if isInteger(v) {
		return fmt.Sprintf("%d", v)
	} else {
		return ""
	}
}
