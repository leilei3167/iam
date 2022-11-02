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
	//此处不同的API可以灵活选择不同的认证方式,如/Login /logout等都采用JWT认证
	//而其余部分使用自动策略(根据Header不同使用不同的策略),策略模式的运用

	// Middlewares. 创建JWT认证策略(接口),断言回具体的实例(jwt.GinJWTMiddleware)
	//先使用newJWTAuth 初始化GinJWTMiddleware
	//TODO:模仿学习JWT策略用法
	jwtStrategy, _ := newJWTAuth().(auth.JWTStrategy)
	g.POST("/login", jwtStrategy.LoginHandler)
	g.POST("/logout", jwtStrategy.LogoutHandler)
	// Refresh time can be longer than token timeout
	g.POST("/refresh", jwtStrategy.RefreshHandler)

	auto := newAutoAuth() //自动认证方式,会根据Header自动从basic和Bearer认证中选择(api的Bearer和authz的是不一样的)
	//api的Bearer认证流程: 由autoAuth的方法判断http.Header中是否有Bearer字段->调用jwt.AuthFunc->GinJWTMiddleware.Middleware方法->mw.middlewareImpl(c)
	g.NoRoute(auto.AuthFunc(), func(c *gin.Context) {
		core.WriteResponse(c, errors.WithCode(code.ErrPageNotFound, "Page not found."), nil)
	})

	// v1 handlers, requiring authentication
	storeIns, _ := mysql.GetMySQLFactoryOr(nil) //仓库层于gRPC服务初始化时已经被注入
	v1 := g.Group("/v1")
	{
		userv1 := v1.Group("/users")
		{ // user RESTful resource 用户相关接口
			userController := user.NewUserController(storeIns)

			userv1.POST("", userController.Create) // 创建用户(注册),不需要认证
			//访问/v1/users,需要认证,以及admin权限(对用户的删改查)
			userv1.Use(auto.AuthFunc(), middleware.Validation()) // users必须先经过认证,之后还必须经过判断是否是管理员
			// v1.PUT("/find_password", userController.FindPassword)
			userv1.DELETE("", userController.DeleteCollection)                 // admin api 删除操作,只有admin用户能够使用
			userv1.DELETE(":name", userController.Delete)                      // admin api
			userv1.PUT(":name/change-password", userController.ChangePassword) // 修改密码
			userv1.PUT(":name", userController.Update)                         // 修改用户属性
			userv1.GET("", userController.List)                                // 用户列表
			userv1.GET(":name", userController.Get)                            // admin api
		}

		//其余v1分组均需要认证才能访问
		v1.Use(auto.AuthFunc()) //TODO:v1.Use为何要在此处?

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
