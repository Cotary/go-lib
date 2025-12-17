package pgsql

import "github.com/lib/pq"

type XormStringArray pq.StringArray

func (s *XormStringArray) FromDB(bts []byte) error {
	if len(bts) == 0 {
		*s = XormStringArray{}
		return nil
	}
	// 直接复用 pq.StringArray 的 Scan 方法
	return (*pq.StringArray)(s).Scan(bts)
}

func (s XormStringArray) ToDB() ([]byte, error) {
	if s == nil || len(s) == 0 {
		return []byte("{}"), nil
	}
	// 直接复用 pq.StringArray 的 Value 方法
	val, err := pq.StringArray(s).Value()
	if err != nil {
		return nil, err
	}
	if val == nil {
		return []byte("{}"), nil
	}
	return []byte(val.(string)), nil
}

type XormInt64Array pq.Int64Array

func (s *XormInt64Array) FromDB(bts []byte) error {
	if len(bts) == 0 {
		*s = XormInt64Array{}
		return nil
	}
	// 直接复用 pq.Int64Array 的 Scan 方法
	return (*pq.Int64Array)(s).Scan(bts)
}

func (s XormInt64Array) ToDB() ([]byte, error) {
	if s == nil || len(s) == 0 {
		return []byte("{}"), nil
	}
	// 直接复用 pq.Int64Array 的 Value 方法
	val, err := pq.Int64Array(s).Value()
	if err != nil {
		return nil, err
	}
	if val == nil {
		return []byte("{}"), nil
	}
	return []byte(val.(string)), nil
}
