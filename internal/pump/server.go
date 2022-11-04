// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package pump

import (
	"context"
	"fmt"
	"sync"
	"time"

	goredislib "github.com/go-redis/redis/v8"
	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v8"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/marmotedu/iam/internal/pump/analytics"
	"github.com/marmotedu/iam/internal/pump/config"
	"github.com/marmotedu/iam/internal/pump/options"
	"github.com/marmotedu/iam/internal/pump/pumps"
	"github.com/marmotedu/iam/internal/pump/storage"
	"github.com/marmotedu/iam/internal/pump/storage/redis"
	"github.com/marmotedu/iam/pkg/log"
)

// 数据采集组件设计的核心是插件化，这里我将需要上报的系统抽象成一个个的 pump.
var pmps []pumps.Pump // 数据采集插件的切片,可以自由添加多种数据采集的插件

type pumpServer struct {
	secInterval    int
	omitDetails    bool
	mutex          *redsync.Mutex // 持有分布式锁
	analyticsStore storage.AnalyticsStorage
	pumps          map[string]options.PumpConfig
}

// preparedGenericAPIServer is a private wrapper that enforces a call of PrepareRun() before Run can be invoked.
type preparedPumpServer struct {
	*pumpServer
}

func createPumpServer(cfg *config.Config) (*pumpServer, error) {
	// use the same redis database with authorization log history
	client := goredislib.NewClient(&goredislib.Options{ // 连接Redis
		Addr:     fmt.Sprintf("%s:%d", cfg.RedisOptions.Host, cfg.RedisOptions.Port),
		Username: cfg.RedisOptions.Username,
		Password: cfg.RedisOptions.Password,
	})

	rs := redsync.New(goredis.NewPool(client))

	server := &pumpServer{
		secInterval:    cfg.PurgeDelay,
		omitDetails:    cfg.OmitDetailedRecording,
		mutex:          rs.NewMutex("iam-pump", redsync.WithExpiry(10*time.Minute)), // 分布式锁
		analyticsStore: &redis.RedisClusterStorageManager{},                         // 上游的数据来源依赖注入为Redis
		pumps:          cfg.Pumps,
	}

	if err := server.analyticsStore.Init(cfg.RedisOptions); err != nil {
		return nil, err
	}

	return server, nil
}

func (s *pumpServer) PrepareRun() preparedPumpServer {
	s.initialize()

	return preparedPumpServer{s}
}

func (s preparedPumpServer) Run(stopCh <-chan struct{}) error {
	ticker := time.NewTicker(time.Duration(s.secInterval) * time.Second) // 进行数据采集的时间间隔
	defer ticker.Stop()

	log.Info("Now run loop to clean data from redis")
	for {
		select {
		case <-ticker.C:
			s.pump()
		// exit consumption cycle when receive SIGINT and SIGTERM signal
		case <-stopCh: // 能退出时一定是pump执行完毕的时候
			log.Info("stop purge loop")

			return nil
		}
	}
}

// pump get authorization log from redis and write to pumps.
func (s *pumpServer) pump() {
	if err := s.mutex.Lock(); err != nil { //分布式锁确保pump集群搭建时,同一时刻只有一个pump对Redis进行消费
		log.Info("there is already an iam-pump instance running.")

		return
	}
	defer func() {
		if _, err := s.mutex.Unlock(); err != nil {
			log.Errorf("could not release iam-pump lock. err: %v", err)
		}
	}()

	analyticsValues := s.analyticsStore.GetAndDeleteSet(storage.AnalyticsKeyName) // 从Redis中获取信息处理
	if len(analyticsValues) == 0 {
		return
	}

	// Convert to something clean
	keys := make([]interface{}, len(analyticsValues))

	for i, v := range analyticsValues {
		decoded := analytics.AnalyticsRecord{}
		err := msgpack.Unmarshal([]byte(v.(string)), &decoded) //将获取到的数据解码
		log.Debugf("Decoded Record: %v", decoded)
		if err != nil {
			log.Errorf("Couldn't unmarshal analytics data: %s", err.Error())
		} else {
			if s.omitDetails { //此处可对数据做处理,此处消除了敏感的数据
				decoded.Policies = ""
				decoded.Deciders = ""
			}
			keys[i] = interface{}(decoded) //转换为空接口类型 存入
		}
	}

	// Send to pumps 将清洗后的数据异步写入到多个下游
	writeToPumps(keys, s.secInterval)
}

