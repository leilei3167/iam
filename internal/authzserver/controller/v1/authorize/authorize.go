// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

// Package authorize implements the authorize handlers.
package authorize

import (
	"github.com/gin-gonic/gin"
	"github.com/marmotedu/component-base/pkg/core"
	"github.com/ory/ladon"

	"github.com/marmotedu/iam/pkg/errors"

	"github.com/marmotedu/iam/internal/authzserver/authorization"
	"github.com/marmotedu/iam/internal/authzserver/authorization/authorizer"
	"github.com/marmotedu/iam/internal/pkg/code"
)

// AuthzController create a authorize handler used to handle authorize request.
type AuthzController struct {
	store authorizer.PolicyGetter
}

// NewAuthzController creates a authorize handler.
func NewAuthzController(store authorizer.PolicyGetter) *AuthzController {
	return &AuthzController{
		store: store,
	}
}

// Authorize returns whether a request is allow or deny to access a resource and do some action
// under specified condition.
func (a *AuthzController) Authorize(c *gin.Context) { // 控制层
	var r ladon.Request
	if err := c.ShouldBind(&r); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, err.Error()), nil)

		return
	}

	auth := authorization.NewAuthorizer(authorizer.NewAuthorization(a.store)) // 使用PolicyGetter(此处是全局cache)
	if r.Context == nil {
		r.Context = ladon.Context{}
	}

	r.Context["username"] = c.GetString("username")
	rsp := auth.Authorize(&r) // 调用此处进行鉴权,会调用Manager的FindRequestCandidates方法->最终调用Cache的GetPolicy方法

	core.WriteResponse(c, nil, rsp)
}
