package repl

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"top.longmoon/lume/internal/bytecode"
	"top.longmoon/lume/internal/compiler"
	"top.longmoon/lume/internal/lexer"
	"top.longmoon/lume/internal/parser"
	"top.longmoon/lume/internal/vm"
)

type REPL struct {
	reader  *bufio.Reader
	history []string
}

func New() *REPL {
	return &REPL{reader: bufio.NewReader(os.Stdin)}
}

func (r *REPL) Run() {
	fmt.Println("Lume v0.3.0 交互式终端")
	fmt.Println("输入代码执行；输入 help 查看命令；exit 退出；Ctrl+C 强制退出")
	fmt.Println()

	for {
		fmt.Print("lume> ")
		line, err := r.reader.ReadString('\n')
		if err != nil {
			fmt.Println()
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		switch {
		case line == "exit" || line == "quit":
			return
		case line == "help" || line == "?":
			printHelp()
			continue
		case line == "pwd":
			wd, _ := os.Getwd()
			fmt.Println(wd)
			continue
		case strings.HasPrefix(line, "cd "):
			r.cd(strings.TrimSpace(line[3:]))
			continue
		case strings.HasPrefix(line, "build "):
			r.buildFile(strings.TrimSpace(line[6:]))
			continue
		case strings.HasPrefix(line, "run "):
			r.runLumFile(strings.TrimSpace(line[4:]))
			continue
		case strings.HasPrefix(line, "lume "):
			r.runSourceFile(strings.TrimSpace(line[5:]))
			continue
		}

		r.evalLine(line)
	}
}

func printHelp() {
	fmt.Println(`REPL 命令：
  <代码>              执行代码（表达式自动打印结果）
  build <file.lume>   编译源码为 .lum
  run <file.lum>      运行字节码文件
  lume <file.lume>    内存编译并运行源码（不落盘）
  cd <目录>           切换当前目录
  pwd                 显示当前目录
  help / ?            显示本帮助
  exit / quit         退出
  Ctrl+C              强制退出`)
}

func (r *REPL) cd(path string) {
	if path == "" {
		fmt.Println("用法: cd <目录>")
		return
	}
	if err := os.Chdir(path); err != nil {
		fmt.Printf("cd 失败: %v\n", err)
		return
	}
	wd, _ := os.Getwd()
	fmt.Println(wd)
}

func (r *REPL) buildFile(path string) {
	if path == "" {
		fmt.Println("用法: build <file.lume>")
		return
	}
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("无法读取 %s: %v\n", path, err)
		return
	}
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				fmt.Printf("编译错误: %v\n", rec)
			}
		}()
		bp, err := compileSource(string(src))
		if err != nil {
			fmt.Printf("编译错误: %v\n", err)
			return
		}
		data, err := bp.Encode()
		if err != nil {
			fmt.Printf("编码失败: %v\n", err)
			return
		}
		out := strings.TrimSuffix(path, ".lume") + ".lum"
		if err := os.WriteFile(out, data, 0644); err != nil {
			fmt.Printf("写入 %s 失败: %v\n", out, err)
			return
		}
		fmt.Printf("已编译 %s -> %s (%d bytes)\n", path, out, len(data))
	}()
}

func (r *REPL) runLumFile(path string) {
	if path == "" {
		fmt.Println("用法: run <file.lum>")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("无法读取 %s: %v\n", path, err)
		return
	}
	bp, err := bytecode.Decode(data)
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}
	if err := vm.New(bp).Run(); err != nil {
		fmt.Printf("错误: %v\n", err)
	}
}

func (r *REPL) runSourceFile(path string) {
	if path == "" {
		fmt.Println("用法: lume <file.lume>")
		return
	}
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("无法读取 %s: %v\n", path, err)
		return
	}
	r.evalSource(string(src))
}

func (r *REPL) evalLine(line string) {
	defer func() {
		if rec := recover(); rec != nil {
			fmt.Printf("错误: %v\n", rec)
		}
	}()
	if isExprOnly(line) {
		r.evalSource("msg{ " + line + " }")
		return
	}
	r.history = append(r.history, line)
	r.evalSource(strings.Join(r.history, "\n"))
}

func (r *REPL) evalSource(src string) {
	defer func() {
		if rec := recover(); rec != nil {
			fmt.Printf("错误: %v\n", rec)
		}
	}()
	tokens := lexer.New(src).Tokenize()
	for _, t := range tokens {
		if t.Type == "ILLEGAL" {
			fmt.Printf("词法错误 L%d:%d %s\n", t.Line, t.Col, t.Literal)
			return
		}
	}
	prog := parser.New(tokens).Parse()
	bp, err := compiler.Compile(prog)
	if err != nil {
		fmt.Printf("编译错误: %v\n", err)
		return
	}
	if err := vm.New(bp).Run(); err != nil {
		fmt.Printf("错误: %v\n", err)
	}
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

func isExprOnly(s string) bool {
	blockKeywords := []string{"var{", "msg{", "input{", "if{", "loop{", "while{", "for{", "set{", "func ", "delay{"}
	for _, k := range blockKeywords {
		if strings.Contains(s, k) {
			return false
		}
	}
	if strings.Contains(s, ":=") || strings.Contains(s, ": ") {
		return false
	}
	return true
}