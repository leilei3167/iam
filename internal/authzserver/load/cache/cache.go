// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package cache

import (
	"sync"

	"github.com/dgraph-io/ristretto"
	pb "github.com/marmotedu/api/proto/apiserver/v1"
	"github.com/ory/ladon"

	"github.com/marmotedu/iam/pkg/errors"

	"github.com/marmotedu/iam/internal/authzserver/store"
)

// Cache is used to store secrets and policies.
// 负责管理从grpc调用apiserver时得到的结果,同时实现了Loader接口.
type Cache struct {
	lock     *sync.RWMutex
	cli      store.Factory
	secrets  *ristretto.Cache // 高性能内存缓存  github.com/dgraph-io/ristretto
	policies *ristretto.Cache // 可以使用Redis代替
}

var (
	// ErrSecretNotFound defines secret not found error.
	ErrSecretNotFound = errors.New("secret not found")
	// ErrPolicyNotFound defines policy not found error.
	ErrPolicyNotFound = errors.New("policy not found")
)

var (
	onceCache sync.Once
	cacheIns  *Cache // 全局缓存,维护来自apiserver拉取的信息
)

// GetCacheInsOr return store instance.
func GetCacheInsOr(cli store.Factory) (*Cache, error) {
	var err error
	if cli != nil {
		var (
			secretCache *ristretto.Cache
			policyCache *ristretto.Cache
		)

		onceCache.Do(func() {
			// 创建内存缓存
			c := &ristretto.Config{
				NumCounters: 1e7,     // number of keys to track frequency of (10M).
				MaxCost:     1 << 30, // maximum cost of cache (1GB).
				BufferItems: 64,      // number of keys per Get buffer.
				Cost:        nil,
			}

			secretCache, err = ristretto.NewCache(c)
			if err != nil {
				return
			}
			policyCache, err = ristretto.NewCache(c)
			if err != nil {
				return
			}

			cacheIns = &Cache{
				cli:      cli,
				lock:     new(sync.RWMutex),
				secrets:  secretCache,
				policies: policyCache,
			}
		})
	}

	return cacheIns, err
}

// GetSecret return secret detail for the given key.
func (c *Cache) GetSecret(key string) (*pb.SecretInfo, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	value, ok := c.secrets.Get(key)
	if !ok {
		return nil, ErrSecretNotFound
	}

	return value.(*pb.SecretInfo), nil
}

// GetPolicy return user's ladon policies for the given user.
// 关键:ladon最终会调用此方法 获取缓存的policy 进行鉴权.
func (c *Cache) GetPolicy(key string) ([]*ladon.DefaultPolicy, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	value, ok := c.policies.Get(key)
	if !ok {
		return nil, ErrPolicyNotFound
	}

	return value.([]*ladon.DefaultPolicy), nil
}

// Reload reload secrets and policies.
func (c *Cache) Reload() error { // 刷新缓存的操作,全程必须加锁
	c.lock.Lock()
	defer c.lock.Unlock()

	// reload secrets
	secrets, err := c.cli.Secrets().List() // 使用gRPC客户端调用apiserver,获取所有secrets信息
	if err != nil {
		return errors.Wrap(err, "list secrets failed")
	}

	c.secrets.Clear() // 清空缓存
	for key, val := range secrets {
		c.secrets.Set(key, val, 1) // 以secretID作为map的Key
	}

	// reload policies
	policies, err := c.cli.Policies().List()
	if err != nil {
		return errors.Wrap(err, "list policies failed")
	}

	c.policies.Clear()
	for key, val := range policies {
		c.policies.Set(key, val, 1)
	}

	return nil
}