func (s *pumpServer) initialize() {
	pmps = make([]pumps.Pump, len(s.pumps)) // 全局pump插件
	i := 0
	for key, pmp := range s.pumps { // 每一个实例对应一个配置
		pumpTypeName := pmp.Type
		if pumpTypeName == "" {
			pumpTypeName = key // type为空则设置为key的名称
		}

		pmpType, err := pumps.GetPumpByName(pumpTypeName) // 从维护的全局pump插件中查询,是否有此名称的pump插件
		if err != nil {
			log.Errorf("Pump load error (skipping): %s", err.Error())
		} else {
			pmpIns := pmpType.New()
			initErr := pmpIns.Init(pmp.Meta) // 将获取到的pump接口初始化,并配置
			if initErr != nil {
				log.Errorf("Pump init error (skipping): %s", initErr.Error())
			} else {
				log.Infof("Init Pump: %s", pmpIns.GetName())
				pmpIns.SetFilters(pmp.Filters)
				pmpIns.SetTimeout(pmp.Timeout)
				pmpIns.SetOmitDetailedRecording(pmp.OmitDetailedRecording)
				pmps[i] = pmpIns // 将配置完毕的pump插件加入到全局的实例切片中
			}
		}
		i++
	}
}

func writeToPumps(keys []interface{}, purgeDelay int) {
	// Send to pumps
	if pmps != nil {
		var wg sync.WaitGroup
		wg.Add(len(pmps))
		for _, pmp := range pmps { // 发送至每个pump,对应多种下游,异步的进行;同一份数据发往多个下游
			go execPumpWriting(&wg, pmp, &keys, purgeDelay)
		}
		wg.Wait()
	} else {
		log.Warn("No pumps defined!")
	}
}

func filterData(pump pumps.Pump, keys []interface{}) []interface{} {
	filters := pump.GetFilters()
	if !filters.HasFilter() && !pump.GetOmitDetailedRecording() {
		return keys
	}
	filteredKeys := keys[:] // nolint: gocritic
	newLenght := 0

	for _, key := range filteredKeys {
		decoded, _ := key.(analytics.AnalyticsRecord)
		if pump.GetOmitDetailedRecording() {
			decoded.Policies = ""
			decoded.Deciders = ""
		}
		if filters.ShouldFilter(decoded) {
			continue
		}
		filteredKeys[newLenght] = decoded
		newLenght++
	}
	filteredKeys = filteredKeys[:newLenght]

	return filteredKeys
}

func execPumpWriting(wg *sync.WaitGroup, pmp pumps.Pump, keys *[]interface{}, purgeDelay int) {
	timer := time.AfterFunc(time.Duration(purgeDelay)*time.Second, func() {
		if pmp.GetTimeout() == 0 {
			log.Warnf(
				"Pump %s is taking more time than the value configured of purge_delay. You should try to set a timeout for this pump.",
				pmp.GetName(),
			)
		} else if pmp.GetTimeout() > purgeDelay {
			log.Warnf("Pump %s is taking more time than the value configured of purge_delay. You should try lowering the timeout configured for this pump.", pmp.GetName())
		}
	})
	defer timer.Stop()
	defer wg.Done()

	log.Debugf("Writing to: %s", pmp.GetName())

	ch := make(chan error, 1)
	var ctx context.Context
	var cancel context.CancelFunc
	// Initialize context depending if the pump has a configured timeout
	if tm := pmp.GetTimeout(); tm > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(tm)*time.Second)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}

	defer cancel()

	go func(ch chan error, ctx context.Context, pmp pumps.Pump, keys *[]interface{}) {
		filteredKeys := filterData(pmp, *keys) //过滤数据

		ch <- pmp.WriteData(ctx, filteredKeys) // 异步调用写入数据
	}(ch, ctx, pmp, keys)

	select {
	case err := <-ch: //写入的结果,没有错误即成功,此协程退出
		if err != nil {
			log.Warnf("Error Writing to: %s - Error: %s", pmp.GetName(), err.Error())
		}
	case <-ctx.Done(): //超时或者取消
		//nolint: errorlint
		switch ctx.Err() {
		case context.Canceled:
			log.Warnf("The writing to %s have got canceled.", pmp.GetName())
		case context.DeadlineExceeded:
			log.Warnf("Timeout Writing to: %s", pmp.GetName())
		}
	}
}
