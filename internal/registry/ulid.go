package registry

import (
	"apery/internal/rng"
	"time"

	"github.com/oklog/ulid/v2"
)

const defaultUlidTimestamp = "2026-01-01T00:00:00Z"

var defaultUlidTime, _ = time.Parse(time.RFC3339, defaultUlidTimestamp)

// ULIDGenerator generates random ULID strings
type ULIDGenerator struct{}

func (u *ULIDGenerator) Next(r *rng.Rng) (any, error) {
	id, err := ulid.New(ulid.Timestamp(defaultUlidTime), r)
	if err != nil {
		return nil, err
	}

	return id.String(), nil
}

func init() {
	Register("ulid", func(config map[string]any) (Generator, error) {
		return &ULIDGenerator{}, nil
	})
}
