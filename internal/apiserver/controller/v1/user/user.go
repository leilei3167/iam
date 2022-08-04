// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package user

import (
	srvv1 "github.com/marmotedu/iam/internal/apiserver/service/v1"
	"github.com/marmotedu/iam/internal/apiserver/store"
)

// UserController create a user handler used to handle request for user resource.
type UserController struct {
	srv srvv1.Service
}

// NewUserController creates a user handler.
func NewUserController(store store.Factory) *UserController {
	return &UserController{
		srv: srvv1.NewService(store),
	}
}

/*



处理函数的设计:
	由一个XXXController结构体继承Service接口,将处理函数作为这个结构的方法来构建
Service接口定义以下几个方法:
	Users() UserSrv
    Secrets() SecretSrv
    Policies() PolicySrv

调用方调用抽象工厂 NewXXXController()函数,传入 store.Factory 实例,即可创建;store.Factory代表着不同类型的数据库抽象实现


*/
