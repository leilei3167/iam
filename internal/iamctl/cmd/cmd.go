// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

// Package cmd create a root cobra command and add subcommands to it.
package cmd

import (
	"flag"
	"io"
	"os"

	cliflag "github.com/marmotedu/component-base/pkg/cli/flag"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/marmotedu/iam/internal/iamctl/cmd/color"
	"github.com/marmotedu/iam/internal/iamctl/cmd/completion"
	"github.com/marmotedu/iam/internal/iamctl/cmd/info"
	"github.com/marmotedu/iam/internal/iamctl/cmd/jwt"
	"github.com/marmotedu/iam/internal/iamctl/cmd/new"
	"github.com/marmotedu/iam/internal/iamctl/cmd/options"
	"github.com/marmotedu/iam/internal/iamctl/cmd/policy"
	"github.com/marmotedu/iam/internal/iamctl/cmd/secret"
	"github.com/marmotedu/iam/internal/iamctl/cmd/set"
	"github.com/marmotedu/iam/internal/iamctl/cmd/user"
	cmdutil "github.com/marmotedu/iam/internal/iamctl/cmd/util"
	"github.com/marmotedu/iam/internal/iamctl/cmd/validate"
	"github.com/marmotedu/iam/internal/iamctl/cmd/version"
	"github.com/marmotedu/iam/internal/iamctl/util/templates"
	genericapiserver "github.com/marmotedu/iam/internal/pkg/server"
	"github.com/marmotedu/iam/pkg/cli/genericclioptions"
)

// NewDefaultIAMCtlCommand creates the `iamctl` command with default arguments.
// 命令建议分组,每一个命令以一个包来组织,其子命令,以<命令_子命令.go>的格式来命名文件.
func NewDefaultIAMCtlCommand() *cobra.Command {
	return NewIAMCtlCommand(os.Stdin, os.Stdout, os.Stderr)
}

