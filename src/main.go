package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"top.longmoon/lume/internal/bytecode"
	"top.longmoon/lume/internal/compiler"
	"top.longmoon/lume/internal/lexer"
	"top.longmoon/lume/internal/parser"
	"top.longmoon/lume/internal/repl"
	"top.longmoon/lume/internal/vm"
)

const version = "0.3.0"

func main() {
	if len(os.Args) < 2 {
		repl.New().Run()
		return
	}

	arg := os.Args[1]
	switch arg {
	case "-v", "--version":
		fmt.Printf("Lume %s\n", version)
		return
	case "-h", "--help":
		printUsage()
		return
	case "build":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "用法: lume build <file.lume>")
			os.Exit(1)
		}
		os.Exit(cmdBuild(os.Args[2]))
	case "run":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "用法: lume run <file.lum>")
			os.Exit(1)
		}
		os.Exit(cmdRun(os.Args[2]))
	}

	// .lum 直接跑：带等待
	if strings.HasSuffix(arg, ".lum") {
		code := cmdRun(arg)
		waitExit()
		os.Exit(code)
	}

	// .lume 或其它：内存编译运行，带等待
	os.Exit(cmdRunSourceFile(arg))
}

func cmdBuild(path string) int {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lume: 无法读取 %s: %v\n", path, err)
		return 1
	}
	bp, err := compileSource(string(src))
	if err != nil {
		fmt.Fprintf(os.Stderr, "lume: 编译失败: %v\n", err)
		return 1
	}
	data, err := bp.Encode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "lume: 编码失败: %v\n", err)
		return 1
	}
	out := strings.TrimSuffix(path, ".lume") + ".lum"
	if err := os.WriteFile(out, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "lume: 写入 %s 失败: %v\n", out, err)
		return 1
	}
	fmt.Printf("已编译 %s -> %s (%d bytes)\n", path, out, len(data))
	return 0
}

func cmdRun(path string) int {
	if strings.HasSuffix(path, ".lume") {
		fmt.Fprintln(os.Stderr, "lume: lume run 只接受 .lum 字节码文件")
		fmt.Fprintln(os.Stderr, "      运行 .lume 源码请用: lume "+path)
		return 1
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lume: 无法读取 %s: %v\n", path, err)
		return 1
	}
	bp, err := bytecode.Decode(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lume: %v\n", err)
		return 1
	}
	if err := vm.New(bp).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "lume: %v\n", err)
		return 1
	}
	return 0
}

func cmdRunSourceFile(path string) (code int) {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lume: 无法读取文件 %s: %v\n", path, err)
		waitExit()
		return 1
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "lume: %v\n", r)
			code = 1
		}
	}()
	bp, err := compileSource(string(src))
	if err != nil {
		fmt.Fprintf(os.Stderr, "lume: %v\n", err)
		waitExit()
		return 1
	}
	if err := vm.New(bp).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "lume: %v\n", err)
		waitExit()
		return 1
	}
	waitExit()
	return 0
}

func compileSource(src string) (*bytecode.Program, error) {
	tokens := lexer.New(src).Tokenize()
	for _, t := range tokens {
		if t.Type == "ILLEGAL" {
			return nil, fmt.Errorf("词法错误 L%d:%d %s", t.Line, t.Col, t.Literal)
		}
	}
	prog := parser.New(tokens).Parse()
	return compiler.Compile(prog)
}

func waitExit() {
	fmt.Print("\n[按Enter关闭]")
	bufio.NewReader(os.Stdin).ReadByte()
	fmt.Println()
}

func printUsage() {
	fmt.Println(`Lume v0.3.0 — 块式交互式脚本语言
用法:
  lume                      进入交互式终端
  lume <file.lume>          编译并运行源码（内存中，不落盘）
  lume <file.lum>           运行字节码文件
  lume build <file.lume>    编译为 .lum
  lume run <file.lum>       运行字节码文件
  lume -v | --version       显示版本
  lume -h | --help          显示帮助`)
}