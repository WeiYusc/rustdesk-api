package custom_types

import (
	"database/sql/driver"
	"time"
)

// AutoTime 自定义时间格式
type AutoTime time.Time

func (mt AutoTime) Value() (driver.Value, error) {
	var zeroTime time.Time
	t := time.Time(mt)
	if t.UnixNano() == zeroTime.UnixNano() {
		return nil, nil
	}
	return t, nil
}

func (mt AutoTime) MarshalJSON() ([]byte, error) {
	t := time.Time(mt)
	if t.IsZero() {
		return []byte("null"), nil
	}
	return t.MarshalJSON()
}
