package ingest

import (
	"errors"
	"time"
)

var (
	ErrInvalidLat  = errors.New("invalid latitude")
	ErrInvalidLng  = errors.New("invalid longitude")
	ErrInvalidTime = errors.New("invalid timestamp")
)

func ValidateCoordinates(lat, lng float64) error {
	if lat < -90 || lat > 90 {
		return ErrInvalidLat
	}
	if lng < -180 || lng > 180 {
		return ErrInvalidLng
	}
	return nil
}

func ParseTimestamp(value string) (time.Time, error) {
	if value == "" {
		return time.Now().UTC(), nil
	}
	ts, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, ErrInvalidTime
	}
	return ts.UTC(), nil
}
