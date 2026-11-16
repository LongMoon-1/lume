package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"top.longmoon/lume/internal/bytecode"
	"top.longmoon/lume/internal/vm"
)

const version = "0.3.1"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		waitExit()
		os.Exit(1)
	}

	arg := os.Args[1]
	switch arg {
	case "-v", "--version":
		fmt.Printf("LumeRun %s\n", version)
		return
	case "-h", "--help":
		printUsage()
		return
	}

	if !strings.HasSuffix(arg, ".lum") {
		fmt.Fprintf(os.Stderr, "lumerun: 只能运行 .lum 文件（收到 %q）\n", arg)
		fmt.Fprintln(os.Stderr, "提示：.lume 源码请先用 lume build 编译成 .lum")
		waitExit()
		os.Exit(1)
	}

	data, err := os.ReadFile(arg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lumerun: 无法读取 %s\n", arg)
		fmt.Fprintf(os.Stderr, "        %v\n", err)
		waitExit()
		os.Exit(1)
	}

	bp, err := bytecode.Decode(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lumerun: 无法加载 %s\n", arg)
		fmt.Fprintf(os.Stderr, "        %v\n", err)
		fmt.Fprintln(os.Stderr, "提示：可能是文件损坏、不是 .lum 文件，或由不兼容的 lume 版本编译")
		waitExit()
		os.Exit(1)
	}

	exitCode := 0
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "\nlumerun: 运行时错误: %v\n", r)
				exitCode = 1
			}
		}()
		if err := vm.New(bp).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "\nlumerun: %v\n", err)
			exitCode = 1
		}
	}()

	waitExit()
	os.Exit(exitCode)
}

// waitExit 打印提示并等待回车；EOF 也返回，避免管道场景卡死
func waitExit() {
	fmt.Print("\n[按Enter关闭]")
	bufio.NewReader(os.Stdin).ReadByte()
	fmt.Println()
}

func printUsage() {
	fmt.Println(`LumeRun v0.3.1 — Lume 字节码运行器
用法:
  lumerun <file.lum>     运行字节码文件
  lumerun -v | --version 显示版本
  lumerun -h | --help    显示帮助

说明:
  lumerun 只能运行 .lum 字节码文件，不含编译器。
  .lume 源码请先用 lume build 编译。`)
}
