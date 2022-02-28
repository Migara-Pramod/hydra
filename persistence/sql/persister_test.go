package sql_test

import (
	"context"
	"testing"

	"github.com/pkg/errors"

	"github.com/gofrs/uuid"

	"github.com/ory/hydra/client"
	"github.com/ory/hydra/consent"
	"github.com/ory/hydra/internal/testhelpers"
	"github.com/ory/hydra/oauth2/trust"
	"github.com/ory/x/networkx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ory/hydra/jwk"

	"github.com/ory/hydra/driver"
	"github.com/ory/hydra/internal"
)

func TestManagers(t *testing.T) {
	ctx := context.TODO()
	registries := map[string]driver.Registry{
		"memory": internal.NewRegistrySQLFromURL(t, "sqlite://file::memory:?cache=shared&_busy_timeout=5000&_fk=true", nil, true),
	}

	tenant2NID, _ := uuid.NewV4()
	tenant2Registries := map[string]driver.Registry{
		"memory": internal.NewRegistrySQLFromURL(t, "sqlite://file::memory:?cache=shared&_busy_timeout=5000&_fk=true", &tenant2NID, true),
	}

	if !testing.Short() {
		registries["postgres"], registries["mysql"], registries["cockroach"], _ = internal.ConnectDatabases(t, nil)
		tenant2Registries["postgres"], tenant2Registries["mysql"], tenant2Registries["cockroach"], _ = internal.ConnectDatabases(t, &tenant2NID)
	}

	for k, t1 := range registries {
		t2 := tenant2Registries[k]
		t2.Persister().Connection(ctx).Create(&networkx.Network{ID: tenant2NID})

		t.Run("package=client/manager="+k, func(t *testing.T) {
			t.Run("case=create-get-update-delete", client.TestHelperCreateGetUpdateDeleteClient(k, t1.ClientManager(), t2.ClientManager()))

			t.Run("case=autogenerate-key", client.TestHelperClientAutoGenerateKey(k, t1.ClientManager()))

			t.Run("case=auth-client", client.TestHelperClientAuthenticate(k, t1.ClientManager()))

			t.Run("case=update-two-clients", client.TestHelperUpdateTwoClients(k, t1.ClientManager()))
		})

		parallel := true
		if k == "memory" || k == "cockroach" { // TODO enable parallel tests for cockroach once we swap the transaction wrapper for one that supports retry
			parallel = false
		}

		t.Run("package=consent/manager="+k, consent.ManagerTests(t1.ConsentManager(), t1.ClientManager(), t1.OAuth2Storage(), "t1", parallel))
		t.Run("package=consent/manager="+k, consent.ManagerTests(t2.ConsentManager(), t2.ClientManager(), t2.OAuth2Storage(), "t2", parallel))

		t.Run("parallel-boundary", func(t *testing.T) {
			t.Run("package=consent/janitor="+k, testhelpers.JanitorTests(t1.Config(ctx), t1.ConsentManager(), t1.ClientManager(), t1.OAuth2Storage(), "t1", parallel))
			t.Run("package=consent/janitor="+k, testhelpers.JanitorTests(t2.Config(ctx), t2.ConsentManager(), t2.ClientManager(), t2.OAuth2Storage(), "t2", parallel))
		})

		t.Run("package=jwk/manager="+k, func(t *testing.T) {
			keyGenerators := new(driver.RegistryBase).KeyGenerators()
			assert.Equalf(t, 6, len(keyGenerators), "Test for key generator is not implemented")

			for _, tc := range []struct {
				keyGenerator jwk.KeyGenerator
				alg          string
				skip         bool
			}{
				{keyGenerator: keyGenerators["RS256"], alg: "RS256", skip: false},
				{keyGenerator: keyGenerators["ES256"], alg: "ES256", skip: false},
				{keyGenerator: keyGenerators["ES512"], alg: "ES512", skip: false},
				{keyGenerator: keyGenerators["HS256"], alg: "HS256", skip: true},
				{keyGenerator: keyGenerators["HS512"], alg: "HS512", skip: true},
				{keyGenerator: keyGenerators["EdDSA"], alg: "EdDSA", skip: t1.Config(ctx).HsmEnabled()},
			} {
				t.Run("key_generator="+tc.alg, func(t *testing.T) {
					if tc.skip {
						t.Skipf("Skipping test. Not applicable for alg: %s", tc.alg)
					}
					if t1.Config(ctx).HsmEnabled() {
						t.Run("TestManagerGenerateAndPersistKeySet", jwk.TestHelperManagerGenerateAndPersistKeySet(t1.KeyManager(), tc.alg, false))
						t.Run("TestManagerGenerateAndPersistKeySet", jwk.TestHelperManagerNIDIsolationKeySet(t1.KeyManager(), t2.KeyManager(), tc.alg))
					} else {
						kid, err := uuid.NewV4()
						require.NoError(t, err)
						ks, err := tc.keyGenerator.Generate(kid.String(), "sig")
						require.NoError(t, err)
						t.Run("TestManagerKey", jwk.TestHelperManagerKey(t1.KeyManager(), tc.alg, ks, kid.String()))
						t.Run("Parallel", func(t *testing.T) {
							t.Run("TestManagerKeySet", jwk.TestHelperManagerKeySet(t1.KeyManager(), tc.alg, ks, kid.String(), parallel))
							t.Run("TestManagerKeySet", jwk.TestHelperManagerKeySet(t2.KeyManager(), tc.alg, ks, kid.String(), parallel))
						})
						t.Run("Parallel", func(t *testing.T) {
							t.Run("TestManagerGenerateAndPersistKeySet", jwk.TestHelperManagerGenerateAndPersistKeySet(t1.KeyManager(), tc.alg, parallel))
							t.Run("TestManagerGenerateAndPersistKeySet", jwk.TestHelperManagerGenerateAndPersistKeySet(t2.KeyManager(), tc.alg, parallel))
						})
					}
				})
			}

			t.Run("TestManagerGenerateAndPersistKeySetWithUnsupportedKeyGenerator", func(t *testing.T) {
				_, err := t1.KeyManager().GenerateAndPersistKeySet(context.TODO(), "foo", "bar", "UNKNOWN", "sig")
				require.Error(t, err)
				assert.IsType(t, errors.WithStack(jwk.ErrUnsupportedKeyAlgorithm), err)
			})
		})

		t.Run("package=grant/trust/manager="+k, func(t *testing.T) {
			t.Run("parallel-boundary", func(t *testing.T) {
				t.Run("case=create-get-delete/tenant=t1", trust.TestHelperGrantManagerCreateGetDeleteGrant(t1.GrantManager(), parallel))
				t.Run("case=create-get-delete/tenant=t2", trust.TestHelperGrantManagerCreateGetDeleteGrant(t2.GrantManager(), parallel))
			})
			t.Run("parallel-boundary", func(t *testing.T) {
				t.Run("case=errors", trust.TestHelperGrantManagerErrors(t1.GrantManager(), parallel))
				t.Run("case=errors", trust.TestHelperGrantManagerErrors(t2.GrantManager(), parallel))
			})
		})
	}
}
