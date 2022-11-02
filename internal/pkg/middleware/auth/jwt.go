// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package auth

import (
	ginjwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"

	"github.com/marmotedu/iam/internal/pkg/middleware"
)

// AuthzAudience defines the value of jwt audience field.
const AuthzAudience = "iam.authz.marmotedu.com"

// JWTStrategy defines jwt bearer authentication strategy.
type JWTStrategy struct { //JWT认证策略
	ginjwt.GinJWTMiddleware
}

var _ middleware.AuthStrategy = &JWTStrategy{}

// NewJWTStrategy create jwt bearer strategy with GinJWTMiddleware.
func NewJWTStrategy(gjwt ginjwt.GinJWTMiddleware) JWTStrategy {
	return JWTStrategy{gjwt}
}

// AuthFunc defines jwt bearer strategy as the gin authentication middleware.
func (j JWTStrategy) AuthFunc() gin.HandlerFunc {
	return j.MiddlewareFunc() //如果Header中有"Bearer"字段,则会调用此处,用于apiserver的Bearer认证
}
