// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/marmotedu/component-base/pkg/json"

	"github.com/marmotedu/iam/internal/authzserver/load"
	"github.com/marmotedu/iam/pkg/log"
	"github.com/marmotedu/iam/pkg/storage"
)

// Publish publish a redis event to specified redis channel when some action occurred.
// TODO:学习redis的用法
func Publish() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next() //先执行

		if c.Writer.Status() != http.StatusOK { //执行未成功,则不进行消息发布
			log.L(c).Debugf("request failed with http status code `%d`, ignore publish message", c.Writer.Status())

			return
		}

		var resource string

		pathSplit := strings.Split(c.Request.URL.Path, "/")
		if len(pathSplit) > 2 {
			resource = pathSplit[2]
		}

		method := c.Request.Method

		switch resource { //发布消息
		case "policies":
			notify(c, method, load.NoticePolicyChanged)
		case "secrets":
			notify(c, method, load.NoticeSecretChanged)
		default:
		}
	}
}

func notify(ctx context.Context, method string, command load.NotificationCommand) {
	switch method { //仅仅针对修改的方法
	case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
		redisStore := &storage.RedisCluster{} //isCache 为false,使用的非cache客户端,使用Redis必须通过RedisCluster
		message, _ := json.Marshal(load.Notification{Command: command})

		if err := redisStore.Publish(load.RedisPubSubChannel, string(message)); err != nil {
			log.L(ctx).Errorw("publish redis message failed", "error", err.Error())
		}
		log.L(ctx).Debugw("publish redis message", "method", method, "command", command)
	default:
	}
}
