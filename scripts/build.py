#!/usr/bin/env python3
"""
Lume 构建脚本
用法: python scripts/build.py
产物: src/lume.exe, src/lumerun.exe
"""

import os
import sys
import shutil
import subprocess
from pathlib import Path

SCRIPT_DIR = Path(__file__).resolve().parent
ROOT = SCRIPT_DIR.parent
SRC = ROOT / "src"

LUME_EXE = SRC / "lume.exe"
LUMERUN_EXE = SRC / "lumerun.exe"
ICON_LUME = SRC / "lume.ico"
ICON_RUN = SRC / "lumerun.ico"
SYSO_LUME = SRC / "rsrc.syso"
SYSO_RUN = SRC / "cmd" / "lumerun" / "rsrc.syso"


def run(cmd, cwd=None):
    print(f"[lume] {' '.join(str(c) for c in cmd)}")
    return subprocess.run(cmd, cwd=cwd).returncode


def command_exists(cmd):
    try:
        subprocess.run(cmd, stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL, check=True)
        return True
    except Exception:
        return False


def find_rsrc():
    """
    找 rsrc.exe 的顺序：
    1. 环境变量 RSRC
    2. PATH 里的 rsrc
    3. GOPATH/bin/rsrc(.exe)
    4. 用户目录下 go/bin/rsrc(.exe)
    找不到返回 None
    """
    # 1. 环境变量
    env = os.environ.get("RSRC")
    if env and Path(env).exists():
        return Path(env)

    # 2. PATH
    p = shutil.which("rsrc")
    if p:
        return Path(p)

    # 3. go env GOPATH
    try:
        out = subprocess.check_output(["go", "env", "GOPATH"], text=True).strip()
        if out:
            for name in ("rsrc.exe", "rsrc"):
                candidate = Path(out) / "bin" / name
                if candidate.exists():
                    return candidate
    except Exception:
        pass

    # 4. 用户目录 go/bin
    for name in ("rsrc.exe", "rsrc"):
        candidate = Path.home() / "go" / "bin" / name
        if candidate.exists():
            return candidate

    return None


def install_rsrc():
    print("[lume] 未找到 rsrc，正在自动安装 ...")
    # 优先用国内代理，不行就直连
    env = os.environ.copy()
    if "GOPROXY" not in env:
        env["GOPROXY"] = "https://goproxy.cn,direct"
    try:
        r = subprocess.run(
            ["go", "install", "github.com/akavel/rsrc@latest"],
            env=env
        )
        return r.returncode == 0
    except Exception as e:
        print(f"[lume] 安装 rsrc 失败: {e}")
        return False


def ensure_rsrc():
    rsrc = find_rsrc()
    if rsrc:
        return rsrc

    if not install_rsrc():
        print("[lume] rsrc 自动安装失败，请手动安装：")
        print("       go install github.com/akavel/rsrc@latest")
        print("       或设置环境变量 RSRC 指向 rsrc.exe")
        return None

    rsrc = find_rsrc()
    if not rsrc:
        print("[lume] 安装完成但找不到 rsrc，请检查 GOPATH 是否在 PATH 中")
        return None

    print(f"[lume] rsrc 已就绪: {rsrc}")
    return rsrc


def ensure_syso(rsrc: Path, icon: Path, out: Path):
    if not icon.exists():
        print(f"[lume] 缺少图标 {icon}")
        return False
    out.parent.mkdir(parents=True, exist_ok=True)
    if out.exists() and out.stat().st_mtime >= icon.stat().st_mtime:
        return True
    print(f"[lume] rsrc {icon.name} -> {out.relative_to(ROOT)}")
    return run([str(rsrc), "-ico", str(icon), "-o", str(out)]) == 0


def clean_stale_syso():
    for p in SRC.glob("*.syso"):
        if p.name != "rsrc.syso":
            try:
                p.unlink()
                print(f"[lume] 清理旧 syso: {p.name}")
            except Exception:
                pass


def main():
    if not (SRC / "go.mod").exists():
        print(f"[lume] 找不到 {SRC / 'go.mod'}")
        return 1

    if not command_exists(["go", "version"]):
        print("[lume] 未找到 go，请先安装 Go")
        return 1

    rsrc = ensure_rsrc()
    if not rsrc:
        return 1

    clean_stale_syso()

    if not ensure_syso(rsrc, ICON_LUME, SYSO_LUME):
        return 1
    if not ensure_syso(rsrc, ICON_RUN, SYSO_RUN):
        return 1

    if run(["go", "mod", "tidy"], cwd=SRC) != 0:
        print("[lume] go mod tidy 失败")
        return 1

    if run(["go", "build", "-trimpath", "-ldflags", "-s -w",
            "-o", "lume.exe", "."], cwd=SRC) != 0:
        print("[lume] lume.exe 构建失败")
        return 1

    if run(["go", "build", "-trimpath", "-ldflags", "-s -w",
            "-o", "lumerun.exe", "./cmd/lumerun"], cwd=SRC) != 0:
        print("[lume] lumerun.exe 构建失败")
        return 1

    for exe in (LUME_EXE, LUMERUN_EXE):
        if exe.exists():
            print(f"[lume] {exe.name} 大小: {exe.stat().st_size} bytes")

    run([str(LUME_EXE), "-v"], cwd=SRC)
    run([str(LUMERUN_EXE), "-v"], cwd=SRC)

    print("[lume] 构建成功 ✔")
    return 0


if __name__ == "__main__":
    sys.exit(main())