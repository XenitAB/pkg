package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/lestrrat-go/jwx/jws"
	"github.com/lestrrat-go/jwx/jwt"
)

type (
	// OIDCConfig defines the config for OIDC middleware.
	OIDCConfig struct {
		// Skipper defines a function to skip middleware.
		Skipper middleware.Skipper

		// BeforeFunc defines a function which is executed just before the middleware.
		BeforeFunc middleware.BeforeFunc

		// SuccessHandler defines a function which is executed for a valid token.
		SuccessHandler OIDCSuccessHandler

		// ErrorHandler defines a function which is executed for an invalid token.
		// It may be used to define a custom OIDC error.
		ErrorHandler OIDCErrorHandler

		// ErrorHandlerWithContext is almost identical to ErrorHandler, but it's passed the current context.
		ErrorHandlerWithContext OIDCErrorHandlerWithContext

		// Context key to store user information from the token into context.
		// Optional. Default value "user".
		ContextKey string

		// TokenLookup is a string in the form of "<source>:<name>" or "<source>:<name>,<source>:<name>" that is used
		// to extract token from the request.
		// Optional. Default value "header:Authorization".
		// Possible values:
		// - "header:<name>"
		// - "query:<name>"
		// - "param:<name>"
		// - "cookie:<name>"
		// - "form:<name>"
		// Multiply sources example:
		// - "header: Authorization,cookie: myowncookie"
		TokenLookup string

		// AuthScheme to be used in the Authorization header.
		// Optional. Default value "Bearer".
		AuthScheme string

		// Issuer is the authority that issues the tokens
		Issuer string

		// DiscoveryUri is where the `jwks_uri` will be grabbed
		// Defaults to `fmt.Sprintf("%s/.well-known/openid-configuration", strings.TrimSuffix(issuer, "/"))`
		DiscoveryUri string

		// JwksUri is used to download the public key(s)
		// Defaults to the `jwks_uri` from the response of DiscoveryUri
		JwksUri string

		// RequiredTokenType is used if only specific tokens should be allowed.
		// Default is empty string `""` and means all token types are allowed.
		// Use case could be to configure this if the TokenType (set in the header of the JWT)
		// should be `JWT` or maybe even `JWT+AT` to diffirentiate between access tokens and
		// id tokens. Not all providers support or use this.
		RequiredTokenType string

		// RequiredAudience is used to require a specific Audience `aud` in the claims.
		// Default to empty string `""` and means all audiences are allowed.
		RequiredAudience string

		// JwksFetchTimeout sets the context timeout when downloading the jwks
		// Defaults to 5 seconds
		JwksFetchTimeout time.Duration

		// AllowedTokenDrift adds the duration to the token expiration to allow
		// for time drift between parties.
		// Defaults to 10 seconds
		AllowedTokenDrift time.Duration

		// keyHandler handles jwks
		keyHandler *keyHandler
	}

	// OIDCSuccessHandler defines a function which is executed for a valid token.
	OIDCSuccessHandler func(echo.Context)

	// OIDCErrorHandler defines a function which is executed for an invalid token.
	OIDCErrorHandler func(error) error

	// OIDCErrorHandlerWithContext is almost identical to OIDCErrorHandler, but it's passed the current context.
	OIDCErrorHandlerWithContext func(error, echo.Context) error

	oidcExtractor func(echo.Context) (string, error)
)

// Errors
var (
	ErrJWTMissing = echo.NewHTTPError(http.StatusBadRequest, "missing or malformed jwt")
	ErrJWTInvalid = echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired jwt")
)

var (
	// DefaultOIDCConfig is the default OIDC auth middleware config.
	DefaultOIDCConfig = OIDCConfig{
		Skipper:     middleware.DefaultSkipper,
		ContextKey:  "user",
		TokenLookup: "header:" + echo.HeaderAuthorization,
		AuthScheme:  "Bearer",
	}
)

// OIDC returns a OpenID Connect (OIDC) auth middleware.
//
// For valid token, it sets the user in context and calls next handler.
// For invalid token, it returns "401 - Unauthorized" error.
// For missing token, it returns "400 - Bad Request" error.
//
// See: https://openid.net/connect/
// See `OIDCConfig.TokenLookup`
func OIDC(key interface{}) echo.MiddlewareFunc {
	c := DefaultOIDCConfig
	return OIDCWithConfig(c)
}

