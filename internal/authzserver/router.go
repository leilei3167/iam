// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package authzserver

import (
	"github.com/gin-gonic/gin"
	"github.com/marmotedu/component-base/pkg/core"

	"github.com/marmotedu/iam/pkg/errors"

	"github.com/marmotedu/iam/internal/authzserver/controller/v1/authorize"
	"github.com/marmotedu/iam/internal/authzserver/load/cache"
	"github.com/marmotedu/iam/internal/pkg/code"
	"github.com/marmotedu/iam/pkg/log"
)

func initRouter(g *gin.Engine) {
	installMiddleware(g)
	installController(g)
}

func installMiddleware(g *gin.Engine) {
}

func installController(g *gin.Engine) *gin.Engine {
	// 设置认证的策略 Bearer认证(获取令牌解析来认证),访问authz的接口的唯一方式就是携带令牌(该令牌需要先在api中创建用户,创建用户后创建secret,使用该secret来签发令牌才合法)
	// 而认证的逻辑就是获取该令牌,取出其中的secretID,然后在缓存中查询对应的secret,验证该令牌是否合法,合法后再进行api调用
	auth := newCacheAuth()
	g.NoRoute(auth.AuthFunc(), func(c *gin.Context) {
		core.WriteResponse(c, errors.WithCode(code.ErrPageNotFound, "page not found."), nil)
	})

	cacheIns, _ := cache.GetCacheInsOr(nil)
	if cacheIns == nil {
		log.Panicf("get nil cache instance")
	}

	apiv1 := g.Group("/v1", auth.AuthFunc()) //从jwt中解析出kid,之后kid从缓存中查询密钥,验证通过后写入username
	{
		authzController := authorize.NewAuthzController(cacheIns) // cache实现了PolicyGetter接口,可从自己的内存缓存中读取policy

		// Router for authorization
		apiv1.POST("/authz", authzController.Authorize)
	}

	return g
}