// NewIAMCtlCommand returns new initialized instance of 'iamctl' root command.
func NewIAMCtlCommand(in io.Reader, out, err io.Writer) *cobra.Command {
	// Parent command to which all subcommands are added.
	// 1.创建基本的根命令,其他所有命令都添加到此之下
	cmds := &cobra.Command{
		Use:   "iamctl",
		Short: "iamctl controls the iam platform",
		Long: templates.LongDesc(`
		iamctl controls the iam platform, is the client side tool for iam platform.

		Find more information at:
			https://github.com/marmotedu/iam/blob/master/docs/guide/en-US/cmd/iamctl/iamctl.md`),
		Run: runHelp, // 根命令默认输出帮助函数
		// Hook before and after Run initialize and write profiles to disk,
		// respectively.
		PersistentPreRunE: func(*cobra.Command, []string) error {
			return initProfiling()
		},
		PersistentPostRunE: func(*cobra.Command, []string) error {
			return flushProfiling()
		},
	}
	// 2.添加命令行选项,此处对命令行的flag进行添加
	flags := cmds.PersistentFlags()
	flags.SetNormalizeFunc(cliflag.WarnWordSepNormalizeFunc) // Warn for "_" flags

	// Normalize all flags that are coming from other packages or pre-configurations
	// a.k.a. change all "_" to "-". e.g. glog package
	flags.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)

	// 向根命令中添加全局的flag(//TODO:全局的flag是隐藏的,需要options命令才会展示,如何做到?)
	addProfilingFlags(flags)

	iamConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag().WithDeprecatedSecretFlag()
	iamConfigFlags.AddFlags(flags) // 添加到根命令的flagset中
	matchVersionIAMConfigFlags := cmdutil.NewMatchVersionFlags(
		iamConfigFlags,
	) // 接口套接口的设计,父类子类都同时实现一个接口,可以在子类中添加额外的逻辑,此处是 版本匹配
	matchVersionIAMConfigFlags.AddFlags(cmds.PersistentFlags()) // 添加match-server-version的flag,默认false

	// 3.此处将完成配置文件的解析;
	// iamctl 需要连接 iam-apiserver，来完成用户、策略和密钥的增删改查，并且需要进行认证。要完成这些功能，需要有比较多的配置项。
	// 这些配置项如果每次都在命令行选项指定，会很麻烦，也容易出错,最好的办法是保存在配置文件中
	//配置文件的优先级:
	//1. --iamconfig 参数显示指定的配置文件
	//2. 当前目录下的iamctl.yaml文件
	//$HOME/.iam/iamctl.yaml 文件(设置的默认配置文件路径)
	_ = viper.BindPFlags(cmds.PersistentFlags()) // 将flag全部加入到配置中,未设置的将为零值
	cobra.OnInitialize(func() {                  // 在每个命令执行时,读取配置信息
		genericapiserver.LoadConfig(viper.GetString(genericclioptions.FlagIAMConfig), "iamctl")
	}) // 之后在执行过程中就可使用viper.GetXXX来获取配置的值
	cmds.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	// 创建用于调用API的接口,在子命令中可以使用其RESTClient或IAMClientSet()方法,分别获取
	// RESTful 和SDK的客户端,从而使用客户端提供的函数
	// TODO:注意此处的接口设计
	f := cmdutil.NewFactory(matchVersionIAMConfigFlags)

	// From this point and forward we get warnings on flags that contain "_" separators
	cmds.SetGlobalNormalizationFunc(cliflag.WarnWordSepNormalizeFunc)

	ioStreams := genericclioptions.IOStreams{In: in, Out: out, ErrOut: err}
	//------------------------------------------------------------------------
	// 命令分组,组内的命令又有自己的子命令
	// 每个命令构建的方式可以完全不同，但最好能按相同的方式去构建，并抽象成一个模型
	// 分组->父命令->子命令
	// 分组中包含若干个父命令,而每个父命令有包含若干个子命令,命令的创建抽象为一下几个行为
	// 1.1 NewCmdXxx:创建命令的框架,初始化cobra.Command的Short,Long,Example,帮助函数等字段
	// 1.2 命令的命令行选项由cmd.Flags().XxxVar来添加
	// 2.通过NewXxxoptions返回带有默认flag的选项(Options),使得命令在未指定flag时也能执行
	// 3.Options具有Complete,Validate,Run三个方法,分别完成选项补全,选项验证,和命令执行,命令
	// 的执行逻辑在func(o *Options)Run(args []string)error中编写
	// 有了抽象的模型,可以按照规定的模型,自动生成新的命令
	groups := templates.CommandGroups{ // 各个命令都组织在一个CommandGroup结构中
		{
			Message: "Basic Commands:",
			Commands: []*cobra.Command{ // 每一个子命令(包括子命令的子命令)的构建都是一样的
				info.NewCmdInfo(f, ioStreams),
				color.NewCmdColor(f, ioStreams),
				new.NewCmdNew(f, ioStreams),
				jwt.NewCmdJWT(f, ioStreams),
			},
		},
		{
			Message: "Identity and Access Management Commands:",
			Commands: []*cobra.Command{
				user.NewCmdUser(f, ioStreams),
				secret.NewCmdSecret(f, ioStreams),
				policy.NewCmdPolicy(f, ioStreams),
			},
		},
		{
			Message: "Troubleshooting and Debugging Commands:",
			Commands: []*cobra.Command{
				validate.NewCmdValidate(f, ioStreams),
			},
		},
		{
			Message: "Settings Commands:",
			Commands: []*cobra.Command{
				set.NewCmdSet(f, ioStreams),
				completion.NewCmdCompletion(ioStreams.Out, ""),
			},
		},
	}
	groups.Add(cmds) // 将各个分组命令添加到根命令下,但此时的输出依然是默认的状态(所有命令和flag全部打印),需要修改输出至终端的格式
	//------------------------------------------------------------------------

	filters := []string{"options"}
	templates.ActsAsRootCommand(cmds, filters, groups...) // TODO:重点:将所有的命令整理,输出模板的设置;此处用的模板语法

	cmds.AddCommand(version.NewCmdVersion(f, ioStreams))
	cmds.AddCommand(options.NewCmdOptions(ioStreams.Out))

	return cmds
}

func runHelp(cmd *cobra.Command, args []string) {
	_ = cmd.Help()
}
