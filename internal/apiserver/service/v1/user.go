// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package v1

import (
	"context"
	"regexp"
	"sync"

	v1 "github.com/marmotedu/api/apiserver/v1"
	metav1 "github.com/marmotedu/component-base/pkg/meta/v1"

	"github.com/marmotedu/iam/pkg/errors"

	"github.com/marmotedu/iam/internal/apiserver/store"
	"github.com/marmotedu/iam/internal/pkg/code"
	"github.com/marmotedu/iam/pkg/log"
)

// UserSrv defines functions used to handle user request.
type UserSrv interface { //User 产品定义,满足下列方法的作为User产品被生产
	Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error
	Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error
	Delete(ctx context.Context, username string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, usernames []string, opts metav1.DeleteOptions) error
	Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error)
	List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error)
	ListWithBadPerformance(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error)
	ChangePassword(ctx context.Context, user *v1.User) error
}

type userService struct { //具体的实现产品
	store store.Factory //依赖另一个工厂方法
}

var _ UserSrv = (*userService)(nil)

func newUsers(srv *service) *userService {
	return &userService{store: srv.store}
}

// List returns user list in the storage. This function has a good performance.
/*并发模板:.*/
func (u *userService) List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error) {
	users, err := u.store.Users().List(ctx, opts) //调用下一层的接口(仓库层的实例化得更早)
	if err != nil {
		log.L(ctx).Errorf("list users from storage failed: %s", err.Error())

		return nil, errors.WithCode(code.ErrDatabase, err.Error())
	}

	wg := sync.WaitGroup{}
	errChan := make(chan error, 1)
	finished := make(chan bool, 1)

	var m sync.Map

	// Improve query efficiency in parallel
	for _, user := range users.Items {
		wg.Add(1)

		go func(user *v1.User) {
			defer wg.Done()

			// some cost time process,因为每一个User信息中还需要总的策略数,所以这里需要并发处理
			policies, err := u.store.Policies().List(ctx, user.Name, metav1.ListOptions{})
			if err != nil {
				errChan <- errors.WithCode(code.ErrDatabase, err.Error())

				return
			}

			m.Store(user.ID, &v1.User{
				ObjectMeta: metav1.ObjectMeta{
					ID:         user.ID,
					InstanceID: user.InstanceID,
					Name:       user.Name,
					Extend:     user.Extend,
					CreatedAt:  user.CreatedAt,
					UpdatedAt:  user.UpdatedAt,
				},
				Nickname:    user.Nickname,
				Email:       user.Email,
				Phone:       user.Phone,
				TotalPolicy: policies.TotalCount,
				LoginedAt:   user.LoginedAt,
			})
		}(user)
	}

	go func() {
		wg.Wait()
		close(finished)
	}()

	select { // 阻塞,直到所有的协程退出或者某一个出现错误
	case <-finished:
	case err := <-errChan: // 只要有任何一个goroutine报错,即返回
		return nil, err
	}

	infos := make([]*v1.User, 0, len(users.Items)) // 为了保证并发处理后的顺序
	for _, user := range users.Items {
		info, _ := m.Load(user.ID)             // 使用数据库主键为key存入查询,因为Items是有序的,所以最终结果infos也是有序的
		infos = append(infos, info.(*v1.User)) // 断言成User
	}

	log.L(ctx).Debugf("get %d users from backend storage.", len(infos))

	return &v1.UserList{ListMeta: users.ListMeta, Items: infos}, nil
}

// ListWithBadPerformance returns user list in the storage. This function has a bad performance.
func (u *userService) ListWithBadPerformance(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error) {
	users, err := u.store.Users().List(ctx, opts)
	if err != nil {
		return nil, errors.WithCode(code.ErrDatabase, err.Error())
	}

	infos := make([]*v1.User, 0)
	for _, user := range users.Items {
		policies, err := u.store.Policies().List(ctx, user.Name, metav1.ListOptions{})
		if err != nil {
			return nil, errors.WithCode(code.ErrDatabase, err.Error())
		}

		infos = append(infos, &v1.User{
			ObjectMeta: metav1.ObjectMeta{
				ID:        user.ID,
				Name:      user.Name,
				CreatedAt: user.CreatedAt,
				UpdatedAt: user.UpdatedAt,
			},
			Nickname:    user.Nickname,
			Email:       user.Email,
			Phone:       user.Phone,
			TotalPolicy: policies.TotalCount,
		})
	}

	return &v1.UserList{ListMeta: users.ListMeta, Items: infos}, nil
}

func (u *userService) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error {
	if err := u.store.Users().Create(ctx, user, opts); err != nil {
		if match, _ := regexp.MatchString("Duplicate entry '.*' for key 'idx_name'", err.Error()); match {
			return errors.WithCode(code.ErrUserAlreadyExist, err.Error())
		}

		return errors.WithCode(code.ErrDatabase, err.Error())
	}

	return nil
}

func (u *userService) DeleteCollection(ctx context.Context, usernames []string, opts metav1.DeleteOptions) error {
	if err := u.store.Users().DeleteCollection(ctx, usernames, opts); err != nil {
		return errors.WithCode(code.ErrDatabase, err.Error())
	}

	return nil
}

func (u *userService) Delete(ctx context.Context, username string, opts metav1.DeleteOptions) error {
	if err := u.store.Users().Delete(ctx, username, opts); err != nil {
		return err
	}

	return nil
}

func (u *userService) Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error) {
	user, err := u.store.Users().Get(ctx, username, opts)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (u *userService) Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error {
	if err := u.store.Users().Update(ctx, user, opts); err != nil {
		return errors.WithCode(code.ErrDatabase, err.Error())
	}

	return nil
}

func (u *userService) ChangePassword(ctx context.Context, user *v1.User) error {
	// Save changed fields.
	if err := u.store.Users().Update(ctx, user, metav1.UpdateOptions{}); err != nil {
		return errors.WithCode(code.ErrDatabase, err.Error())
	}

	return nil
}
