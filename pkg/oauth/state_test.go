package oauth

import (
	"sync"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
)

type OAuthStateSuite struct {
	suite.Suite
}

func TestOAuthState(t *testing.T) {
	suite.Run(t, new(OAuthStateSuite))
}

func (s *OAuthStateSuite) TestHasProviderConfigChanged() {
	s.Run("nil snapshots", func() {
		var snap *Snapshot
		s.True(snap.HasProviderConfigChanged(&Snapshot{}))
		s.True((&Snapshot{}).HasProviderConfigChanged(nil))
		s.True((*Snapshot)(nil).HasProviderConfigChanged(&Snapshot{AuthorizationURL: "https://example.com"}))
	})

	s.Run("both nil", func() {
		var a, b *Snapshot
		s.False(a.HasProviderConfigChanged(b))
	})

	s.Run("same values", func() {
		a := &Snapshot{AuthorizationURL: "https://auth.example.com", CertificateAuthority: "/ca.pem"}
		b := &Snapshot{AuthorizationURL: "https://auth.example.com", CertificateAuthority: "/ca.pem"}
		s.False(a.HasProviderConfigChanged(b))
	})

	s.Run("authorization URL changed", func() {
		a := &Snapshot{AuthorizationURL: "https://old.example.com"}
		b := &Snapshot{AuthorizationURL: "https://new.example.com"}
		s.True(a.HasProviderConfigChanged(b))
	})

	s.Run("certificate authority changed", func() {
		a := &Snapshot{CertificateAuthority: "/old-ca.pem"}
		b := &Snapshot{CertificateAuthority: "/new-ca.pem"}
		s.True(a.HasProviderConfigChanged(b))
	})

	s.Run("non-provider fields do not trigger change", func() {
		a := &Snapshot{AuthorizationURL: "https://auth.example.com", OAuthScopes: []string{"a"}}
		b := &Snapshot{AuthorizationURL: "https://auth.example.com", OAuthScopes: []string{"b"}}
		s.False(a.HasProviderConfigChanged(b))
	})
}

func (s *OAuthStateSuite) TestHasWellKnownConfigChanged() {
	s.Run("scopes changed", func() {
		a := &Snapshot{OAuthScopes: []string{"openid"}}
		b := &Snapshot{OAuthScopes: []string{"openid", "profile"}}
		s.True(a.HasWellKnownConfigChanged(b))
	})

	s.Run("disable dynamic client registration changed", func() {
		a := &Snapshot{DisableDynamicClientRegistration: false}
		b := &Snapshot{DisableDynamicClientRegistration: true}
		s.True(a.HasWellKnownConfigChanged(b))
	})

	s.Run("provider config change implies wellknown change", func() {
		a := &Snapshot{AuthorizationURL: "https://old.example.com"}
		b := &Snapshot{AuthorizationURL: "https://new.example.com"}
		s.True(a.HasWellKnownConfigChanged(b))
	})

	s.Run("no changes", func() {
		a := &Snapshot{
			AuthorizationURL:                 "https://auth.example.com",
			OAuthScopes:                      []string{"openid"},
			DisableDynamicClientRegistration: true,
		}
		b := &Snapshot{
			AuthorizationURL:                 "https://auth.example.com",
			OAuthScopes:                      []string{"openid"},
			DisableDynamicClientRegistration: true,
		}
		s.False(a.HasWellKnownConfigChanged(b))
	})
}

func (s *OAuthStateSuite) TestStateLoadStore() {
	s.Run("load returns stored snapshot", func() {
		snap := &Snapshot{AuthorizationURL: "https://auth.example.com"}
		state := NewState(snap)
		s.Equal(snap, state.Load())
	})

	s.Run("store replaces snapshot", func() {
		snap1 := &Snapshot{AuthorizationURL: "https://old.example.com"}
		snap2 := &Snapshot{AuthorizationURL: "https://new.example.com"}
		state := NewState(snap1)
		state.Store(snap2)
		s.Equal(snap2, state.Load())
	})

	s.Run("concurrent access is safe", func() {
		state := NewState(&Snapshot{AuthorizationURL: "https://initial.example.com"})
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				state.Store(&Snapshot{AuthorizationURL: "https://new.example.com"})
			}()
			go func() {
				defer wg.Done()
				snap := state.Load()
				s.NotNil(snap)
			}()
		}
		wg.Wait()
	})
}

func (s *OAuthStateSuite) TestCreateOIDCProviderAndClient() {
	s.Run("empty authorization URL returns nil", func() {
		cfg := config.Default()
		provider, client, err := CreateOIDCProviderAndClient(cfg)
		s.NoError(err)
		s.Nil(provider)
		s.Nil(client)
	})
}
