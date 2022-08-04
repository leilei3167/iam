package main

import (
	"fmt"
	"time"

	"github.com/marmotedu/iam/pkg/shutdown"
	"github.com/marmotedu/iam/pkg/shutdown/shutdownmanagers/posixsignal"
)

func main() {
	//初始化GracefulShutdown
	gs := shutdown.New()
	gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

	//向其中添加关停时的回调函数
	gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		fmt.Println("Shutdown callback start!")
		time.Sleep(time.Second * 3)
		fmt.Println("shutdown callback end! exiting...")
		return nil
	}))

	//启动manager
	if err := gs.Start(); err != nil {
		fmt.Println("start:", err)
	}
	//做其他事,可以阻塞在此,在所有的回调函数执行完毕后会之间退出程序
	fmt.Println("do something")
	time.Sleep(time.Second * 100)

}
