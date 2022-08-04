// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package templates

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"unicode"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"

	"github.com/marmotedu/iam/internal/iamctl/util/term"
)

type FlagExposer interface {
	ExposeFlags(cmd *cobra.Command, flags ...string) FlagExposer
}

// ActsAsRootCommand 执行此函数会修改默认的终端输出格式.
func ActsAsRootCommand(cmd *cobra.Command, filters []string, groups ...CommandGroup) FlagExposer {
	if cmd == nil {
		panic("nil root command")
	}
	templater := &templater{ // 设置模板,通过templater的方法来格式化界面,会根据cmd的命令构建来生成展示的页面
		RootCmd:       cmd,
		UsageTemplate: MainUsageTemplate(), // 添加Usage页面的模板
		HelpTemplate:  MainHelpTemplate(),  // 添加Help命令的模板(拼接多个模板)
		CommandGroups: groups,
		Filtered:      filters,
	}
	cmd.SilenceUsage = true                         // 关闭默认的Usage
	cmd.SetFlagErrorFunc(templater.FlagErrorFunc()) // 设置当输入错误的flag的时候打印的信息
	cmd.SetUsageFunc(
		templater.UsageFunc(),
	) // 实际就是执行templater中已经传入的模板(Excute的参数是cobra的Command,模板语法可以调用其方法以及访问字段)
	cmd.SetHelpFunc(templater.HelpFunc())
	return templater
}

func UseOptionsTemplates(cmd *cobra.Command) {
	templater := &templater{
		UsageTemplate: OptionsUsageTemplate(),
		HelpTemplate:  OptionsHelpTemplate(),
	}
	cmd.SetUsageFunc(templater.UsageFunc())
	cmd.SetHelpFunc(templater.HelpFunc())
}

// 负责对输出模板的格式化.
type templater struct {
	UsageTemplate string
	HelpTemplate  string
	RootCmd       *cobra.Command
	CommandGroups
	Filtered []string
}

func (t *templater) FlagErrorFunc(
	exposedFlags ...string,
) func(*cobra.Command, error) error { // 作为cobra命令的Flag错误时的页面展示函数
	return func(c *cobra.Command, err error) error {
		c.SilenceUsage = true
		switch c.CalledAs() { // 返回当前调用的命令的名称
		case "options":
			return fmt.Errorf("%s\nrun '%s' without flags", err, c.CommandPath())
		default:
			return fmt.Errorf("%s\nsee '%s --help' for usage", err, c.CommandPath())
		}
	}
}

func (t *templater) ExposeFlags(cmd *cobra.Command, flags ...string) FlagExposer {
	cmd.SetUsageFunc(t.UsageFunc(flags...))
	return t
}

func (t *templater) HelpFunc() func(*cobra.Command, []string) {
	return func(c *cobra.Command, s []string) {
		tt := template.New("help")
		tt.Funcs(t.templateFuncs()) // 传入自定义的函数,在模板中能够调用
		template.Must(tt.Parse(t.HelpTemplate))
		out := term.NewResponsiveWriter(c.OutOrStdout()) // 根据终端大小调整输出
		err := tt.Execute(out, c)
		if err != nil {
			c.Println(err)
		}
	}
}

func (t *templater) UsageFunc(exposedFlags ...string) func(*cobra.Command) error {
	return func(c *cobra.Command) error {
		tt := template.New("usage")                // 创建模板引擎
		tt.Funcs(t.templateFuncs(exposedFlags...)) // 传入模板需要的函数,一般不会在模板中直接定义函数
		template.Must(tt.Parse(t.UsageTemplate))   // 解析指定的模板格式(Must是一个包装,确保执行成功)
		out := term.NewResponsiveWriter(c.OutOrStderr())
		return tt.Execute(out, c) // 将Command结构体作为参数传入,在模板中可以访问Command的字段和方法,执行之后就是模板语法来进行数据的填充与输出
	}
}

func (t *templater) templateFuncs(exposedFlags ...string) template.FuncMap {
	return template.FuncMap{
		"trim":                strings.TrimSpace,
		"trimRight":           func(s string) string { return strings.TrimRightFunc(s, unicode.IsSpace) },
		"trimLeft":            func(s string) string { return strings.TrimLeftFunc(s, unicode.IsSpace) },
		"gt":                  cobra.Gt,
		"eq":                  cobra.Eq,
		"rpad":                rpad,
		"appendIfNotPresent":  appendIfNotPresent,
		"flagsNotIntersected": flagsNotIntersected,
		"visibleFlags":        visibleFlags,
		"flagsUsages":         flagsUsages,
		"cmdGroups":           t.cmdGroups,
		"cmdGroupsString":     t.cmdGroupsString,
		"rootCmd":             t.rootCmdName,
		"isRootCmd":           t.isRootCmd,
		"optionsCmdFor":       t.optionsCmdFor,
		"usageLine":           t.usageLine,
		"exposed": func(c *cobra.Command) *flag.FlagSet { // 指定只展示哪些flag
			exposed := flag.NewFlagSet("exposed", flag.ContinueOnError)
			if len(exposedFlags) > 0 {
				for _, name := range exposedFlags {
					if flag := c.Flags().Lookup(name); flag != nil {
						exposed.AddFlag(flag)
					}
				}
			}
			return exposed
		},
	}
}

