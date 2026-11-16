package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"top.longmoon/lume/internal/bytecode"
	"top.longmoon/lume/internal/compiler"
	"top.longmoon/lume/internal/lexer"
	"top.longmoon/lume/internal/parser"
	"top.longmoon/lume/internal/repl"
	"top.longmoon/lume/internal/vm"
)

const version = "0.3.1"

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
	case "pack":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "用法: lume pack <file.lume | file.lum>")
			os.Exit(1)
		}
		os.Exit(cmdPack(os.Args[2]))
	}

	// .lum 直接跑
	if strings.HasSuffix(arg, ".lum") {
		code := cmdRun(arg)
		waitExit()
		os.Exit(code)
	}

	// .lume 或其它：内存编译运行
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
	fmt.Println(`Lume v0.3.1 — 块式交互式脚本语言
用法:
  lume                      进入交互式终端
  lume <file.lume>          编译并运行源码（内存中，不落盘）
  lume <file.lum>           运行字节码文件
  lume build <file.lume>    编译为 .lum
  lume run <file.lum>       运行字节码文件
  lume pack <file>          将源码或字节码打包为独立可执行文件 (需要安装 Go)
  lume -v | --version       显示版本
  lume -h | --help          显示帮助`)
}

// ==========================================
// 打包工具逻辑
// ==========================================

// 内嵌运行器源码，%s 会被替换为十六进制字节码
const runnerTemplate = `package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"top.longmoon/lume/internal/bytecode"
	"top.longmoon/lume/internal/vm"
)

var embeddedHex = "%s"

func main() {
	data, err := hex.DecodeString(embeddedHex)
	if err != nil {
		fmt.Printf("内部字节码损坏: %%v\n", err)
		os.Exit(1)
	}

	bp, err := bytecode.Decode(data)
	if err != nil {
		fmt.Printf("加载失败: %%v\n", err)
		os.Exit(1)
	}

	if err := vm.New(bp).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "\n运行时错误: %%v\n", err)
	}

	fmt.Print("\n[按Enter关闭]")
	var b [1]byte
	os.Stdin.Read(b[:])
}
`

func cmdPack(inputFile string) int {
	ext := filepath.Ext(inputFile)
	baseName := strings.TrimSuffix(inputFile, ext)

	if ext != ".lume" && ext != ".lum" {
		fmt.Println("错误: 仅支持 .lume 或 .lum 文件")
		return 1
	}

	var bytecodeData []byte

	// 1. 如果是源码，直接在内存里编译
	if ext == ".lume" {
		fmt.Printf("[1/3] 正在编译 %s ...\n", inputFile)
		src, err := os.ReadFile(inputFile)
		if err != nil {
			fmt.Printf("错误: 读取失败 %v\n", err)
			return 1
		}
		bp, err := compileSource(string(src))
		if err != nil {
			fmt.Printf("错误: 编译失败 %v\n", err)
			return 1
		}
		bytecodeData, _ = bp.Encode()
	} else {
		// 已经是 lum，直接读取
		fmt.Printf("[1/3] 正在读取 %s ...\n", inputFile)
		data, err := os.ReadFile(inputFile)
		if err != nil {
			fmt.Printf("错误: 读取 %s 失败: %v\n", inputFile, err)
			return 1
		}
		bytecodeData = data
	}

	// 2. 转为十六进制注入模板
	fmt.Printf("[2/3] 正在注入字节码 ...\n")
	hexData := hex.EncodeToString(bytecodeData)

	// 注意：必须依赖当前的 internal 目录进行编译
	// 生成临时文件
	tmpGoFile := fmt.Sprintf("embedded_runner_%d.go", os.Getpid())
	runnerCode := fmt.Sprintf(runnerTemplate, hexData)
	
	err := os.WriteFile(tmpGoFile, []byte(runnerCode), 0644)
	if err != nil {
		fmt.Printf("错误: 写入临时源码失败: %v\n", err)
		return 1
	}
	defer os.Remove(tmpGoFile) // 无论成败，最后删除临时文件

	// 3. 调用 go build
	outExe := baseName + ".exe"
	if runtime.GOOS != "windows" {
		outExe = baseName // macOS / Linux 不需要 .exe
	}
	fmt.Printf("[3/3] 正在生成 %s (需要 Go 环境) ...\n", outExe)
	
	cmd := exec.Command("go", "build", "-ldflags", "-s -w", "-o", outExe, tmpGoFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		fmt.Println("错误: 打包编译失败，请确保已安装 Go 且在 PATH 中")
		return 1
	}

	fmt.Printf("打包成功！产物: %s\n", outExe)
	return 0
}
