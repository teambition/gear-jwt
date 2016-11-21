package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/SermoDigital/jose/crypto"
	"github.com/SermoDigital/jose/jws"
	"github.com/SermoDigital/jose/jwt"
	"github.com/teambition/gear"
)

// TokenExtractor is a function that takes a gear.Context as input and
// returns either a string token or an empty string. Default to:
//
//  func(ctx *gear.Context) (token string) {
//  	if auth := ctx.Get("Authorization"); strings.HasPrefix(auth, "BEARER ") {
//  		token = auth[7:]
//  	} else {
//  		token = ctx.Param("access_token")
//  	}
//  	return
//  }
//
type TokenExtractor func(ctx *gear.Context) (token string)

// KeyPair represents key struct for ECDSA, RS/PS SigningMethod.
type KeyPair struct {
	PrivateKey interface{}
	PublicKey  interface{}
}

// JWT represents a module. it can be use to create, decode or verify JWT token.
// It can be used as gear middleware to authenticate client too.
type JWT struct {
	keys       []interface{}
	expiration time.Duration

	issuer    string
	methods   []crypto.SigningMethod
	validator []*jwt.Validator
	extractor TokenExtractor
}

// NewJWT returns a JWT instance, jwter.
// if key omit, jwter will use crypto.Unsecured as signing method.
// Otherwise crypto.SigningMethodHS256 will be used. You can change it by jwter.SetMethods.
func NewJWT(keys ...interface{}) *JWT {
	j := &JWT{methods: []crypto.SigningMethod{crypto.Unsecured}}
	j.keys = keys
	if len(keys) == 0 {
		j.keys = []interface{}{nil}
	} else {
		j.methods[0] = crypto.SigningMethodHS256
	}
	j.extractor = func(ctx *gear.Context) (token string) {
		if auth := ctx.Get("Authorization"); strings.HasPrefix(auth, "BEARER ") {
			token = auth[7:]
		} else {
			token = ctx.Query("access_token")
		}
		return
	}
	return j
}

// Sign creates a JWT token with the given content and optional signing method.
//
//  token1, err1 := jwter.Sign(map[string]interface{}{"UserId": "xxxxx"})
//  // or
//  claims := jwt.Claims{} // or claims := jws.Claims{}
//  claims.Set("hello", "world")
//  token2, err2 := jwter.Sign(claims)
//
func (j *JWT) Sign(content map[string]interface{}, method ...crypto.SigningMethod) (string, error) {
	claims := jws.Claims(content)
	if j.issuer != "" {
		claims.SetIssuer(j.issuer)
	}
	if j.expiration > 0 {
		claims.SetExpiration(time.Now().Add(j.expiration))
	}
	if len(method) == 0 {
		method = j.methods
	}

	var key interface{} = j.keys[0]
	if k, ok := key.(KeyPair); ok { // try to extract PrivateKey
		key = k.PrivateKey
	}
	buf, err := jws.NewJWT(claims, method[0]).Serialize(key)
	if err == nil {
		return string(buf), nil
	}
	return "", err
}

// Decode parse a string token, but don't validate it.
func (j *JWT) Decode(token string) (jwt.Claims, error) {
	jwtToken, err := jws.ParseJWT([]byte(token))
	if err == nil {
		return jwtToken.Claims(), nil
	}
	return nil, &gear.Error{Code: 401, Msg: err.Error()}
}

// Verify parse a string token and validate it with keys, signingMethods and validator in rotationally.
func (j *JWT) Verify(token string) (jwt.Claims, error) {
	jwtToken, err := jws.ParseJWT([]byte(token))
	if err == nil {
		for _, key := range j.keys { // key rotation
			if k, ok := key.(KeyPair); ok { // try to extract PublicKey
				key = k.PublicKey
			}
			for _, method := range j.methods { // method rotation
				if err = jwtToken.Validate(key, method, j.validator...); err == nil {
					return jwtToken.Claims(), nil
				}
			}
		}
	}
	return nil, &gear.Error{Code: 401, Msg: err.Error()}
}

// SetIssuer set a issuer to jwter.
// Default to "", no "iss" will be added.
func (j *JWT) SetIssuer(issuer string) {
	j.issuer = issuer
}

// SetExpiration set a expiration time duration to jwter.
// Default to 0, no "exp" will be added.
func (j *JWT) SetExpiration(expiration time.Duration) {
	j.expiration = expiration
}

// SetMethods set one or more signing methods which can be used rotational.
func (j *JWT) SetMethods(methods ...crypto.SigningMethod) {
	if len(methods) == 0 {
		panic(errors.New("Invalid signing method"))
	}
	j.methods = methods
}

// SetValidator set a custom jwt.Validator to jwter. Default to nil.
func (j *JWT) SetValidator(validator *jwt.Validator) {
	if validator == nil {
		panic(errors.New("Invalid validator"))
	}
	j.validator = []*jwt.Validator{validator}
}

// SetTokenParser set a custom tokenExtractor to jwter. Default to:
//
//  func(ctx *gear.Context) (token string) {
//  	if auth := ctx.Get("Authorization"); strings.HasPrefix(auth, "BEARER ") {
//  		token = auth[7:]
//  	} else {
//  		token = ctx.Query("access_token")
//  	}
//  	return
//  }
//
func (j *JWT) SetTokenParser(extractor TokenExtractor) {
	j.extractor = extractor
}

// New implements gear.Any interface, then we can use it with ctx.Any:
//
//  any, err := ctx.Any(jwter)
//  if err != nil {
//  	return err
//  }
//  claims := any.(jwt.Claims)
//
// that is jwter.FromCtx doing for us.
//
func (j *JWT) New(ctx *gear.Context) (interface{}, error) {
	if token := j.extractor(ctx); token != "" {
		return j.Verify(token)
	}
	return nil, &gear.Error{Code: 401, Msg: "No token found"}
}

// FromCtx will parse and validate token from the ctx, and return it as jwt.Claims.
// If token not exists or validate failure, a empty jwt.Claims instance returned.
//
//  claims := jwter.FromCtx(ctx)
//  fmt.Println(claims)
//
func (j *JWT) FromCtx(ctx *gear.Context) jwt.Claims {
	any, err := ctx.Any(j)
	if err == nil {
		return any.(jwt.Claims)
	}
	claims := jwt.Claims{}
	ctx.SetAny(j, claims)
	return claims
}

// Serve implements gear.Handler interface. We can use it as middleware.
// It will parse and validate token from the ctx, if succeed, gear's middleware process
// will go on, otherwise process ended and a 401 error will be to respond to client.
//
//  app := gear.New()
//  jwter := auth.New()
//  app.UseHandler(jwter)
//  // or
//  app.Use(jwter.Serve)
//
func (j *JWT) Serve(ctx *gear.Context) error {
	claims, err := j.New(ctx)
	if err == nil {
		ctx.SetAny(j, claims)
	}
	return err
}
