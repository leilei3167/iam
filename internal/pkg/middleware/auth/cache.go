// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	jwt "github.com/golang-jwt/jwt/v4"

	"github.com/marmotedu/component-base/pkg/core"

	"github.com/marmotedu/iam/pkg/errors"

	"github.com/marmotedu/iam/internal/pkg/code"
	"github.com/marmotedu/iam/internal/pkg/middleware"
)

// Defined errors.
var (
	ErrMissingKID    = errors.New("Invalid token format: missing kid field in claims")
	ErrMissingSecret = errors.New("Can not obtain secret information from cache")
)

// Secret contains the basic information of the secret key.
type Secret struct {
	Username string
	ID       string
	Key      string
	Expires  int64
}

// CacheStrategy defines jwt bearer authentication strategy which called `cache strategy`.
// Secrets are obtained through grpc api interface and cached in memory.
type CacheStrategy struct {
	get func(kid string) (Secret, error)
}

var _ middleware.AuthStrategy = &CacheStrategy{}

// NewCacheStrategy create cache strategy with function which can list and cache secrets.
func NewCacheStrategy(get func(kid string) (Secret, error)) CacheStrategy {
	return CacheStrategy{get}
}

// AuthFunc defines cache strategy as the gin authentication middleware.
func (cache CacheStrategy) AuthFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.Request.Header.Get("Authorization")
		if len(header) == 0 {
			core.WriteResponse(c, errors.WithCode(code.ErrMissingHeader, "Authorization header cannot be empty."), nil)
			c.Abort()

			return
		}

		var rawJWT string
		// Parse the header to get the token part.
		fmt.Sscanf(header, "Bearer %s", &rawJWT) // 扫描Header字符串,填充占位符,并占位符处赋值给rawJWT

		// Use own validation logic, see below
		var secret Secret

		claims := &jwt.MapClaims{}
		// Verify the token,验证该JWT Token
		parsedT, err := jwt.ParseWithClaims(rawJWT, claims, func(token *jwt.Token) (interface{}, error) { //keyFunc是指明,如何获取密钥(此处从缓存中查询)
			// Validate the alg is HMAC signature
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}

			kid, ok := token.Header["kid"].(string) //解析出kid,kid 即为密钥 ID：secretID
			if !ok {
				return nil, ErrMissingKID
			}

			var err error
			secret, err = cache.get(kid) //从缓存中根据secretID 查询对应的密钥信息
			if err != nil {
				return nil, ErrMissingSecret
			}

			return []byte(secret.Key), nil //获取到secretID对应的密钥后,对JWT的第三段进行签名验证
		})
		if err != nil || !parsedT.Valid {
			core.WriteResponse(c, errors.WithCode(code.ErrSignatureInvalid, err.Error()), nil)
			c.Abort()

			return
		}
		//验证通过后 检查是否过期
		if KeyExpired(secret.Expires) {
			tm := time.Unix(secret.Expires, 0).Format("2006-01-02 15:04:05")
			core.WriteResponse(c, errors.WithCode(code.ErrExpired, "expired at: %s", tm), nil)
			c.Abort()

			return
		}

		//未过期 设置Header 增加username: colin
		c.Set(middleware.UsernameKey, secret.Username)
		c.Next()
	}
}

// KeyExpired checks if a key has expired, if the value of user.SessionState.Expires is 0, it will be ignored.
func KeyExpired(expires int64) bool {
	if expires >= 1 {
		return time.Now().After(time.Unix(expires, 0))
	}

	return false
}
