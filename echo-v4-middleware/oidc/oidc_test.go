package oidc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/lestrrat-go/jwx/jws"
	"github.com/lestrrat-go/jwx/jwt"
	"github.com/stretchr/testify/require"
	"github.com/xenitab/dispans/server"
)

func TestNewKeyHandler(t *testing.T) {
	op := server.NewTesting(t)
	issuer := op.GetURL(t)
	discoveryUri := getDiscoveryUriFromIssuer(issuer)
	jwksUri, err := getJwksUriFromDiscoveryUri(discoveryUri, 10*time.Millisecond)
	require.NoError(t, err)

	keyHandler, err := newKeyHandler(jwksUri, 10*time.Millisecond)
	require.NoError(t, err)

	keySet1 := keyHandler.getKeySet()
	require.Equal(t, 1, keySet1.Len())

	expectedKey1, ok := keySet1.Get(0)
	require.True(t, ok)

	token1 := op.GetToken(t)
	keyID1, err := getKeyIDFromTokenString(token1.AccessToken)
	require.NoError(t, err)

	// Test valid key id
	key1, err := keyHandler.getByKeyID(keyID1, false)
	require.NoError(t, err)
	require.Equal(t, expectedKey1, key1)

	// Test invalid key id
	_, err = keyHandler.getByKeyID("foo", false)
	require.Error(t, err)

	// Test with rotated keys
	op.RotateKeys(t)

	token2 := op.GetToken(t)
	keyID2, err := getKeyIDFromTokenString(token2.AccessToken)
	require.NoError(t, err)

	key2, err := keyHandler.getByKeyID(keyID2, false)
	require.NoError(t, err)

	keySet2 := keyHandler.getKeySet()
	require.Equal(t, 1, keySet2.Len())

	expectedKey2, ok := keySet2.Get(0)
	require.True(t, ok)

	require.Equal(t, expectedKey2, key2)

	// Test that old key doesn't match new key
	require.NotEqual(t, key1, key2)

	// Validate that error is returned when using fake jwks uri
	_, err = newKeyHandler("http://foo.bar/baz", 10*time.Millisecond)
	require.Error(t, err)

	// Validate that error is returned when keys are rotated,
	// new token with new key and jwks uri isn't accessible
	op.RotateKeys(t)
	token3 := op.GetToken(t)
	keyID3, err := getKeyIDFromTokenString(token3.AccessToken)
	require.NoError(t, err)
	op.Close(t)
	_, err = keyHandler.getByKeyID(keyID3, false)
	require.Error(t, err)
}

func TestGetHeadersFromTokenString(t *testing.T) {
	key := testNewKey(t)

	// Test with KeyID and Type
	token1 := jwt.New()
	token1.Set("foo", "bar")

	headers1 := jws.NewHeaders()
	headers1.Set(jws.KeyIDKey, "foo")
	headers1.Set(jws.TypeKey, "JWT")

	signedTokenBytes1, err := jwt.Sign(token1, jwa.ES384, key, jwt.WithHeaders(headers1))
	require.NoError(t, err)

	signedToken1 := string(signedTokenBytes1)
	parsedHeaders1, err := getHeadersFromTokenString(signedToken1)
	require.NoError(t, err)

	require.Equal(t, headers1.KeyID(), parsedHeaders1.KeyID())
	require.Equal(t, headers1.Type(), parsedHeaders1.Type())

	// Test with empty headers
	payload1 := `{"foo":"bar"}`

	headers2 := jws.NewHeaders()

	signedTokenBytes2, err := jws.Sign([]byte(payload1), jwa.ES384, key, jws.WithHeaders(headers2))
	require.NoError(t, err)

	signedToken2 := string(signedTokenBytes2)
	parsedHeaders2, err := getHeadersFromTokenString(signedToken2)
	require.NoError(t, err)

	require.Empty(t, parsedHeaders2.KeyID())
	require.Empty(t, parsedHeaders2.Type())

	// Test with multiple signatures
	payload2 := `{"foo":"bar"}`

	signer1, err := jws.NewSigner(jwa.ES384)
	require.NoError(t, err)
	signer2, err := jws.NewSigner(jwa.ES384)
	require.NoError(t, err)

	signedTokenBytes3, err := jws.SignMulti([]byte(payload2), jws.WithSigner(signer1, key, nil, nil), jws.WithSigner(signer2, key, nil, nil))
	require.NoError(t, err)

	signedToken3 := string(signedTokenBytes3)

	_, err = getHeadersFromTokenString(signedToken3)
	require.Error(t, err)
	require.Equal(t, "more than one signature in token", err.Error())

	// Test with non-token string
	_, err = getHeadersFromTokenString("foo")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unable to parse tokenString")
}

func TestGetKeyIDFromTokenString(t *testing.T) {
	key := testNewKey(t)

	// Test with KeyID
	token1 := jwt.New()
	token1.Set("foo", "bar")

	headers1 := jws.NewHeaders()
	headers1.Set(jws.KeyIDKey, "foo")

	signedTokenBytes1, err := jwt.Sign(token1, jwa.ES384, key, jwt.WithHeaders(headers1))
	require.NoError(t, err)

	signedToken1 := string(signedTokenBytes1)
	keyID, err := getKeyIDFromTokenString(signedToken1)
	require.NoError(t, err)

	require.Equal(t, headers1.KeyID(), keyID)

	// Test without KeyID
	token2 := jwt.New()
	token2.Set("foo", "bar")

	headers2 := jws.NewHeaders()

	signedTokenBytes2, err := jwt.Sign(token2, jwa.ES384, key, jwt.WithHeaders(headers2))
	require.NoError(t, err)

	signedToken2 := string(signedTokenBytes2)
	_, err = getKeyIDFromTokenString(signedToken2)
	require.Error(t, err)
	require.Equal(t, "token header does not contain key id (kid)", err.Error())

	// Test with non-token string
	_, err = getKeyIDFromTokenString("foo")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unable to parse tokenString")
}

func TestGetTokenTypeFromTokenString(t *testing.T) {
	key := testNewKey(t)

	// Test with Type
	token1 := jwt.New()
	token1.Set("foo", "bar")

	headers1 := jws.NewHeaders()
	headers1.Set(jws.TypeKey, "foo")

	signedTokenBytes1, err := jwt.Sign(token1, jwa.ES384, key, jwt.WithHeaders(headers1))
	require.NoError(t, err)

	signedToken1 := string(signedTokenBytes1)
	tokenType, err := getTokenTypeFromTokenString(signedToken1)
	require.NoError(t, err)

	require.Equal(t, headers1.Type(), tokenType)

	// Test without KeyID
	payload1 := `{"foo":"bar"}`

	signer1, err := jws.NewSigner(jwa.ES384)
	require.NoError(t, err)

	signedTokenBytes2, err := jws.SignMulti([]byte(payload1), jws.WithSigner(signer1, key, nil, nil))
	require.NoError(t, err)

	signedToken2 := string(signedTokenBytes2)
	_, err = getTokenTypeFromTokenString(signedToken2)
	require.Error(t, err)
	require.Equal(t, "token header does not contain type (typ)", err.Error())

	// Test with non-token string
	_, err = getTokenTypeFromTokenString("foo")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unable to parse tokenString")
}

func testNewKey(t *testing.T) jwk.Key {
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)

	key, err := jwk.New(ecdsaKey)
	require.NoError(t, err)

	return key
}
