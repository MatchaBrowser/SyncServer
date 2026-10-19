package middleware

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/matchabrowser/syncserver/interfaces/http/problem"

	"github.com/gofiber/fiber/v3"
)

// ClientHeader carries the version of Matcha: "<version> (<build>)"
// (SYNC.md § 4.1).
const ClientHeader = "Matcha-Client"

// ClientVersion answers 426 unsupported_client to a Matcha older than
// minimum, or that does not say its version. It returns a nil handler when
// minimum is empty: every version is accepted.
func ClientVersion(minimum string) (fiber.Handler, error) {
	if strings.TrimSpace(minimum) == "" {
		return nil, nil
	}
	required, err := ParseVersion(minimum)
	if err != nil {
		return nil, fmt.Errorf("http.minimum_client_version: %w", err)
	}

	return func(c fiber.Ctx) error {
		version, _, _ := strings.Cut(strings.TrimSpace(c.Get(ClientHeader)), " ")
		current, err := ParseVersion(version)
		if err != nil || current.Less(required) {
			return problem.Write(c, problem.New(problem.UnsupportedClient, fmt.Sprintf("Matcha %s or later is required", minimum)))
		}
		return c.Next()
	}, nil
}

// Version is a major.minor.patch version.
type Version [3]int

// ParseVersion reads "1", "1.4" or "1.4.2". A pre-release or build suffix
// ("1.4.2-beta", "1.4.2+42") is ignored.
func ParseVersion(s string) (Version, error) {
	core, _, _ := strings.Cut(s, "-")
	core, _, _ = strings.Cut(core, "+")
	parts := strings.Split(core, ".")
	if core == "" || len(parts) > 3 {
		return Version{}, fmt.Errorf("invalid version %q", s)
	}
	var v Version
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("invalid version %q", s)
		}
		v[i] = n
	}
	return v, nil
}

// Less reports whether v is older than other.
func (v Version) Less(other Version) bool {
	for i := range v {
		if v[i] != other[i] {
			return v[i] < other[i]
		}
	}
	return false
}
