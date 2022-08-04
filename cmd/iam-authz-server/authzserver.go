// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

// authzserver is the server for iam-authz-server.
// It is responsible for serving the ladon authorization request.
package main

import (
	"math/rand"
	"time"

	"github.com/marmotedu/iam/internal/authzserver"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	authzserver.NewApp("iam-authz-server").Run()
}

/*
一.Authz核心功能:
	1.提供HTTP服务,用于鉴权(可用cabin,此处使用的ladon);
为了实现快速获取密钥和策略信息,将此部分维护在内存缓存中,在api-server对数据库进行写操作后,更新缓存数据库

	2.需要实现内存缓存的自动更新
通过订阅Redis Channel实现,API-server会在修改数据后发送一条通知,在接收到通知后,会使用gRPC客户端调用api-server的接口,更新本地缓存
api-server通过Publish中间件,对于secret和policy的POST,UPDATE,DELETE,PATCH请求时,会调用Redis发送通知

	3.提供对鉴权日志的记录
需要实现定期批量存储鉴权记录,使用Redis的Pipe功能

二.重点
	1.高性能异步数据上报功能的实现(异步,批量)
	2.为提高查询性能,缓存的设计,以及如何确保缓存和数据库的数据一致性






*/
