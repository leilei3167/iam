// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package authzserver

import (
	"context"

	"github.com/marmotedu/iam/pkg/errors"

	"github.com/marmotedu/iam/internal/authzserver/analytics"
	"github.com/marmotedu/iam/internal/authzserver/config"
	"github.com/marmotedu/iam/internal/authzserver/load"
	"github.com/marmotedu/iam/internal/authzserver/load/cache"
	"github.com/marmotedu/iam/internal/authzserver/store/apiserver"
	genericoptions "github.com/marmotedu/iam/internal/pkg/options"
	genericapiserver "github.com/marmotedu/iam/internal/pkg/server"
	"github.com/marmotedu/iam/pkg/log"
	"github.com/marmotedu/iam/pkg/shutdown"
	"github.com/marmotedu/iam/pkg/shutdown/shutdownmanagers/posixsignal"
	"github.com/marmotedu/iam/pkg/storage"
)

// RedisKeyPrefix defines the prefix key in redis for analytics data.
const RedisKeyPrefix = "analytics-"

type authzServer struct {
	gs               *shutdown.GracefulShutdown
	rpcServer        string // apiserver提供的rpc服务的地址
	clientCA         string
	redisOptions     *genericoptions.RedisOptions
	genericAPIServer *genericapiserver.GenericAPIServer
	analyticsOptions *analytics.AnalyticsOptions
	redisCancelFunc  context.CancelFunc
}

type preparedAuthzServer struct {
	*authzServer
}

// func createAuthzServer(cfg *config.Config) (*authzServer, error) {.
func createAuthzServer(cfg *config.Config) (*authzServer, error) {
	gs := shutdown.New()
	gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

	genericConfig, err := buildGenericConfig(cfg)
	if err != nil {
		return nil, err
	}

	genericServer, err := genericConfig.Complete().New() // http服务api
	if err != nil {
		return nil, err
	}

	server := &authzServer{
		gs:               gs,
		redisOptions:     cfg.RedisOptions,
		analyticsOptions: cfg.AnalyticsOptions,
		rpcServer:        cfg.RPCServer,
		clientCA:         cfg.ClientCA,
		genericAPIServer: genericServer,
	}

	return server, nil
}

func (s *authzServer) PrepareRun() preparedAuthzServer {
	_ = s.initialize() // 1.初始化并连接到Redis;2.初始化内存缓存,并开启事件监听,同步(用Redis的发布订阅和gRPC调用api-server实现)

	initRouter(s.genericAPIServer.Engine)

	return preparedAuthzServer{s}
}

// Run start to run AuthzServer.
func (s preparedAuthzServer) Run() error {
	// in order to ensure that the reported data is not lost,
	// please ensure the following graceful shutdown sequence
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		s.genericAPIServer.Close()
		if s.analyticsOptions.Enable {
			analytics.GetAnalytics().Stop()
		}
		s.redisCancelFunc()

		return nil
	}))

	// start shutdown managers
	if err := s.gs.Start(); err != nil {
		log.Fatalf("start shutdown manager failed: %s", err.Error())
	}

	return s.genericAPIServer.Run()
}

func buildGenericConfig(cfg *config.Config) (genericConfig *genericapiserver.Config, lastErr error) {
	genericConfig = genericapiserver.NewConfig()
	if lastErr = cfg.GenericServerRunOptions.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	if lastErr = cfg.FeatureOptions.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	if lastErr = cfg.SecureServing.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	if lastErr = cfg.InsecureServing.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	return
}

func (s *authzServer) buildStorageConfig() *storage.Config {
	return &storage.Config{
		Host:                  s.redisOptions.Host,
		Port:                  s.redisOptions.Port,
		Addrs:                 s.redisOptions.Addrs,
		MasterName:            s.redisOptions.MasterName,
		Username:              s.redisOptions.Username,
		Password:              s.redisOptions.Password,
		Database:              s.redisOptions.Database,
		MaxIdle:               s.redisOptions.MaxIdle,
		MaxActive:             s.redisOptions.MaxActive,
		Timeout:               s.redisOptions.Timeout,
		EnableCluster:         s.redisOptions.EnableCluster,
		UseSSL:                s.redisOptions.UseSSL,
		SSLInsecureSkipVerify: s.redisOptions.SSLInsecureSkipVerify,
	}
}

func (s *authzServer) initialize() error {
	ctx, cancel := context.WithCancel(context.Background())
	s.redisCancelFunc = cancel

	// keep redis connected;使用Redis配置连接Redis(动态的维护2个Redis集群)
	go storage.ConnectToRedis(ctx, s.buildStorageConfig()) // Redis用于订阅apiserver,如果apiserver发生了对数据库的写操作,则会收到通知,进行一次缓存的更新

	// cron to reload all secrets and policies from iam-apiserver
	// 创建gRPC客户端,并初始化本地内存缓存(同时实现Loader接口)
	cacheIns, err := cache.GetCacheInsOr(apiserver.GetAPIServerFactoryOrDie(s.rpcServer, s.clientCA))
	if err != nil {
		return errors.Wrap(err, "get cache instance failed")
	}

	load.NewLoader(ctx, cacheIns).Start() // 同步服务;TODO:学习 1.Redis的服务被抽象成统一的服务,在pkg/storage中;2.事件驱动的同步服务实现方式

	// 数据上报功能的开关 设置为 true 后 iam-authz-server 会向Redis上报授权审计日志
	// 因为数据上报会影响性能,因此需要设计为可配置的
	// 此处数据上报至Redis中,由Pump组件进行数据采集与处理
	// 数据处理模型:
	// 应用程序对数据进行上报->input(redis,kafka等)->数据处理模块(去重,精简数据等)->output(ES,MongoDB等下游存储模块)
	// 此处对应数据上报阶段
	// 需要考虑:
	// 1.如何设计数据上报,使得其对应用代码影响最小?
	// 2.input中数据产生速度大于了output的消费能力,产生数据堆积,怎么办?
	// 3.数据采集后需要存储到下游系统。在存储之前，我们需要对数据进行不同的处理，并可能会存储到不同的下游系统，这种可变的需求如何满足？
	if s.analyticsOptions.Enable {
		analyticsStore := storage.RedisCluster{
			KeyPrefix: RedisKeyPrefix,
		} // 设置key前缀;RedisCluster实现了AnalyticsHandler接口
		analyticsIns := analytics.NewAnalytics(s.analyticsOptions, &analyticsStore) // 创建数据上报服务
		analyticsIns.Start()
	}

	return nil
}
