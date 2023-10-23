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

package graph

import (
	"context"

	"github.com/raft-tech/syncd/internal/api"
)

type Graph interface {
	Source(peer string, filters ...Filter) Source
	Destination() Destination
}

type Source interface {
	Fetch(ctx context.Context) <-chan *api.Record
	Error() error
	SetStatus(context.Context, ...*api.RecordStatus) error
}

type Destination interface {
	Write(context.Context, <-chan *api.Record) <-chan *api.RecordStatus
	Error() error
}

type Factory interface {
	Build(ctx context.Context, m *Model) (Graph, error)
	Close(ctx context.Context) error
}

type Model struct {
	Name     string
	Table    Table
	IsSet    bool
	ChildKey string
	Children []Model
	Filters  []string
}

type Table struct {
	Name          string
	KeyField      string
	SequenceField string
	PriorityField string
	VersionField  string
}

type Filter struct {
	Key      string
	Operator FilterOperator
	Value    interface{}
}

type FilterOperator string

const (
	GreaterThanFilterOperator FilterOperator = "GreaterThan"
	EqualFilterOperator       FilterOperator = "Equal"
	InOperator                FilterOperator = "In"
	NotInOperator             FilterOperator = "NotIn"
)
