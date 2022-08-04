// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

// Package apiserver does all the work necessary to create a iam APIServer.
package apiserver

import (
	"github.com/marmotedu/iam/internal/apiserver/config"
	"github.com/marmotedu/iam/internal/apiserver/options"
	"github.com/marmotedu/iam/pkg/app"
	"github.com/marmotedu/iam/pkg/log"
)

const commandDesc = `The IAM API server validates and configures data
for the api objects which include users, policies, secrets, and
others. The API Server services REST operations to do the api objects management.

Find more iam-apiserver information at:
    https://github.com/marmotedu/iam/blob/master/docs/guide/en-US/cmd/iam-apiserver.md`

// NewApp creates an App object with default parameters.
func NewApp(basename string) *app.App {
	opts := options.NewOptions()
	application := app.NewApp("IAM API Server", // 典型的选项模式
		basename,
		app.WithOptions(opts), // 选项接口
		app.WithDescription(commandDesc),
		app.WithDefaultValidArgs(),
		app.WithRunFunc(run(opts)), // 关键,这里设置了app的RunFunc
	)

	return application
}

/*
启动流程设计:
首先,NewOptions()创建带有默认值的 Options 类型变量 opts,opts传入NewApp中被用于容纳命令行和配置文件的内容(二者合并),最终
opts会被作为应用创建的配置

接着,会将run函数以opts为参数注册到cobra的RunE中,里面封装了整个组件的启动逻辑,run函数中,首先会初始化log包,后续代码中
随时就可以记录日志了

然后,会通过config.CreateConfigFromOptions函数将携带完整配置的opts来创建应用配置cfg;
应用配置和 Options 配置其实是完全独立的，二者可能完全不同，但在 iam-apiserver 中，二者配置项是相同的。

之后,根据应用配置cfg,创建HTTP/GRPC服务器所使用的配置,配置创建好后会进行补全,补全后的配置调用New()方法,就会创建对应的
服务实例.
*/
func run(opts *options.Options) app.RunFunc { // opts将在此函数执行前被全部赋值(被viper合并后的,在cmd.RunE中)
	return func(basename string) error {
		log.Init(opts.Log)
		defer log.Flush()
		// 此函数被执行时opts都是被全部赋值了的,应用程序根据这些配置创建组件配置
		cfg, err := config.CreateConfigFromOptions(opts)
		if err != nil {
			return err
		}

		// 此处将应用配置传入,程序启动
		return Run(cfg)
	}
}
