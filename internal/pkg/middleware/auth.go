// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package middleware

import (
	"github.com/gin-gonic/gin"
)

// AuthStrategy defines the set of methods used to do resource authentication.
type AuthStrategy interface { //策略模式,定义认证的策略接口
	AuthFunc() gin.HandlerFunc //每一种认证策略都以中间件的形式存在
}

// AuthOperator used to switch between different authentication strategy.
type AuthOperator struct { // operator继承策略接口,将其作为为策略的执行者
	strategy AuthStrategy
}

// SetStrategy used to set to another authentication strategy.
func (operator *AuthOperator) SetStrategy(strategy AuthStrategy) {
	operator.strategy = strategy
}

// AuthFunc execute resource authentication.
func (operator *AuthOperator) AuthFunc() gin.HandlerFunc {
	return operator.strategy.AuthFunc()
}
