// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

// iamctl is the command line tool for iam platform.
package main

import (
	"os"

	"github.com/marmotedu/iam/internal/iamctl/cmd"
)

func main() {
	command := cmd.NewDefaultIAMCtlCommand()
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}

/*
iamctl是一个高可用性的命令行工具
	命令行工具的实现关键:
	1.命令的分组处理,需要一个命令抽象模型,子命令统一采用类似的结构来生成
	2.配置文件的管理,配置文件的路径搜索;
	3.展示页面的自定义模板的编写(模板语法,隐藏部分命令)



*/
