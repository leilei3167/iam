// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package v1

//go:generate mockgen -self_package=github.com/marmotedu/iam/internal/apiserver/service/v1 -destination mock_service.go -package v1 github.com/marmotedu/iam/internal/apiserver/service/v1 Service,UserSrv,SecretSrv,PolicySrv

import "github.com/marmotedu/iam/internal/apiserver/store"

// Service defines functions used to return resource interface.
// 工厂方法模式,一个接口包含多个接口的实现
// 此处Service就相当于是一个工厂抽象接口.
type Service interface { //生产多种产品的方法列表
	Users() UserSrv // 生产UserSrv产品的方法
	Secrets() SecretSrv
	Policies() PolicySrv
}

type service struct { // 实现工厂接口
	store store.Factory // 业务层对仓库层 依赖Factory接口
}

// NewService returns Service interface.
func NewService(store store.Factory) Service { //初始化工厂
	return &service{
		store: store,
	}
}

func (s *service) Users() UserSrv { //UserSrv产品
	return newUsers(s)
}

func (s *service) Secrets() SecretSrv {
	return newSecrets(s)
}

func (s *service) Policies() PolicySrv {
	return newPolicies(s)
}
