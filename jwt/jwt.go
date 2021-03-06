package jwt

import (
	"errors"
	"net/textproto"
	"time"

	josecrypto "github.com/SermoDigital/jose/crypto"
	josejws "github.com/SermoDigital/jose/jws"
	josejwt "github.com/SermoDigital/jose/jwt"
)

// KeyPair represents key struct for ECDSA, RS/PS SigningMethod.
type KeyPair struct {
	PrivateKey interface{}
	PublicKey  interface{}
}

// JWT represents a module. it can be use to create, decode or verify JWT token.
type JWT struct {
	keys         rotating
	expiresIn    time.Duration
	issuer       string
	audience     []string
	method       josecrypto.SigningMethod
	validator    []*josejwt.Validator
	backupKeys   rotating
	backupMethod josecrypto.SigningMethod
}

// New returns a JWT instance.
// if key omit, jwt will use crypto.Unsecured as signing method.
// Otherwise crypto.SigningMethodHS256 will be used. You can change it by jwt.SetMethods.
func New(keys ...interface{}) *JWT {
	j := &JWT{method: josecrypto.Unsecured}
	j.keys = keys
	if len(keys) == 0 {
		j.keys = []interface{}{nil}
	} else {
		j.method = josecrypto.SigningMethodHS256
	}
	return j
}

// Sign creates a JWT token with the given content and optional expiresIn.
//
//  token1, err1 := jwt.Sign(map[string]interface{}{"UserId": "xxxxx"})
//  // or
//  claims := josejwt.Claims{} // or claims := josejws.Claims{}
//  claims.Set("hello", "world")
//  token2, err2 := jwt.Sign(claims)
//
// if expiresIn <= 0, expiration will not be set to claims:
//
//  token1, err1 := jwt.Sign(map[string]interface{}{"UserId": "xxxxx"}, time.Duration(0))
//
func (j *JWT) Sign(content map[string]interface{}, expiresIn ...time.Duration) (string, error) {
	claims := josejwt.Claims(content)
	if j.issuer != "" {
		claims.SetIssuer(j.issuer)
	}
	if len(j.audience) > 0 {
		claims.SetAudience(j.audience...)
	}
	if len(expiresIn) > 0 {
		if expiresIn[0] > 0 {
			claims.SetExpiration(time.Now().Add(expiresIn[0]))
		}
	} else if j.expiresIn > 0 {
		claims.SetExpiration(time.Now().Add(j.expiresIn))
	}

	var key interface{} = j.keys[0]
	return Sign(claims, j.method, key)
}

// Decode parse a string token, but don't validate it.
func (j *JWT) Decode(token string) (josejwt.Claims, error) {
	return Decode(token)
}

// Verify parse a string token and validate it with keys, signingMethods and validator in rotationally.
func (j *JWT) Verify(token string) (claims josejwt.Claims, err error) {
	jwtToken, err := josejws.ParseJWT([]byte(token))

	if err == nil {
		claims, err = Verify(jwtToken, j.method, j.keys, j.validator...)
		if err != nil && j.backupKeys != nil {
			claims, err = Verify(jwtToken, j.backupMethod, j.backupKeys, j.validator...)
		}
		if err == nil {
			return claims, nil
		}
	}

	return nil, &textproto.Error{Code: 401, Msg: err.Error()}
}

// SetIssuer set a issuer to jwt.
// Default to "", no "iss" will be added.
func (j *JWT) SetIssuer(issuer string) {
	j.issuer = issuer
}

// SetAudience sets claim "aud" per its type in
// https://tools.ietf.org/html/rfc7519#section-4.1.3
func (j *JWT) SetAudience(audience ...string) {
	j.audience = audience
}

// GetExpiresIn returns jwt's expiration.
func (j *JWT) GetExpiresIn() time.Duration {
	return j.expiresIn
}

// SetExpiresIn set a expire duration to jwt.
// Default to 0, no "exp" will be added.
func (j *JWT) SetExpiresIn(expiresIn time.Duration) {
	j.expiresIn = expiresIn
}

// SetKeys set new keys to jwt.
// [deprecated] Please use SetSigning method.
func (j *JWT) SetKeys(keys ...interface{}) {
	if len(keys) == 0 || keys[0] == nil {
		panic(errors.New("invalid keys"))
	}
	j.keys = keys
}

// SetMethods set one or more signing methods which can be used rotational.
// [deprecated] Please use SetSigning method.
func (j *JWT) SetMethods(method josecrypto.SigningMethod) {
	if method == nil {
		panic(errors.New("invalid signing method"))
	}
	j.method = method
}

// SetValidator set a custom jwt.Validator to jwt. Default to nil.
func (j *JWT) SetValidator(validator *josejwt.Validator) {
	if validator == nil {
		panic(errors.New("invalid validator"))
	}
	j.validator = []*josejwt.Validator{validator}
}

// SetSigning add signing method and keys.
func (j *JWT) SetSigning(method josecrypto.SigningMethod, keys ...interface{}) {
	if len(keys) == 0 || keys[0] == nil {
		panic(errors.New("invalid keys"))
	}
	if method == nil {
		panic(errors.New("invalid signing method"))
	}
	j.method = method
	j.keys = keys
}

// SetBackupSigning add a backup signing for Verify method, not for Sign method.
func (j *JWT) SetBackupSigning(method josecrypto.SigningMethod, keys ...interface{}) {
	if len(keys) == 0 || keys[0] == nil {
		panic(errors.New("invalid keys"))
	}
	if method == nil {
		panic(errors.New("invalid signing method"))
	}
	j.backupMethod = method
	j.backupKeys = keys
}

// Sign creates a JWT token with the given claims, signing method and key.
func Sign(claims josejwt.Claims, method josecrypto.SigningMethod, key interface{}) (string, error) {
	if k, ok := key.(KeyPair); ok { // try to extract PrivateKey
		key = k.PrivateKey
	}
	if !claims.Has("iat") {
		claims.Set("iat", time.Now().Unix())
	}
	buf, err := josejws.NewJWT(josejws.Claims(claims), method).Serialize(key)
	if err == nil {
		return string(buf), nil
	}
	return "", err
}

// Decode parse a string token, but don't validate it.
func Decode(token string) (josejwt.Claims, error) {
	jwtToken, err := josejws.ParseJWT([]byte(token))
	if err == nil {
		return jwtToken.Claims(), nil
	}
	return nil, err
}

// Verify parse a string token and validate it with keys, signingMethods in rotationally.
func Verify(token josejwt.JWT, method josecrypto.SigningMethod, keys []interface{}, v ...*josejwt.Validator) (claims josejwt.Claims, err error) {
	if rotating(keys).Verify(func(key interface{}) bool {
		if k, ok := key.(KeyPair); ok { // try to extract PublicKey
			key = k.PublicKey
		}
		if err = token.Validate(key, method, v...); err == nil {
			claims = token.Claims()
			return true
		}
		return false
	}) >= 0 {
		return claims, nil
	}
	return nil, err
}

type rotating []interface{}

func (r rotating) Verify(v func(interface{}) bool) (index int) {
	for i, key := range r { // key rotation
		if v(key) {
			return i
		}
	}
	return -1
}

// StrToKeys converts string slice to keys slice.
func StrToKeys(keys ...string) (res []interface{}) {
	for _, key := range keys {
		res = append(res, []byte(key))
	}
	return
}
