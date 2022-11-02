// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package auth

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/marmotedu/component-base/pkg/core"

	"github.com/marmotedu/iam/pkg/errors"

	"github.com/marmotedu/iam/internal/pkg/code"
	"github.com/marmotedu/iam/internal/pkg/middleware"
)

const authHeaderCount = 2

// AutoStrategy defines authentication strategy which can automatically choose between Basic and Bearer
// according `Authorization` header.
type AutoStrategy struct { //自动策略,根据Header自动选择认证的策略
	basic middleware.AuthStrategy
	jwt   middleware.AuthStrategy
}

var _ middleware.AuthStrategy = &AutoStrategy{}

// NewAutoStrategy create auto strategy with basic strategy and jwt strategy.
func NewAutoStrategy(basic, jwt middleware.AuthStrategy) AutoStrategy { //传入的是接口,而不是具体的策略实现的实例,因此basic和jwt的策略可以有多种实现(方便拓展)
	return AutoStrategy{
		basic: basic,
		jwt:   jwt,
	}
}

// AuthFunc defines auto strategy as the gin authentication middleware.
func (a AutoStrategy) AuthFunc() gin.HandlerFunc { //实现策略接口
	return func(c *gin.Context) {
		operator := middleware.AuthOperator{} //创建策略执行
		authHeader := strings.SplitN(c.Request.Header.Get("Authorization"), " ", 2)

		if len(authHeader) != authHeaderCount {
			core.WriteResponse(
				c,
				errors.WithCode(code.ErrInvalidAuthHeader, "Authorization header format is wrong."),
				nil,
			)
			c.Abort()

			return
		}
		// 选择所需的策略
		switch authHeader[0] { //根据情况,判断所需要的策略
		case "Basic":
			operator.SetStrategy(a.basic)
		case "Bearer":
			operator.SetStrategy(a.jwt)
			// a.JWT.MiddlewareFunc()(c)
		default:
			core.WriteResponse(c, errors.WithCode(code.ErrSignatureInvalid, "unrecognized Authorization header."), nil)
			c.Abort()

			return
		}
		//执行设置的策略,调用策略的逻辑(传入context),Basic或Bearer
		operator.AuthFunc()(c)

		c.Next()
	}
}
