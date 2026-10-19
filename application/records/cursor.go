package records

import (
	"encoding/base64"
	"encoding/binary"

	"github.com/matchabrowser/syncserver/domain"

	"github.com/google/uuid"
)

// Cursor is the position of a device in the changes of its account: the
// epoch of the database and the last version read. It is opaque for the
// client (SYNC.md § 4.6).
type Cursor struct {
	Epoch   string
	Version int64
}

// Encode writes the cursor in base 64 URL: 16 bytes of epoch, 8 of version.
func (c Cursor) Encode() string {
	epoch, err := uuid.Parse(c.Epoch)
	if err != nil {
		epoch = uuid.Nil
	}
	b := make([]byte, 24)
	copy(b, epoch[:])
	binary.BigEndian.PutUint64(b[16:], uint64(c.Version))
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor reads a cursor written by Encode.
func DecodeCursor(s string) (Cursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(b) != 24 {
		return Cursor{}, domain.ErrInvalidCursor
	}
	epoch, err := uuid.FromBytes(b[:16])
	if err != nil {
		return Cursor{}, domain.ErrInvalidCursor
	}
	version := int64(binary.BigEndian.Uint64(b[16:]))
	if version < 0 {
		return Cursor{}, domain.ErrInvalidCursor
	}
	return Cursor{Epoch: epoch.String(), Version: version}, nil
}
