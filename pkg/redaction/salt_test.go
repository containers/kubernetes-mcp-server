package redaction

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type SaltSuite struct {
	suite.Suite
}

func (s *SaltSuite) TestNewSalt() {
	s.Run("generates 32-byte key and 8-char generation ID", func() {
		salt := newSalt()
		s.Require().NotNil(salt)
		s.Len(salt.bytes, 32)
		s.Len(salt.generationID, 8, "4 bytes = 8 hex chars")
	})
}

func (s *SaltSuite) TestHashConsistency() {
	s.Run("same value produces same hash with same salt", func() {
		salt := newSalt()
		hash1 := salt.Hash("my-secret-value")
		hash2 := salt.Hash("my-secret-value")
		s.Equal(hash1, hash2)
		s.Len(hash1, 16)
	})
}

func (s *SaltSuite) TestHashDifference() {
	s.Run("different values produce different hashes", func() {
		salt := newSalt()
		hash1 := salt.Hash("value-a")
		hash2 := salt.Hash("value-b")
		s.NotEqual(hash1, hash2)
	})
}

func (s *SaltSuite) TestDifferentSalts() {
	s.Run("different salts produce different hashes for same value", func() {
		s1 := newSalt()
		s2 := newSalt()
		hash1 := s1.Hash("same-value")
		hash2 := s2.Hash("same-value")
		s.NotEqual(hash1, hash2)
	})
}

func (s *SaltSuite) TestGlobalSaltSingleton() {
	s.Run("returns the same instance on repeated calls", func() {
		salt1 := GlobalSalt()
		salt2 := GlobalSalt()
		s.Equal(salt1, salt2)
	})
}

func TestSalt(t *testing.T) {
	suite.Run(t, new(SaltSuite))
}
