package redaction

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

// Salt holds a random salt and generation ID for consistent hashing within a server session.
// The salt is generated once at initialization and stays in memory — it is never persisted.
// After a server restart a new salt and generation ID are created, so hashes are not
// comparable across restarts. The generation ID is included in redacted values so that
// consumers can detect when the salt changed.
type Salt struct {
	bytes        []byte
	generationID string
}

var (
	globalSalt     *Salt
	globalSaltOnce sync.Once
)

// GlobalSalt returns the process-wide Salt, creating it on the first call.
func GlobalSalt() *Salt {
	globalSaltOnce.Do(func() {
		globalSalt = newSalt()
	})
	return globalSalt
}

func newSalt() *Salt {
	saltBytes := make([]byte, 32)
	if _, err := rand.Read(saltBytes); err != nil {
		panic(fmt.Sprintf("failed to generate redaction salt: %v", err))
	}
	genID := make([]byte, 4)
	if _, err := rand.Read(genID); err != nil {
		panic(fmt.Sprintf("failed to generate redaction generation ID: %v", err))
	}
	return &Salt{
		bytes:        saltBytes,
		generationID: hex.EncodeToString(genID),
	}
}

// Hash returns a truncated HMAC-SHA256 of the given value using this salt.
// The result is a 16-character hex string.
func (s *Salt) Hash(value string) string {
	mac := hmac.New(sha256.New, s.bytes)
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}

// GenerationID returns the short random ID for this salt generation.
func (s *Salt) GenerationID() string {
	return s.generationID
}