// 将所有的命令归到Available Commands: 下,而根命令不会进行分组.
func (t *templater) cmdGroups(c *cobra.Command, all []*cobra.Command) []CommandGroup {
	if len(t.CommandGroups) > 0 && c == t.RootCmd {
		all = filter(all, t.Filtered...)
		return AddAdditionalCommands(t.CommandGroups, "Other Commands:", all)
	}
	all = filter(all, "options")
	return []CommandGroup{
		{
			Message:  "Available Commands:",
			Commands: all,
		},
	}
}

// 将该命令的子命令按分组打印.
// TODO:.
func (t *templater) cmdGroupsString(c *cobra.Command) string {
	groups := []string{}
	for _, cmdGroup := range t.cmdGroups(c, c.Commands()) { // 获取各个分组
		cmds := []string{cmdGroup.Message}      // 名称 \n
		for _, cmd := range cmdGroup.Commands { // 打印当前分组的名称
			if cmd.IsAvailableCommand() {
				cmds = append(cmds, "  "+rpad(cmd.Name(), cmd.NamePadding())+" "+cmd.Short)
			}
		}
		groups = append(groups, strings.Join(cmds, "\n"))
	}
	return strings.Join(groups, "\n\n")
}

func (t *templater) rootCmdName(c *cobra.Command) string {
	return t.rootCmd(c).CommandPath()
}

func (t *templater) isRootCmd(c *cobra.Command) bool {
	return t.rootCmd(c) == c
}

func (t *templater) parents(c *cobra.Command) []*cobra.Command {
	parents := []*cobra.Command{c}                                    // 当前命令->父命令1->父命令2->根命令
	for current := c; !t.isRootCmd(current) && current.HasParent(); { // 当前命令不是根命令,且有父命令
		current = current.Parent()         // 则向上级遍历
		parents = append(parents, current) // 整个调用链
	}
	return parents
}

func (t *templater) rootCmd(c *cobra.Command) *cobra.Command {
	if c != nil && !c.HasParent() { // 没有父命令的就是根命令
		return c
	}
	if t.RootCmd == nil {
		panic("nil root cmd")
	}
	return t.RootCmd
}

// 组合打印,需要组成一条命令链,如rootCmd cmd1 cmd2 cmd3 options.
func (t *templater) optionsCmdFor(c *cobra.Command) string {
	if !c.Runnable() {
		return ""
	}
	rootCmdStructure := t.parents(c)                  // 获取从当前命令到根命令的路径
	for i := len(rootCmdStructure) - 1; i >= 0; i-- { // 从最后一个(即根命令遍历)
		cmd := rootCmdStructure[i]
		if _, _, err := cmd.Find([]string{"options"}); err == nil { // 从根命令往下搜索options
			return cmd.CommandPath() + " options"
		}
	}
	return ""
}

func (t *templater) usageLine(c *cobra.Command) string {
	usage := c.UseLine()
	suffix := "[options]"
	if c.HasFlags() && !strings.Contains(usage, suffix) { // 如果有flag,并且其usage中不包含[options]后缀
		usage += " " + suffix // 则添加后缀
	}
	return usage
}

func flagsUsages(f *flag.FlagSet) string {
	x := new(bytes.Buffer)

	f.VisitAll(func(flag *flag.Flag) {
		if flag.Hidden {
			return
		}
		format := "--%s=%s: %s\n"

		if flag.Value.Type() == "string" {
			format = "--%s='%s': %s\n"
		}

		if len(flag.Shorthand) > 0 {
			format = "  -%s, " + format
		} else {
			format = "   %s   " + format
		}

		fmt.Fprintf(x, format, flag.Shorthand, flag.Name, flag.DefValue, flag.Usage)
	})

	return x.String()
}

func rpad(s string, padding int) string {
	template := fmt.Sprintf("%%-%ds", padding)
	return fmt.Sprintf(template, s)
}

func appendIfNotPresent(s, stringToAppend string) string {
	if strings.Contains(s, stringToAppend) {
		return s
	}
	return s + " " + stringToAppend
}

// 检测command的LocalFlags和PersistentFlags.
func flagsNotIntersected(l *flag.FlagSet, r *flag.FlagSet) *flag.FlagSet {
	f := flag.NewFlagSet("notIntersected", flag.ContinueOnError)
	l.VisitAll(func(flag *flag.Flag) {
		if r.Lookup(flag.Name) == nil { // 在全局flag中没有的本地flag,添加到set中
			f.AddFlag(flag)
		}
	})
	return f
}

// 将help 的flag隐藏.
func visibleFlags(l *flag.FlagSet) *flag.FlagSet {
	hidden := "help"
	f := flag.NewFlagSet("visible", flag.ContinueOnError)
	l.VisitAll(func(flag *flag.Flag) {
		if flag.Name != hidden {
			f.AddFlag(flag)
		}
	})
	return f
}

func filter(cmds []*cobra.Command, names ...string) []*cobra.Command {
	out := []*cobra.Command{}
	for _, c := range cmds {
		if c.Hidden {
			continue
		}
		skip := false
		for _, name := range names {
			if name == c.Name() {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		out = append(out, c)
	}
	return out
}
