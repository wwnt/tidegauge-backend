package custype

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"time"
)

type UnixMs int64

func ToUnixMs(t time.Time) UnixMs {
	return UnixMs(t.UnixMilli())
}

func (t UnixMs) ToInt64() int64 {
	return int64(t)
}

func (t UnixMs) ToTime() time.Time {
	return time.UnixMilli(int64(t))
}

func (t UnixMs) Value() (driver.Value, error) {
	return t.ToTime(), nil
}

func (t *UnixMs) Scan(src any) error {
	switch s := src.(type) {
	case time.Time:
		*t = ToUnixMs(s)
	case int64:
		*t = UnixMs(s)
	case nil:
		*t = 0
	default:
		return fmt.Errorf("unsupported Scan, storing driver.Value type %T into type time.Time", src)
	}
	return nil
}

func (t UnixMs) String() string {
	return strconv.FormatInt(int64(t), 10)
}