// OIDCWithConfig returns a OIDC auth middleware with config.
// See: `OIDC()`.
func OIDCWithConfig(config OIDCConfig) echo.MiddlewareFunc {
	// Defaults
	if config.Issuer == "" {
		panic("echo: oidc middleware requires Issuer")
	}
	if config.DiscoveryUri == "" {
		config.DiscoveryUri = getDiscoveryUriFromIssuer(config.Issuer)
	}
	if config.JwksUri == "" {
		jwksUri, err := getJwksUriFromDiscoveryUri(config.DiscoveryUri, 5*time.Second)
		if err != nil {
			panic(fmt.Sprintf("echo: oidc middleware unable to fetch JwksUri from DiscoveryUri (%s): %v", config.DiscoveryUri, err))
		}
		config.JwksUri = jwksUri
	}
	if config.JwksFetchTimeout == 0 {
		config.JwksFetchTimeout = 5 * time.Second
	}
	if config.AllowedTokenDrift == 0 {
		config.AllowedTokenDrift = 10 * time.Second
	}
	if config.Skipper == nil {
		config.Skipper = DefaultOIDCConfig.Skipper
	}
	if config.ContextKey == "" {
		config.ContextKey = DefaultOIDCConfig.ContextKey
	}
	if config.TokenLookup == "" {
		config.TokenLookup = DefaultOIDCConfig.TokenLookup
	}
	if config.AuthScheme == "" {
		config.AuthScheme = DefaultOIDCConfig.AuthScheme
	}

	// Initialize
	// KeyHandler
	keyHandler, err := newKeyHandler(config.JwksUri, config.JwksFetchTimeout)
	if err != nil {
		panic(fmt.Sprintf("echo: oidc middleware unable to initialize keyHandler: %v", err))
	}

	config.keyHandler = keyHandler

	// Split sources
	sources := strings.Split(config.TokenLookup, ",")
	var extractors []oidcExtractor
	for _, source := range sources {
		parts := strings.Split(source, ":")

		switch parts[0] {
		case "query":
			extractors = append(extractors, jwtFromQuery(parts[1]))
		case "param":
			extractors = append(extractors, jwtFromParam(parts[1]))
		case "cookie":
			extractors = append(extractors, jwtFromCookie(parts[1]))
		case "form":
			extractors = append(extractors, jwtFromForm(parts[1]))
		case "header":
			extractors = append(extractors, jwtFromHeader(parts[1], config.AuthScheme))
		}
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			if config.BeforeFunc != nil {
				config.BeforeFunc(c)
			}
			var auth string
			var err error
			for _, extractor := range extractors {
				// Extract token from extractor, if it's not fail break the loop and
				// set auth
				auth, err = extractor(c)
				if err == nil {
					break
				}
			}
			// If none of extractor has a token, handle error
			if err != nil {
				if config.ErrorHandler != nil {
					return config.ErrorHandler(err)
				}

				if config.ErrorHandlerWithContext != nil {
					return config.ErrorHandlerWithContext(err, c)
				}
				return err
			}

			token, err := config.parseToken(auth, c)
			if err == nil {
				// Store user information from token into context.
				c.Set(config.ContextKey, token)
				if config.SuccessHandler != nil {
					config.SuccessHandler(c)
				}
				return next(c)
			}
			if config.ErrorHandler != nil {
				return config.ErrorHandler(err)
			}
			if config.ErrorHandlerWithContext != nil {
				return config.ErrorHandlerWithContext(err, c)
			}
			return &echo.HTTPError{
				Code:     ErrJWTInvalid.Code,
				Message:  ErrJWTInvalid.Message,
				Internal: err,
			}
		}
	}
}

func (config *OIDCConfig) parseToken(auth string, c echo.Context) (jwt.Token, error) {
	keyID, err := getKeyIDFromTokenString(auth)
	if err != nil {
		return nil, err
	}

	if config.RequiredTokenType != "" {
		tokenType, err := getTokenTypeFromTokenString(auth)
		if err != nil {
			return nil, err
		}

		if tokenType != config.RequiredTokenType {
			return nil, fmt.Errorf("token type %q required, but received: %s", config.RequiredTokenType, tokenType)
		}
	}

	key, err := config.keyHandler.getByKeyID(keyID, false)
	if err != nil {
		return nil, err
	}

	keySet := jwk.NewSet()
	keySet.Add(key)

	token, err := jwt.ParseString(auth, jwt.WithKeySet(keySet))
	if err != nil {
		return nil, err
	}

	tokenExpired := token.Expiration().Round(0).Add(-config.AllowedTokenDrift).Before(time.Now())

	if tokenExpired {
		return nil, fmt.Errorf("token has expired: %s", token.Expiration())
	}

	if config.Issuer != token.Issuer() {
		return nil, fmt.Errorf("required issuer %q was not found, received: %s", config.Issuer, token.Issuer())
	}

	if config.RequiredAudience != "" {
		audiences := token.Audience()
		audienceFound := false
		for _, audience := range audiences {
			if audience == config.RequiredAudience {
				audienceFound = true
			}
		}

		if !audienceFound {
			return nil, fmt.Errorf("required audience %q was not found, received: %v", config.RequiredAudience, audiences)
		}
	}

	return token, nil
}

// jwtFromHeader returns a `oidcExtractor` that extracts token from the request header.
func jwtFromHeader(header string, authScheme string) oidcExtractor {
	return func(c echo.Context) (string, error) {
		auth := c.Request().Header.Get(header)
		l := len(authScheme)
		if len(auth) > l+1 && auth[:l] == authScheme {
			return auth[l+1:], nil
		}
		return "", ErrJWTMissing
	}
}

