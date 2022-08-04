// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package apiserver

import "github.com/marmotedu/iam/internal/apiserver/config"

// Run runs the specified APIServer. This should never exit.
func Run(cfg *config.Config) error {
	// 根据应用配置创建对应的实例
	server, err := createAPIServer(cfg)
	if err != nil {
		return err
	}
	// 创建好实例之后,使用PrepareRun准备函数,这里进行一些初始化的操作,入数据库的初始化,中间件,API路由的注册等等
	// 最后在Run中 启动了对应的服务
	return server.PrepareRun().Run()
}
