# Lume

> 一门用 Go 编写的块式脚本语言，编译为字节码运行。

[![Build](https://img.shields.io/github/actions/workflow/status/longmoon-1/lume/build.yml?branch=main&style=flat-square)](https://github.com/longmoon-1/lume/actions)
[![Release](https://img.shields.io/github/v/release/longmoon-1/lume?style=flat-square)](https://github.com/longmoon-1/lume/releases)
[![License](https://img.shields.io/github/license/longmoon-1/lume?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/github/go-mod/go-version/longmoon-1/lume?style=flat-square)](src/go.mod)

Lume 是一门以 `关键字{ ... }` 组织代码的交互式脚本语言。内置了输入输出、数组、函数、文件读写和 HTTP 服务器，适用于命令行小游戏、交互式脚本以及简单的本地后端。

文档：https://files.longmoon.top/md/lume-doc

### 特性

- **块式语法**：结构清晰，无需依赖缩进。
- **编译为字节码**：区别于传统的树遍历解释器，代码会先编译为 `.lum` 字节码再执行，速度更快。
- **单文件运行**：`lume.exe` 无第三方依赖，下载即用。
- **开箱即用**：内置 `input` / `msg` / `file` / `http` 模块，无需安装额外包。

### 安装

**系统要求**：Windows 10 及以上（x64）。

前往 [Releases](https://github.com/longmoon-1/lume/releases) 下载最新的 `lume.exe`，放入系统环境变量 `PATH` 中的任意目录。

**从源码构建**：
```bash
git clone https://github.com/longmoon-1/lume
cd lume
python scripts/build.py
```
构建产物生成在 `src/lume.exe` 和 `src/lumerun.exe`。

### 快速开始

新建 `hello.lume`：
```lume
var{
  name: "World"
}

msg{ "Hello, " + name }
```

运行代码（内存编译并执行）：
```bash
lume hello.lume
```

编译为字节码并执行：
```bash
lume build hello.lume    # 生成 hello.lum
lumerun hello.lum        # 使用纯运行器执行
```

进入交互式终端（REPL）：
```bash
lume
```

### 示例

除了基础运算，Lume 内置了 HTTP 模块，可以直接写出一个简单后端：
```lume
var{
  import: "http"
}

http_route{
  method: "GET"
  path: "/api/ping"
  body: "{\"ok\":true}"
  type: "application/json"
}

http_static{
  root: "public"
  index: "index.html"
}

http_serve{
  port: 8080
  log: true
  msg{ "服务器在 localhost:" + port + " 开放" }
}
```
更多完整示例（如猜数字、石头剪刀布、带存档的剧情游戏等）请查看 [examples](src/examples) 目录。

### 致谢

Lume 在设计和实现上深受以下项目启发：
- **[Lua](https://www.lua.org/)**：借用了其块式语法（`end` 替换为 `}`）、`--` 单行注释风格，以及动态类型的运行语义。
- **[Crafting Interpreters](https://craftinginterpreters.com/)**：本项目的字节码编译器与栈式虚拟机架构参考了该书的设计路线。
- **Go**：作为宿主语言，Go 强大的标准库承担了文件系统、HTTP、字符串等底层基础能力。

Lume 并非任何语言的方言，而是一个独立实现的脚本语言。

### 开源协议

[MIT](LICENSE)
