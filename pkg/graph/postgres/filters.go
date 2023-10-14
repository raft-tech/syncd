package postgres

import (
	"fmt"
	"github.com/raft-tech/syncd/pkg/graph"
)

func filterStatement(filter *graph.Filter) (stmt string, ok bool) {
	switch filter.Operator {
	case graph.GreaterThanFilterOperator:
		var val int64
		if val, ok = filter.Value.(int64); !ok {
			if v, ook := filter.Value.(int); ook {
				val = int64(v)
				ok = true
			} else {
				return
			}
		}
		stmt = fmt.Sprintf("AND %s > %d", filter.Key, val)
	}
	return
}
