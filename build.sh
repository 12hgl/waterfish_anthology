#!/usr/bin/env bash
#
# 水鱼文集 —— 一键构建脚本
#
#   ./build.sh                 构建 x86 与 arm 两个 .fpk
#   ./build.sh x86             只构建 x86
#   ./build.sh arm             只构建 arm
#   VERSION=1.1.0 ./build.sh   指定版本号
#   ./build.sh --icons         额外重新生成应用图标
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC="${ROOT}/src"
PKG="${ROOT}/packaging"
DIST="${ROOT}/dist"
WORK="${ROOT}/.build"
TOOLS="${ROOT}/.tools"
APPNAME="waterfish_anthology"
VERSION="${VERSION:-1.0.0}"
FNPACK_VERSION="1.2.3"
FNPACK_URL="https://static2.fnnas.com/fnpack/fnpack-${FNPACK_VERSION}-linux-amd64"

GEN_ICONS=0
FILTERED=()
for arg in "$@"; do
  case "${arg}" in
    --icons) GEN_ICONS=1 ;;
    x86|arm) FILTERED+=("${arg}") ;;
    *) echo "未知参数：${arg}" >&2; exit 1 ;;
  esac
done
if [ "${#FILTERED[@]}" -eq 0 ]; then
  FILTERED=(x86 arm)
fi

log() { printf '\033[36m▸\033[0m %s\n' "$*"; }
die() { printf '\033[31m✗\033[0m %s\n' "$*" >&2; exit 1; }

# ---------- 前置检查 ----------
command -v go >/dev/null || die "未找到 go，请先安装 Go 1.21+"

fnpack_bin() {
  if command -v fnpack >/dev/null; then echo "fnpack"; return; fi
  local local_bin="${TOOLS}/fnpack"
  if [ ! -x "${local_bin}" ]; then
    log "下载 fnpack ${FNPACK_VERSION} …"
    mkdir -p "${TOOLS}"
    curl -fsSL "${FNPACK_URL}" -o "${local_bin}" || die "fnpack 下载失败"
    chmod +x "${local_bin}"
  fi
  echo "${local_bin}"
}

# ---------- 图标 ----------
if [ "${GEN_ICONS}" = "1" ]; then
  log "重新生成应用图标"
  python3 "${PKG}/make-icons.py" || die "图标生成失败"
fi

# ---------- 编译 + 打包 ----------
FNPACK="$(fnpack_bin)"
mkdir -p "${DIST}"
rm -rf "${WORK}"
mkdir -p "${WORK}"

for platform in "${FILTERED[@]}"; do
  case "${platform}" in
    x86) goarch="amd64" ;;
    arm) goarch="arm64" ;;
  esac

  log "编译 ${platform} (linux/${goarch}) v${VERSION}"
  out_bin="${WORK}/${platform}/waterfish-anthology"
  mkdir -p "$(dirname "${out_bin}")"
  (
    cd "${SRC}"
    CGO_ENABLED=0 GOOS=linux GOARCH="${goarch}" \
      go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" -o "${out_bin}" .
  )

  stage="${WORK}/stage-${platform}"
  log "组装 ${platform} 应用目录"
  cp -a "${PKG}/." "${stage}/"
  rm -f "${stage}/make-icons.py" "${stage}/icon.svg"
  find "${stage}" -name '.DS_Store' -delete 2>/dev/null || true
  mkdir -p "${stage}/app/server"
  cp "${out_bin}" "${stage}/app/server/waterfish-anthology"
  chmod +x "${stage}/app/server/waterfish-anthology"
  chmod +x "${stage}"/cmd/*

  # 写入版本号与目标架构
  sed -i "s/^version  *=.*/version               = ${VERSION}/" "${stage}/manifest"
  sed -i "s/^platform  *=.*/platform              = ${platform}/" "${stage}/manifest"

  log "打包 ${APPNAME}_${platform}.fpk"
  (
    cd "${DIST}"
    "${FNPACK}" build --directory "${stage}" >/dev/null
  ) || die "fnpack 打包失败（${platform}）"

  # fnpack 产物固定叫 <appname>.fpk，这里按架构重命名以区分
  mv -f "${DIST}/${APPNAME}.fpk" "${DIST}/${APPNAME}_${platform}.fpk"
done

log "完成，产物位于 dist/"
ls -lh "${DIST}" | sed '1d' | awk '{printf "   %-38s %s\n", $9, $5}'
