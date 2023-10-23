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

type StringData struct{}

func (s StringData) From(str string) *Data {
	d := s.New()
	d.Strings = []string{str}
	return d
}

func (s StringData) FromAll(str ...string) *Data {
	d := s.NewList()
	d.Strings = str
	return d
}

func (StringData) New() *Data {
	d := &Data{}
	d.Reset()
	d.Type = DataType_STRING
	return d
}

func (s StringData) NewList() *Data {
	d := s.New()
	d.IsList = true
	return d
}

func (StringData) Type() DataType {
	return DataType_STRING
}

func (StringData) Append(value interface{}, data *Data) error {

	if data == nil {
		return errors.New("data must not be nil")
	} else if data.Type != DataType_STRING || !data.IsList {
		return dataTypeMismatchError
	}

	var str string
	if s, ok := value.(string); ok {
		str = s
	} else if ptr, ok := value.(*string); ok {
		if ptr != nil {
			str = *ptr
		}
	} else {
		return dataTypeMismatchError
	}
	data.Strings = append(data.Strings, str)

	return nil
}

func (s StringData) MustAppend(value interface{}, data *Data) {
	if err := s.Append(value, data); err != nil {
		panic("error appending value to []string")
	}
}

func (StringData) NewValue() interface{} {
	var str string
	return &str
}

func (StringData) Encode(value interface{}, data *Data) (err error) {

	if data != nil {
		data.Reset()
		data.Type = DataType_STRING
	} else {
		return errors.New("data must not be nil")
	}

	if str, ok := value.(string); ok {
		data.Strings = []string{str}
	} else if ptr, ok := value.(*string); ok {
		if ptr != nil {
			data.Strings = []string{*ptr}
		}
	} else {
		err = dataTypeMismatchError
	}

	return
}

func (s StringData) MustEncode(value interface{}, data *Data) {
	if e := s.Encode(value, data); e != nil {
		panic("error encoding value to string data")
	}
}

func (StringData) Decode(data *Data, value interface{}) error {

	if data == nil {
		return errors.New("data must not be nil")
	} else if data.Type != DataType_STRING || data.IsList {
		return dataTypeMismatchError
	}

	if ptr, ok := value.(*string); !ok {
		return dataTypeMismatchError
	} else if len(data.Strings) == 0 {
		ptr = nil
	} else {
		*ptr = data.Strings[0]
	}

	return nil
}

func (s StringData) MustDecode(data *Data, value interface{}) {
	if err := s.Decode(data, value); err != nil {
		panic("error decoding string value")
	}
}
