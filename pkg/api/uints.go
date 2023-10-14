package api

import "errors"

type UintData struct{}

func (s UintData) From(i uint64) *Data {
	d := s.New()
	d.Uints = []uint64{i}
	return d
}

func (s UintData) FromAll(i ...uint64) *Data {
	d := s.NewList()
	d.Uints = i
	return d
}

func (UintData) New() *Data {
	d := &Data{}
	d.Reset()
	d.Type = DataType_UINT
	return d
}

func (s UintData) NewList() *Data {
	d := s.New()
	d.IsList = true
	return d
}

func (UintData) Type() DataType {
	return DataType_UINT
}

func (UintData) Append(value interface{}, data *Data) error {

	if data == nil {
		return errors.New("data must not be nil")
	} else if data.Type != DataType_UINT || !data.IsList {
		return dataTypeMismatchError
	}

	var i uint64
	if ii, ok := value.(uint64); ok {
		i = ii
	} else if ptr, ok := value.(*uint64); ok {
		if ptr != nil {
			i = *ptr
		}
	} else {
		return dataTypeMismatchError
	}
	data.Uints = append(data.Uints, i)

	return nil
}

func (s UintData) MustAppend(value interface{}, data *Data) {
	if err := s.Append(value, data); err != nil {
		panic("error appending value to []int64")
	}
}

func (UintData) NewValue() interface{} {
	var i uint64
	return &i
}

func (UintData) Encode(value interface{}, data *Data) (err error) {

	if data != nil {
		data.Reset()
		data.Type = DataType_UINT
	} else {
		return errors.New("data must not be nil")
	}

	if i, ok := value.(uint64); ok {
		data.Uints = []uint64{i}
	} else if ptr, ok := value.(*uint64); ok {
		if ptr != nil {
			data.Uints = []uint64{*ptr}
		}
	} else if i, ok := value.(int64); ok && i >= 0 {
		data.Uints = []uint64{uint64(i)}
	} else if ptr, ok := value.(*int64); ok {
		if ptr != nil {
			if i := *ptr; i >= 0 {
				data.Uints = []uint64{uint64(i)}
			} else {
				err = dataTypeMismatchError
			}
		}
	} else {
		err = dataTypeMismatchError
	}

	return
}

func (s UintData) MustEncode(value interface{}, data *Data) {
	if e := s.Encode(value, data); e != nil {
		panic("error encoding value to int data")
	}
}

func (UintData) Decode(data *Data, value interface{}) error {

	if data == nil {
		return errors.New("data must not be nil")
	} else if data.Type != DataType_UINT || data.IsList {
		return dataTypeMismatchError
	}

	if ptr, ok := value.(*uint64); !ok {
		return dataTypeMismatchError
	} else if len(data.Uints) == 0 {
		ptr = nil
	} else {
		*ptr = data.Uints[0]
	}

	return nil
}

func (s UintData) MustDecode(data *Data, value interface{}) {
	if err := s.Decode(data, value); err != nil {
		panic("error decoding string value")
	}
}
