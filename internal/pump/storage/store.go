// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

// Package storage defines storages which store the analytics data from iam-authz-server.
package storage

// AnalyticsStorage defines the analytics storage interface
// 上游的数据来源抽象为接口.
type AnalyticsStorage interface { //抽象的上游数据接口
	Init(config interface{}) error
	GetName() string
	Connect() bool
	GetAndDeleteSet(string) []interface{}
}

const (
	// AnalyticsKeyName defines the key name in redis which used to analytics.
	AnalyticsKeyName string = "iam-system-analytics"
)
