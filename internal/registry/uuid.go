package registry

import (
	"github.com/compuficial/apery/internal/rng"

	"github.com/google/uuid"
)

const (
	uuidByteLength = 16
	byteMax        = 256

	// Byte positions for UUID version and variant
	versionByteIndex = 6
	variantByteIndex = 8

	// Masks and values for UUID v4 (RFC 4122)
	versionMask    = 0x0f
	version4       = 0x40
	variantMask    = 0x3f
	variantRFC4122 = 0x80
)

// UUIDGenerator generates random UUID v4 strings
type UUIDGenerator struct{}

// Next returns the next generated UUID string.
func (u *UUIDGenerator) Next(r *rng.Rng) (any, error) {
	var bytes [uuidByteLength]byte
	for i := range uuidByteLength {
		bytes[i] = byte(r.Intn(byteMax))
	}

	// Set version 4 and RFC 4122 variant
	bytes[versionByteIndex] = (bytes[versionByteIndex] & versionMask) | version4
	bytes[variantByteIndex] = (bytes[variantByteIndex] & variantMask) | variantRFC4122

	return uuid.UUID(bytes).String(), nil
}

// init registers the uuid generator.
func init() {
	MustRegister("uuid", func(config map[string]any) (Generator, error) {
		return &UUIDGenerator{}, nil
	})
	MustRegisterInfo("uuid", GeneratorInfo{
		Description: "UUID v4 string",
		Example: `- name: id
  gen: uuid`,
	})
}
