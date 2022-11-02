// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package apiserver

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	v1 "github.com/marmotedu/api/apiserver/v1"
	metav1 "github.com/marmotedu/component-base/pkg/meta/v1"
	"github.com/spf13/viper"

	"github.com/marmotedu/iam/internal/apiserver/store"
	"github.com/marmotedu/iam/internal/pkg/middleware"
	"github.com/marmotedu/iam/internal/pkg/middleware/auth"
	"github.com/marmotedu/iam/pkg/log"
)

const (
	// APIServerAudience defines the value of jwt audience field.
	APIServerAudience = "iam.api.marmotedu.com"

	// APIServerIssuer defines the value of jwt issuer field.
	APIServerIssuer = "iam-apiserver"
)

type loginInfo struct {
	Username string `form:"username" json:"username" binding:"required,username"`
	Password string `form:"password" json:"password" binding:"required,password"`
}

func newBasicAuth() middleware.AuthStrategy { //初始化基本认证的策略实现的实例
	return auth.NewBasicStrategy(func(username string, password string) bool { //定义basic认证的密码校验方式
		// fetch user from database
		user, err := store.Client().Users().Get(context.TODO(), username, metav1.GetOptions{}) //仓库层提供一个Client方法,使得其他地方能够访问数据库(仅限于中间件)
		if err != nil {
			return false
		}

		// Compare the login password with the user password.
		if err := user.Compare(password); err != nil {
			return false
		}

		user.LoginedAt = time.Now()
		_ = store.Client().Users().Update(context.TODO(), user, metav1.UpdateOptions{}) //更新登录时间

		return true
	})
}

func newJWTAuth() middleware.AuthStrategy { //初始化JWT认证策略的实例
	ginjwt, _ := jwt.New(&jwt.GinJWTMiddleware{
		Realm:            viper.GetString("jwt.Realm"), //直接从viper中读取
		SigningAlgorithm: "HS256",
		Key:              []byte(viper.GetString("jwt.key")), //签发api的登录状态的jwt
		Timeout:          viper.GetDuration("jwt.timeout"),
		MaxRefresh:       viper.GetDuration("jwt.max-refresh"),
		Authenticator:    authenticator(), //登录时调用
		LoginResponse:    loginResponse(),
		LogoutResponse: func(c *gin.Context, code int) {
			c.JSON(http.StatusOK, nil)
		},
		RefreshResponse: refreshResponse(),
		PayloadFunc:     payloadFunc(),
		IdentityHandler: func(c *gin.Context) interface{} { //在gin.Context中添加的一个键,设置为username
			claims := jwt.ExtractClaims(c) //上述函数会从 Token 的 Payload 中获取 identity 域的值，identity 域的值是在签发 Token 时指定的，它的值其实是用户名，你可以查看payloadFunc函数了解。

			return claims[jwt.IdentityKey]
		},
		IdentityKey:  middleware.UsernameKey,
		Authorizator: authorizator(), //验证的方法
		Unauthorized: func(c *gin.Context, code int, message string) {
			c.JSON(code, gin.H{
				"message": message,
			})
		},
		TokenLookup:   "header: Authorization, query: token, cookie: jwt",
		TokenHeadName: "Bearer",
		SendCookie:    true,
		TimeFunc:      time.Now,
		// TODO: HTTPStatusMessageFunc:
	})

	return auth.NewJWTStrategy(*ginjwt)
}

func newAutoAuth() middleware.AuthStrategy { //返回的都是接口,调用者不用关心底层实现,只需调用其AuthFunc方法即可
	return auth.NewAutoStrategy(newBasicAuth().(auth.BasicStrategy), newJWTAuth().(auth.JWTStrategy))
}

func authenticator() func(c *gin.Context) (interface{}, error) {
	return func(c *gin.Context) (interface{}, error) {
		var login loginInfo
		var err error

		// support header and body both,支持从Header或Body中获取账户和密码
		if c.Request.Header.Get("Authorization") != "" {
			login, err = parseWithHeader(c)
		} else {
			login, err = parseWithBody(c)
		}
		if err != nil {
			return "", jwt.ErrFailedAuthentication
		}

		// Get the user information by the login username. 根据用户名查询得到信息
		user, err := store.Client().Users().Get(c, login.Username, metav1.GetOptions{})
		if err != nil {
			log.Errorf("get user information failed: %s", err.Error())

			return "", jwt.ErrFailedAuthentication
		}

		// Compare the login password with the user password.将登录输入的密码和查询得到的加密后的密码进行对比
		if err := user.Compare(login.Password); err != nil {
			return "", jwt.ErrFailedAuthentication
		}

		user.LoginedAt = time.Now()
		_ = store.Client().Users().Update(c, user, metav1.UpdateOptions{})

		return user, nil
	}
}

func parseWithHeader(c *gin.Context) (loginInfo, error) {
	//"Authorization: Basic YWRtaW46QWRtaW5AMjAyMQ=="
	auth := strings.SplitN(c.Request.Header.Get("Authorization"), " ", 2)
	if len(auth) != 2 || auth[0] != "Basic" {
		log.Errorf("get basic string from Authorization header failed")

		return loginInfo{}, jwt.ErrFailedAuthentication
	}

	payload, err := base64.StdEncoding.DecodeString(auth[1]) //解码YWRtaW46QWRtaW5AMjAyMQ==得到 admin:Admin@2021
	if err != nil {
		log.Errorf("decode basic string: %s", err.Error())

		return loginInfo{}, jwt.ErrFailedAuthentication
	}

	pair := strings.SplitN(string(payload), ":", 2) //admin:Admin@2021
	if len(pair) != 2 {
		log.Errorf("parse payload failed")

		return loginInfo{}, jwt.ErrFailedAuthentication
	}

	return loginInfo{
		Username: pair[0],
		Password: pair[1],
	}, nil
}

func parseWithBody(c *gin.Context) (loginInfo, error) {
	var login loginInfo
	if err := c.ShouldBindJSON(&login); err != nil {
		log.Errorf("parse login parameters: %s", err.Error())

		return loginInfo{}, jwt.ErrFailedAuthentication
	}

	return login, nil
}

func refreshResponse() func(c *gin.Context, code int, token string, expire time.Time) {
	return func(c *gin.Context, code int, token string, expire time.Time) {
		c.JSON(http.StatusOK, gin.H{
			"token":  token,
			"expire": expire.Format(time.RFC3339),
		})
	}
}

func loginResponse() func(c *gin.Context, code int, token string, expire time.Time) {
	return func(c *gin.Context, code int, token string, expire time.Time) {
		c.JSON(http.StatusOK, gin.H{
			"token":  token,
			"expire": expire.Format(time.RFC3339),
		})
	}
}

func payloadFunc() func(data interface{}) jwt.MapClaims {
	return func(data interface{}) jwt.MapClaims {
		claims := jwt.MapClaims{
			"iss": APIServerIssuer,
			"aud": APIServerAudience,
		}
		if u, ok := data.(*v1.User); ok { //只取user的一部分脱敏信息
			claims[jwt.IdentityKey] = u.Name
			claims["sub"] = u.Name
		}

		return claims
	}
}

func authorizator() func(data interface{}, c *gin.Context) bool {
	return func(data interface{}, c *gin.Context) bool {
		if v, ok := data.(string); ok {
			log.L(c).Infof("user `%s` is authenticated.", v)

			return true
		}

		return false
	}
}
