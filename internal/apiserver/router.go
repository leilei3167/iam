// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package apiserver

import (
	"github.com/gin-gonic/gin"
	"github.com/marmotedu/component-base/pkg/core"

	"github.com/marmotedu/iam/pkg/errors"

	"github.com/marmotedu/iam/internal/apiserver/controller/v1/policy"
	"github.com/marmotedu/iam/internal/apiserver/controller/v1/secret"
	"github.com/marmotedu/iam/internal/apiserver/controller/v1/user"
	"github.com/marmotedu/iam/internal/apiserver/store/mysql"
	"github.com/marmotedu/iam/internal/pkg/code"
	"github.com/marmotedu/iam/internal/pkg/middleware"
	"github.com/marmotedu/iam/internal/pkg/middleware/auth"

	// custom gin validators.
	_ "github.com/marmotedu/iam/pkg/validator"
)

func initRouter(g *gin.Engine) {
	installMiddleware(g)
	installController(g)
}

func installMiddleware(g *gin.Engine) {
}

func installController(g *gin.Engine) *gin.Engine { // 注册处理器的关键点
	/*--------认证相关接口--------*/
	// Middlewares. 需要断言回jwt.GinJWTMiddleware的实例
	jwtStrategy, _ := newJWTAuth().(auth.JWTStrategy)
	g.POST("/login", jwtStrategy.LoginHandler)
	g.POST("/logout", jwtStrategy.LogoutHandler)
	// Refresh time can be longer than token timeout
	g.POST("/refresh", jwtStrategy.RefreshHandler)

	auto := newAutoAuth()
	g.NoRoute(auto.AuthFunc(), func(c *gin.Context) {
		core.WriteResponse(c, errors.WithCode(code.ErrPageNotFound, "Page not found."), nil)
	})

	// v1 handlers, requiring authentication
	storeIns, _ := mysql.GetMySQLFactoryOr(nil) // 注意:此处依赖注入 TODO:此处传入的nil???
	v1 := g.Group("/v1")
	{
		userv1 := v1.Group("/users")
		{ // user RESTful resource 用户相关接口
			userController := user.NewUserController(storeIns)

			userv1.POST("", userController.Create)               // 创建用户
			userv1.Use(auto.AuthFunc(), middleware.Validation()) // users相关的分组都必须经过认证
			// v1.PUT("/find_password", userController.FindPassword)
			userv1.DELETE("", userController.DeleteCollection)                 // admin api 删除操作,只有admin用户能够使用
			userv1.DELETE(":name", userController.Delete)                      // admin api
			userv1.PUT(":name/change-password", userController.ChangePassword) // 修改密码
			userv1.PUT(":name", userController.Update)                         // 修改用户属性
			userv1.GET("", userController.List)                                // 用户列表
			userv1.GET(":name", userController.Get)                            // admin api
		}

		v1.Use(auto.AuthFunc())

		// policy RESTful resource
		policyv1 := v1.Group("/policies", middleware.Publish())
		{ // 策略相关接口
			policyController := policy.NewPolicyController(storeIns)

			policyv1.POST("", policyController.Create)
			policyv1.DELETE("", policyController.DeleteCollection)
			policyv1.DELETE(":name", policyController.Delete)
			policyv1.PUT(":name", policyController.Update)
			policyv1.GET("", policyController.List)
			policyv1.GET(":name", policyController.Get)
		}

		// secret RESTful resource
		secretv1 := v1.Group("/secrets", middleware.Publish())
		{ // 密钥相关接口
			secretController := secret.NewSecretController(storeIns)

			secretv1.POST("", secretController.Create)
			secretv1.DELETE(":name", secretController.Delete)
			secretv1.PUT(":name", secretController.Update)
			secretv1.GET("", secretController.List)
			secretv1.GET(":name", secretController.Get)
		}
	}

	return g
}
