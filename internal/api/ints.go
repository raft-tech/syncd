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

package api

import "errors"

type IntData struct{}

func (s IntData) From(i int64) *Data {
	d := s.New()
	d.Ints = []int64{i}
	return d
}

func (s IntData) FromAll(i ...int64) *Data {
	d := s.NewList()
	d.Ints = i
	return d
}

func (IntData) New() *Data {
	d := &Data{}
	d.Reset()
	d.Type = DataType_INT
	return d
}

func (s IntData) NewList() *Data {
	d := s.New()
	d.IsList = true
	return d
}

func (IntData) Type() DataType {
	return DataType_INT
}

func (IntData) Append(value interface{}, data *Data) error {

	if data == nil {
		return errors.New("data must not be nil")
	} else if data.Type != DataType_INT || !data.IsList {
		return dataTypeMismatchError
	}

	var i int64
	if ii, ok := value.(int64); ok {
		i = ii
	} else if ptr, ok := value.(*int64); ok {
		if ptr != nil {
			i = *ptr
		}
	} else {
		return dataTypeMismatchError
	}
	data.Ints = append(data.Ints, i)

	return nil
}

func (s IntData) MustAppend(value interface{}, data *Data) {
	if err := s.Append(value, data); err != nil {
		panic("error appending value to []int64")
	}
}

func (IntData) NewValue() interface{} {
	var i int64
	return &i
}

func (IntData) Encode(value interface{}, data *Data) (err error) {

	if data != nil {
		data.Reset()
		data.Type = DataType_INT
	} else {
		return errors.New("data must not be nil")
	}

	if i, ok := value.(int64); ok {
		data.Ints = []int64{i}
	} else if ptr, ok := value.(*int64); ok {
		if ptr != nil {
			data.Ints = []int64{*ptr}
		}
	} else {
		err = dataTypeMismatchError
	}

	return
}

func (s IntData) MustEncode(value interface{}, data *Data) {
	if e := s.Encode(value, data); e != nil {
		panic("error encoding value to int data")
	}
}

func (IntData) Decode(data *Data, value interface{}) error {

	if data == nil {
		return errors.New("data must not be nil")
	} else if data.Type != DataType_INT || data.IsList {
		return dataTypeMismatchError
	}

	if ptr, ok := value.(*int64); !ok {
		return dataTypeMismatchError
	} else if len(data.Ints) == 0 {
		ptr = nil
	} else {
		*ptr = data.Ints[0]
	}

	return nil
}

func (s IntData) MustDecode(data *Data, value interface{}) {
	if err := s.Decode(data, value); err != nil {
		panic("error decoding string value")
	}
}
