// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

// Package analytics defines functions and structs used to store authorization audit data to redis.
package analytics

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/vmihailenco/msgpack/v5"

	"github.com/marmotedu/iam/pkg/log"
	"github.com/marmotedu/iam/pkg/storage"
)

const analyticsKeyName = "iam-system-analytics"

const (
	recordsBufferForcedFlushInterval = 1 * time.Second
)

// AnalyticsRecord encodes the details of a authorization request.
type AnalyticsRecord struct {
	TimeStamp  int64     `json:"timestamp"`
	Username   string    `json:"username"`
	Effect     string    `json:"effect"`
	Conclusion string    `json:"conclusion"`
	Request    string    `json:"request"`
	Policies   string    `json:"policies"`
	Deciders   string    `json:"deciders"`
	ExpireAt   time.Time `json:"expireAt"   bson:"expireAt"`
}

var analytics *Analytics // 单例模式,此处确定只会被初始化一次,所以没有用Once;在鉴权失败或成功时会被调用

// SetExpiry set expiration time to a key.
func (a *AnalyticsRecord) SetExpiry(expiresInSeconds int64) {
	expiry := time.Duration(expiresInSeconds) * time.Second
	if expiresInSeconds == 0 {
		// Expiry is set to 100 years
		expiry = 24 * 365 * 100 * time.Hour
	}

	t := time.Now()
	t2 := t.Add(expiry)
	a.ExpireAt = t2
}

// Analytics will record analytics data to a redis back end as defined in the Config object.
type Analytics struct { // 功能模块化,相当于类,NewXXX相当于他的构造函数,下面跟一系列的方法,使得功能围绕着Analytics展开
	store                      storage.AnalyticsHandler
	poolSize                   int
	recordsChan                chan *AnalyticsRecord
	workerBufferSize           uint64
	recordsBufferFlushInterval uint64
	shouldStop                 uint32
	poolWg                     sync.WaitGroup
}

// NewAnalytics returns a new analytics instance.
func NewAnalytics(options *AnalyticsOptions, store storage.AnalyticsHandler) *Analytics {
	ps := options.PoolSize
	recordsBufferSize := options.RecordsBufferSize
	workerBufferSize := recordsBufferSize / uint64(ps)
	log.Debug("Analytics pool worker buffer size", log.Uint64("workerBufferSize", workerBufferSize))

	recordsChan := make(chan *AnalyticsRecord, recordsBufferSize)

	analytics = &Analytics{
		store:                      store,
		poolSize:                   ps, // worker个数
		recordsChan:                recordsChan,
		workerBufferSize:           workerBufferSize,      // 每个worker的批量投递的缓冲大小
		recordsBufferFlushInterval: options.FlushInterval, // 投递最长间隔
	}

	return analytics
}

// GetAnalytics returns the existed analytics instance.
// Need to initialize `analytics` instance before calling GetAnalytics.
func GetAnalytics() *Analytics {
	return analytics
}

// Start start the analytics service.
func (r *Analytics) Start() {
	r.store.Connect() //连接到存储目的地(此处为redis)

	// start worker pool
	atomic.SwapUint32(&r.shouldStop, 0)
	for i := 0; i < r.poolSize; i++ {
		r.poolWg.Add(1)
		go r.recordWorker() // 并发记录
	}
}

// Stop stop the analytics service.
func (r *Analytics) Stop() {
	// flag to stop sending records into channel
	atomic.SwapUint32(&r.shouldStop, 1) // 设置退出标记,避免在此期间继续投递

	// close channel to stop workers;关闭后会使得所有的worker感知到,将各自当前缓冲区的消息立即投递后退出
	close(r.recordsChan)

	// wait for all workers to be done
	r.poolWg.Wait()
}

// RecordHit will store an AnalyticsRecord in Redis.
func (r *Analytics) RecordHit(record *AnalyticsRecord) error {
	// check if we should stop sending records 1st
	if atomic.LoadUint32(&r.shouldStop) > 0 { // 在缓存前，需要判断上报服务是否在优雅关停中，如果在关停中，则丢弃该消息
		return nil
	}

	// just send record to channel consumed by pool of workers
	// leave all data crunching and Redis I/O work for pool workers
	r.recordsChan <- record // 将日志内容写入Chan后退出(异步上报)

	return nil
}

// 默认会开启50个工作协程,发送的条件:1.达到单次发送的容量上限;2.达到了发送等待间隔的上限(需记录上一次发送的时间).
func (r *Analytics) recordWorker() {
	defer r.poolWg.Done() // 确保计数器归零

	// this is buffer to send one pipelined command to redis
	// use r.recordsBufferSize as cap to reduce slice re-allocations
	recordsBuffer := make([][]byte, 0, r.workerBufferSize) // 用于存放批量日志数据的切片,以此为单位发送

	// read records from channel and process
	lastSentTS := time.Now()
	for {
		var readyToSend bool
		select {
		case record, ok := <-r.recordsChan: //来自于鉴权api调用RecordHit方法
			// check if channel was closed and it is time to exit from worker
			if !ok { // 支持优雅退出,通道关闭时,将当前缓冲区的数据发送完毕再退出
				// send what is left in buffer,将缓冲中剩余的数据发送
				r.store.AppendToSetPipelined(analyticsKeyName, recordsBuffer)

				return
			}

			// we have new record - prepare it and add to buffer

			if encoded, err := msgpack.Marshal(record); err != nil { // msgpack序列化 比 JSON 更快、更小
				log.Errorf("Error encoding analytics data: %s", err.Error())
			} else {
				recordsBuffer = append(recordsBuffer, encoded)
			}

			// identify that buffer is ready to be sent
			// 此处为阈值判断，当buffer中的数据量达到阈值时，就可以发送了(单个数据的容量并未指定)
			readyToSend = uint64(len(recordsBuffer)) == r.workerBufferSize

		case <-time.After(time.Duration(r.recordsBufferFlushInterval) * time.Millisecond): // 超时投递,避免消息产生较慢,达不到batchSize无法发送的情况
			// nothing was received for that period of time
			// anyways send whatever we have, don't hold data too long in buffer
			// 超过了发送超时
			readyToSend = true
		}

		// send data to Redis and reset buffer
		// 此处判断投递条件,决定是否投递,投递后将Buffer重置,避免重复投递
		if len(recordsBuffer) > 0 && (readyToSend || time.Since(lastSentTS) >= recordsBufferForcedFlushInterval) {
			r.store.AppendToSetPipelined(analyticsKeyName, recordsBuffer) // 投递
			recordsBuffer = recordsBuffer[:0]                             // 重置
			lastSentTS = time.Now()
		}
	}
}

// DurationToMillisecond convert time duration type to float64.
func DurationToMillisecond(d time.Duration) float64 {
	return float64(d) / 1e6
}
