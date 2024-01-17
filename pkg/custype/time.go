package custype

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"time"
)

type TimeSecond int64

func ToTimeSecond(t time.Time) TimeSecond {
	return TimeSecond(t.Unix())
}
func (t TimeSecond) ToInt64() int64 {
	return int64(t)
}
func (t TimeSecond) ToTime() time.Time {
	return time.Unix(int64(t), 0)
}
func (t TimeSecond) Value() (driver.Value, error) {
	return t.ToTime(), nil
}
func (t *TimeSecond) Scan(src any) error {
	switch s := src.(type) {
	case time.Time:
		*t = ToTimeSecond(s)
	case int64:
		*t = TimeSecond(s)
	case nil:
		*t = 0
	default:
		return fmt.Errorf("unsupported Scan, storing driver.Value type %T into type time.Time", src)
	}
	return nil
}
func (t TimeSecond) String() string {
	return strconv.FormatInt(int64(t), 10)
}

type TimeMillisecond int64

func ToTimeMillisecond(t time.Time) TimeMillisecond {
	return TimeMillisecond(t.UnixMilli())
}
func (t TimeMillisecond) ToInt64() int64 {
	return int64(t)
}
func (t TimeMillisecond) ToTime() time.Time {
	return time.UnixMilli(int64(t))
}
func (t TimeMillisecond) Value() (driver.Value, error) {
	return t.ToTime(), nil
}
func (t *TimeMillisecond) Scan(src any) error {
	switch s := src.(type) {
	case time.Time:
		*t = ToTimeMillisecond(s)
	case int64:
		*t = TimeMillisecond(s)
	case nil:
		*t = 0
	default:
		return fmt.Errorf("unsupported Scan, storing driver.Value type %T into type time.Time", src)
	}
	return nil
}
func (t TimeMillisecond) String() string {
	return strconv.FormatInt(int64(t), 10)
}