// jwtFromQuery returns a `oidcExtractor` that extracts token from the query string.
func jwtFromQuery(param string) oidcExtractor {
	return func(c echo.Context) (string, error) {
		token := c.QueryParam(param)
		if token == "" {
			return "", ErrJWTMissing
		}
		return token, nil
	}
}

// jwtFromParam returns a `oidcExtractor` that extracts token from the url param string.
func jwtFromParam(param string) oidcExtractor {
	return func(c echo.Context) (string, error) {
		token := c.Param(param)
		if token == "" {
			return "", ErrJWTMissing
		}
		return token, nil
	}
}

// jwtFromCookie returns a `oidcExtractor` that extracts token from the named cookie.
func jwtFromCookie(name string) oidcExtractor {
	return func(c echo.Context) (string, error) {
		cookie, err := c.Cookie(name)
		if err != nil {
			return "", ErrJWTMissing
		}
		return cookie.Value, nil
	}
}

// jwtFromForm returns a `oidcExtractor` that extracts token from the form field.
func jwtFromForm(name string) oidcExtractor {
	return func(c echo.Context) (string, error) {
		field := c.FormValue(name)
		if field == "" {
			return "", ErrJWTMissing
		}
		return field, nil
	}
}

type keyHandler struct {
	sync.RWMutex
	jwksURI      string
	keySet       jwk.Set
	fetchTimeout time.Duration
}

func newKeyHandler(jwksUri string, fetchTimeout time.Duration) (*keyHandler, error) {
	h := &keyHandler{
		jwksURI:      jwksUri,
		fetchTimeout: fetchTimeout,
	}

	err := h.updateKeySet()
	if err != nil {
		return nil, err
	}

	return h, nil
}

func (h *keyHandler) updateKeySet() error {
	ctx, cancel := context.WithTimeout(context.Background(), h.fetchTimeout)
	defer cancel()
	keySet, err := jwk.Fetch(ctx, h.jwksURI)
	if err != nil {
		return fmt.Errorf("Unable to fetch keys from %q: %v", h.jwksURI, err)
	}

	h.Lock()
	h.keySet = keySet
	h.Unlock()

	return nil
}

func (h *keyHandler) getKeySet() jwk.Set {
	h.RLock()
	defer h.RUnlock()
	return h.keySet
}

func (h *keyHandler) getByKeyID(keyID string, retry bool) (jwk.Key, error) {
	keySet := h.getKeySet()
	key, found := keySet.LookupKeyID(keyID)

	if !found && !retry {
		err := h.updateKeySet()
		if err != nil {
			return nil, fmt.Errorf("unable to update key set for key %q: %v", keyID, err)
		}

		return h.getByKeyID(keyID, true)
	}

	if !found && retry {
		return nil, fmt.Errorf("unable to find key %q", keyID)
	}

	return key, nil
}

func getDiscoveryUriFromIssuer(issuer string) string {
	return fmt.Sprintf("%s/.well-known/openid-configuration", strings.TrimSuffix(issuer, "/"))
}

func getJwksUriFromDiscoveryUri(discoveryUri string, fetchTimeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryUri, nil)
	if err != nil {
		return "", err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	err = res.Body.Close()
	if err != nil {
		return "", err
	}

	var discoveryData struct {
		JwksUri string `json:"jwks_uri"`
	}

	err = json.Unmarshal(bodyBytes, &discoveryData)
	if err != nil {
		return "", err
	}

	if discoveryData.JwksUri == "" {
		return "", fmt.Errorf("JwksURI is empty")
	}

	return discoveryData.JwksUri, nil
}

func getKeyIDFromTokenString(tokenString string) (string, error) {
	headers, err := getHeadersFromTokenString(tokenString)
	if err != nil {
		return "", err
	}

	keyID := headers.KeyID()
	if keyID == "" {
		return "", fmt.Errorf("token header does not contain key id (kid)")
	}

	return keyID, nil
}

func getTokenTypeFromTokenString(tokenString string) (string, error) {
	headers, err := getHeadersFromTokenString(tokenString)
	if err != nil {
		return "", err
	}

	tokenType := headers.Type()
	if tokenType == "" {
		return "", fmt.Errorf("token header does not contain type (typ)")
	}

	return tokenType, nil
}

func getHeadersFromTokenString(tokenString string) (jws.Headers, error) {
	msg, err := jws.ParseString(tokenString)
	if err != nil {
		return nil, err
	}

	signatures := msg.Signatures()
	if len(signatures) != 1 {
		return nil, fmt.Errorf("more than one signature in token")
	}

	headers := signatures[0].ProtectedHeaders()
	if headers == nil {
		return nil, fmt.Errorf("token headers nil")
	}

	return headers, nil
}
