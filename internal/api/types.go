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

package api

import (
	"errors"
)

const API_VERSION uint32 = 1

var (
	dataTypeMismatchError = errors.New("data type mismatch")
)

const (
	nilRecordError uint32 = iota
	localRecordError
	remoteRecordError
)

func CheckInfo() *Info {
	return &Info{
		ApiVersion: API_VERSION,
	}
}

func NoRecordError() *uint32 {
	v := nilRecordError
	return &v
}

func LocalRecordError() *uint32 {
	v := localRecordError
	return &v
}

type IDataType interface {
	New() *Data
	NewList() *Data
	NewValue() interface{}
	Type() DataType
	Append(value interface{}, data *Data) error
	MustAppend(value interface{}, data *Data)
	Encode(value interface{}, data *Data) error
	MustEncode(value interface{}, data *Data)
	Decode(data *Data, value interface{}) error
	MustDecode(data *Data, value interface{})
}

func (d *Data) Describe() string {
	if d == nil {
		return "<NIL>"
	}
	str := DataType_name[int32(d.Type)]
	if d.IsList {
		str = "[]" + str
	}
	return str
}
