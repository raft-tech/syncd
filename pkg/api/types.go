package api

import (
	"errors"
)

var (
	dataTypeMismatchError = errors.New("data type mismatch")
)

const (
	nilRecordError uint32 = iota
	localRecordError
	remoteRecordError
)

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
