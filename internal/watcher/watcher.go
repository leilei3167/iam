// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package watcher

import (
	"context"
	"fmt"
	"time"

	goredislib "github.com/go-redis/redis/v8"
	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v8"
	"github.com/robfig/cron/v3"

	genericoptions "github.com/marmotedu/iam/internal/pkg/options"
	"github.com/marmotedu/iam/internal/watcher/options"
	"github.com/marmotedu/iam/internal/watcher/watcher"

	// trigger init functions in `internal/watcher/watcher/`. 此处没有显示的注入某个定时任务的实例,而是通过init函数来注册clean和task.
	_ "github.com/marmotedu/iam/internal/watcher/watcher/all"
	"github.com/marmotedu/iam/pkg/log"
	"github.com/marmotedu/iam/pkg/log/cronlog"
)

type watchJob struct {
	*cron.Cron // 执行定时任务的库
	config     *options.WatcherOptions
	rs         *redsync.Redsync // 分布式锁操作
}

func newWatchJob(redisOptions *genericoptions.RedisOptions, watcherOptions *options.WatcherOptions) *watchJob {
	logger := cronlog.NewLogger(log.SugaredLogger()) // 实现corn.Logger

	client := goredislib.NewClient(&goredislib.Options{ // 建立redis连接
		Addr:     fmt.Sprintf("%s:%d", redisOptions.Host, redisOptions.Port),
		Username: redisOptions.Username,
		Password: redisOptions.Password,
	})

	rs := redsync.New(goredis.NewPool(client))

	cronjob := cron.New( // 定时任务启动器
		cron.WithSeconds(),
		cron.WithChain(cron.SkipIfStillRunning(logger), cron.Recover(logger)), // 类似于中间件,添加记录日志和recover的功能
	)

	return &watchJob{
		Cron:   cronjob,
		config: watcherOptions,
		rs:     rs,
	}
}

func (w *watchJob) addWatchers() *watchJob {
	for name, watch := range watcher.ListWatchers() { // 全局map中获取(由init注册),未直接依赖全局变量,而是用函数
		// log with `{"watcher": "counter"}` key-value to distinguish which watcher the log comes from.
		//nolint: golint,staticcheck
		ctx := context.WithValue(context.Background(), log.KeyWatcherName, name)
		// 依次初始化所有的watcher,每一个独立的watcher对应一个单独的分布式锁
		if err := watch.Init(ctx, w.rs.NewMutex(name, redsync.WithExpiry(2*time.Hour)), w.config); err != nil {
			log.Panicf("construct watcher %s failed: %s", name, err.Error())
		}

		_, _ = w.AddJob(watch.Spec(), watch) // 将watcher任务添加至Corn
	}

	return w
}
