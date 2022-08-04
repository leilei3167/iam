// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package store

//go:generate mockgen -self_package=github.com/marmotedu/iam/internal/apiserver/store -destination mock_store.go -package store github.com/marmotedu/iam/internal/apiserver/store Factory,UserStore,SecretStore,PolicyStore

var client Factory

// Factory defines the iam platform storage interface.
// 此接口作为仓库层的唯一入口,一切与数据库的CRUD操作均通过此接口执行,http和grpc的handler实例均依赖此接口.
// 项目将model相关的代码放在了github.com/marmotedu/api,根据组件的api版本来组织存放api组件的model的文件(http的model和GRPC的proto文件),其中model还可能包含一些gorm钩子函数
// 项目所用的最基础通用的组件全部放在github.com/marmotedu/component-base 可以相当于某些项目中的utils文件夹,其中的组件可以被多个项目使用.
// 其中值得关注的是基础组件包的meta/v1的设计,iam-apiserver 将 REST 资源的属性也进一步规范化了,所有的REST资源均支持2种属性: 公有属性和资源自有的属性.
type Factory interface {
	Users() UserStore
	Secrets() SecretStore
	Policies() PolicyStore
	PolicyAudits() PolicyAuditStore
	Close() error
}

// Client return the store client instance.
func Client() Factory {
	return client
}

// SetClient set the iam store client.
func SetClient(factory Factory) {
	client = factory
}
