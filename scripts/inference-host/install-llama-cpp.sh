#!/usr/bin/env bash
#
# install-llama-cpp.sh —— 跨平台安装 llama.cpp（macOS / Linux）
#
# 设计：
#   - 自动探测平台与现有 llama-server
#   - macOS 优先 Homebrew（最快、含 Metal 加速）
#   - Linux 优先 apt（Debian/Ubuntu）；其他发行版走源码 cmake
#   - 源码安装提供 --no-source 跳过选项
#   - 安装后写入 LLAMACPP_BIN 提示，便于 export
#
# 用法：
#   bash scripts/inference-host/install-llama-cpp.sh
#   bash scripts/inference-host/install-llama-cpp.sh --no-source   # 跳过源码兜底
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=env.sh
source "$SCRIPT_DIR/env.sh"

NO_SOURCE=0
for arg in "$@"; do
  case "$arg" in
    --no-source) NO_SOURCE=1 ;;
    -h|--help)
      cat <<EOF
用法: $0 [--no-source]

  --no-source   跳过源码 cmake 兜底（apt/brew 失败时直接报错）
  -h, --help    显示本帮助

安装顺序：
  1) 探测已有 llama-server（存在则跳过）
  2) macOS  : brew install llama.cpp
  3) Linux  : apt install llama.cpp（Debian/Ubuntu sid）或源码 cmake
EOF
      exit 0
      ;;
  esac
done

print_inference_host_banner

# 1) 探测已有
if [[ -n "$LLAMACPP_BIN" ]]; then
  echo "✅ 已检测到 llama-server：$LLAMACPP_BIN"
  "$LLAMACPP_BIN" --version 2>&1 | head -3 || true
  exit 0
fi

OS="$(uname -s)"
ARCH="$(uname -m)"

echo "[install] 平台: $OS / $ARCH"
echo "[install] 当前未检测到 llama-server，开始安装..."

install_macos() {
  if ! command -v brew >/dev/null 2>&1; then
    echo "❌ macOS 需要 Homebrew：https://brew.sh" >&2
    return 1
  fi
  echo "[install] brew install llama.cpp"
  brew install llama.cpp
  # 重新探测
  LLAMACPP_BIN="$(detect_llamacpp_bin)"
  [[ -n "$LLAMACPP_BIN" ]] || { echo "❌ 安装后仍未找到 llama-server" >&2; return 1; }
  echo "✅ 安装完成：$LLAMACPP_BIN"
}

install_linux_apt() {
  echo "[install] 尝试 apt 源..."
  if command -v apt-get >/dev/null 2>&1; then
    # Debian sid / Ubuntu 24.04+ 自带 llama.cpp 包
    if apt-cache show llama.cpp 2>/dev/null | grep -q '^Package:'; then
      sudo apt-get update -y
      sudo apt-get install -y llama.cpp
      LLAMACPP_BIN="$(detect_llamacpp_bin)"
      [[ -n "$LLAMACPP_BIN" ]] && { echo "✅ apt 安装完成：$LLAMACPP_BIN"; return 0; }
    fi
  fi
  return 1
}

install_linux_source() {
  if [[ "$NO_SOURCE" == "1" ]]; then
    echo "❌ --no-source 已禁用源码兜底" >&2
    return 1
  fi
  local build_dir="${HIVEMTK_BUILD_DIR:-$HOME/.hivemtk/build/llama.cpp}"
  echo "[install] 源码安装到：$build_dir"
  mkdir -p "$build_dir"
  if [[ ! -d "$build_dir/llama.cpp" ]]; then
    git clone --depth=1 https://github.com/ggml-org/llama.cpp.git "$build_dir/llama.cpp"
  fi
  cd "$build_dir/llama.cpp"
  # 编译 server + 必要依赖
  cmake -B build
  cmake --build build --config Release -j"$(nproc 2>/dev/null || echo 2)" --target llama-server
  local bin="$build_dir/llama.cpp/build/bin/llama-server"
  if [[ -x "$bin" ]]; then
    local link_dir="$HOME/.local/bin"
    mkdir -p "$link_dir"
    ln -sf "$bin" "$link_dir/llama-server"
    echo "✅ 源码安装完成：$bin"
    echo "   已软链到 $link_dir/llama-server"
    echo "   建议加入 PATH：export PATH=\"$link_dir:\$PATH\""
    LLAMACPP_BIN="$link_dir/llama-server"
  else
    echo "❌ 编译完成但未找到二进制 $bin" >&2
    return 1
  fi
}

case "$OS" in
  Darwin)
    install_macos
    ;;
  Linux)
    install_linux_apt || install_linux_source
    ;;
  *)
    echo "❌ 不支持的平台：$OS（仅 macOS / Linux）" >&2
    exit 1
    ;;
esac

# 验证
if [[ -n "$LLAMACPP_BIN" && -x "$LLAMACPP_BIN" ]]; then
  echo
  echo "============================================================"
  echo "✅ llama.cpp 安装成功"
  echo "   路径：$LLAMACPP_BIN"
  echo "   版本：$("$LLAMACPP_BIN" --version 2>&1 | head -1)"
  echo
  echo "下一步："
  echo "  export LLAMACPP_BIN=\"$LLAMACPP_BIN\""
  echo "  bash $SCRIPT_DIR/download-models.sh"
  echo "============================================================"
else
  echo "❌ 安装失败，请检查上述日志" >&2
  exit 1
fi
