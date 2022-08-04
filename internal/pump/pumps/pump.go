// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package pumps

import (
	"context"
	"errors"

	"github.com/marmotedu/iam/internal/pump/analytics"
)

// Pump defines the interface for all analytics back-end.数据采集插件抽象.
type Pump interface {
	GetName() string
	New() Pump                                      // 构造
	Init(interface{}) error                         // 初始化,此处建立与下游的连接等
	WriteData(context.Context, []interface{}) error // 向下游写入数据,为了性能,建议能够实现批量的写入
	SetFilters(analytics.AnalyticsFilters)          // 设置数据的过滤,对于数据采集模块非常重要的需求
	GetFilters() analytics.AnalyticsFilters
	SetTimeout(timeout int) // 设置超时时间,确保采集系统正常运行
	GetTimeout() int
	SetOmitDetailedRecording(bool) // 过滤掉详细的数据,避免过于详细的数据快速站满存储空间
	GetOmitDetailedRecording() bool
}

// GetPumpByName returns the pump instance by given name.
func GetPumpByName(name string) (Pump, error) {
	if pump, ok := availablePumps[name]; ok && pump != nil {
		return pump, nil
	}

	return nil, errors.New(name + " Not found")
}
