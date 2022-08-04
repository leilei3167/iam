// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package load

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/marmotedu/iam/pkg/log"
	"github.com/marmotedu/iam/pkg/storage"
)

// Loader defines function to reload storage.
type Loader interface { // Loader不关心Reload如何实现,只会在特定时间执行Reload方法,因此可以扩展不同的缓存策略
	Reload() error
}

// Load is used to reload given storage.
type Load struct {
	ctx    context.Context
	lock   *sync.RWMutex
	loader Loader
}

// NewLoader return a loader with a loader implement.
func NewLoader(ctx context.Context, loader Loader) *Load {
	return &Load{
		ctx:    ctx,
		lock:   new(sync.RWMutex),
		loader: loader,
	}
}

// Start start a loop service.
func (l *Load) Start() {
	go startPubSubLoop()   // 订阅Redis,在指定channel得到消息时,执行指定的任务(投入到reloadQueue中)
	go l.reloadQueueLoop() // 读取reloadQueue中的消息,一旦接收到,将其中的函数加入到一个队列中
	// 1s is the minimum amount of time between hot reloads. The
	// interval counts from the start of one reload to the next.
	go l.reloadLoop() // 消费队列中的Reload消息

	l.DoReload() // 立刻先同步一次
}

func startPubSubLoop() {
	cacheStore := storage.RedisCluster{} // 使用的非cache集群
	cacheStore.Connect()
	// On message, synchronize
	for {
		err := cacheStore.StartPubSubHandler(RedisPubSubChannel, func(v interface{}) { // 订阅Redis的消息,接收到通知时,执行回调
			handleRedisEvent(v, nil, nil)
		})
		if err != nil {
			if !errors.Is(err, storage.ErrRedisIsDown) {
				log.Errorf("Connection to Redis failed, reconnect in 10s: %s", err.Error())
			}

			time.Sleep(10 * time.Second)
			log.Warnf("Reconnecting: %s", err.Error())
		}
	}
}

// shouldReload returns true if we should perform any reload. Reloads happens if
// we have reload callback queued.
func shouldReload() ([]func(), bool) {
	requeueLock.Lock()
	defer requeueLock.Unlock()
	if len(requeue) == 0 {
		return nil, false
	}
	n := requeue         // 将本地队列交出执行
	requeue = []func(){} // 清空

	return n, true
}

func (l *Load) reloadLoop(complete ...func()) {
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-l.ctx.Done():
			return
		// We don't check for reload right away as the gateway peroms this on the
		// startup sequence. We expect to start checking on the first tick after the
		// gateway is up and running.
		case <-ticker.C:
			cb, ok := shouldReload() // 看队列中是否为空,不为空则执行已获取的函数
			if !ok {
				continue // 为空,跳过
			}
			start := time.Now() // 不为空,执行刷新缓存
			l.DoReload()
			for _, c := range cb {
				// most of the callbacks are nil, we don't want to execute nil functions to
				// avoid panics.
				if c != nil {
					c() // 回调不为nil,执行
				}
			}
			if len(complete) != 0 {
				complete[0]()
			}
			log.Infof("reload: cycle completed in %v", time.Since(start))
		}
	}
}

// reloadQueue used to queue a reload. It's not
// buffered, as reloadQueueLoop should pick these up immediately.
var reloadQueue = make(chan func())

var requeueLock sync.Mutex

// This is a list of callbacks to execute on the next reload. It is protected by
// requeueLock for concurrent use.
var requeue []func()

func (l *Load) reloadQueueLoop(cb ...func()) {
	for {
		select {
		case <-l.ctx.Done():
			return
		case fn := <-reloadQueue:
			requeueLock.Lock()
			requeue = append(requeue, fn)
			requeueLock.Unlock()
			log.Info("Reload queued")
			if len(cb) != 0 {
				cb[0]()
			}
		}
	}
}

// DoReload reload secrets and policies.
func (l *Load) DoReload() {
	l.lock.Lock()
	defer l.lock.Unlock()

	if err := l.loader.Reload(); err != nil {
		log.Errorf("faild to refresh target storage: %s", err.Error())
	}

	log.Debug("refresh target storage succ")
}
