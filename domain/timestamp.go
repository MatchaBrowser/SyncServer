package domain

import (
	"encoding/json"
	"time"
)

// TimestampLayout is the format of the dates of the API: ISO 8601, UTC, with
// exactly three digits of milliseconds (SYNC.md § 4.1).
const TimestampLayout = "2006-01-02T15:04:05.000Z"

// Timestamp is a date of the API.
type Timestamp time.Time

func (t Timestamp) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(t).UTC().Format(TimestampLayout))
}

func (t *Timestamp) UnmarshalJSON(data []byte) error {
	var parsed time.Time
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*t = Timestamp(parsed)
	return nil
}
