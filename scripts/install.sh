#!/usr/bin/env bash
set -euo pipefail

DEFAULT_REPO="sunliang711/sbox-manager"
REPO="${DEFAULT_REPO}"
VERSION="latest"
OS=""
ARCH=""
INSTALL_DIR="/usr/local/bin"
TMP_DIR=""
DRY_RUN=0
FORCE=0
NO_CHECKSUM=0

# 输出帮助信息，适用于用户直接执行脚本或传入 --help。
usage() {
    cat <<'EOF'
Usage:
  install.sh [--version vX.Y.Z] [--repo OWNER/REPO] [--install-dir DIR] [--dry-run] [--force]

Options:
  --version V              Release tag, default latest
  --repo OWNER/REPO        GitHub repository, default sunliang711/sbox-manager
  --os OS                  Override OS: linux or darwin
  --arch ARCH              Override arch: amd64 or arm64
  --install-dir DIR        Install directory, default /usr/local/bin
  --tmp-dir DIR            Temporary directory
  --dry-run                Print actions without writing files
  --force                  Overwrite existing binaries
  --no-checksum            Skip checksum verification
  -h, --help               Show help
EOF
}

# 输出流程日志，适用于安装过程定位，不包含敏感信息。
log_info() {
    printf 'INFO %s\n' "$*" >&2
}

# 输出错误并退出，适用于参数、依赖和安全校验失败。
die() {
    printf 'ERROR %s\n' "$*" >&2
    exit 1
}

# 校验依赖命令，适用于实际下载、解压和安装前置检查。
require_command() {
    local name="$1"
    command -v "$name" >/dev/null 2>&1 || die "Missing required command: $name"
}

# 解析命令行参数，适用于 install.sh 的单入口调用。
parse_args() {
    while [ "$#" -gt 0 ]; do
        case "$1" in
        --version)
            shift
            [ "$#" -gt 0 ] || die "Missing value for --version"
            VERSION="$1"
            ;;
        --repo)
            shift
            [ "$#" -gt 0 ] || die "Missing value for --repo"
            REPO="$1"
            ;;
        --os)
            shift
            [ "$#" -gt 0 ] || die "Missing value for --os"
            OS="$1"
            ;;
        --arch)
            shift
            [ "$#" -gt 0 ] || die "Missing value for --arch"
            ARCH="$1"
            ;;
        --install-dir)
            shift
            [ "$#" -gt 0 ] || die "Missing value for --install-dir"
            INSTALL_DIR="$1"
            ;;
        --tmp-dir)
            shift
            [ "$#" -gt 0 ] || die "Missing value for --tmp-dir"
            TMP_DIR="$1"
            ;;
        --dry-run)
            DRY_RUN=1
            ;;
        --force)
            FORCE=1
            ;;
        --no-checksum)
            NO_CHECKSUM=1
            ;;
        -h | --help)
            usage
            exit 0
            ;;
        *)
            die "Unknown option: $1"
            ;;
        esac
        shift
    done
}

# 探测操作系统，适用于未显式传入 --os 的安装。
detect_os() {
    if [ -n "$OS" ]; then
        return 0
    fi
    case "$(uname -s)" in
    Linux) OS="linux" ;;
    Darwin) OS="darwin" ;;
    *) die "Unsupported OS: $(uname -s)" ;;
    esac
}

# 探测 CPU 架构，适用于未显式传入 --arch 的安装。
detect_arch() {
    if [ -n "$ARCH" ]; then
        return 0
    fi
    case "$(uname -m)" in
    x86_64 | amd64) ARCH="amd64" ;;
    arm64 | aarch64) ARCH="arm64" ;;
    *) die "Unsupported arch: $(uname -m)" ;;
    esac
}

# 校验参数白名单，避免把异常值拼到下载 URL 和文件名中。
validate_inputs() {
    case "$REPO" in
    "" | /* | *..* | *\\* | *//* | *[!A-Za-z0-9._/-]*) die "Unsafe --repo: $REPO" ;;
    esac
    case "$REPO" in
    */*/*) die "--repo must be OWNER/REPO" ;;
    esac
    case "$REPO" in
    */*) ;;
    *) die "--repo must be OWNER/REPO" ;;
    esac
    validate_version
    case "$OS" in
    linux | darwin) ;;
    *) die "Unsupported --os: $OS" ;;
    esac
    case "$ARCH" in
    amd64 | arm64) ;;
    *) die "Unsupported --arch: $ARCH" ;;
    esac
    [ -n "$INSTALL_DIR" ] || die "--install-dir must not be empty"
}

# 校验版本字符串，避免路径分隔符或穿越片段进入资产名。
validate_version() {
    case "$VERSION" in
    "" | */* | *\\* | *..*) die "Unsafe --version: $VERSION" ;;
    esac
    case "$VERSION" in
    *[!A-Za-z0-9._-]*) die "Unsafe --version: $VERSION" ;;
    esac
    case "$VERSION" in
    latest | v*.*.*) ;;
    *) die "--version must be latest or a vX.Y.Z tag" ;;
    esac
}

# 解析 latest tag，适用于用户未指定具体版本的场景。
resolve_version() {
    if [ "$VERSION" != "latest" ]; then
        return 0
    fi
    require_command curl
    VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
    [ -n "$VERSION" ] || die "Failed to resolve latest release for ${REPO}"
}

# 创建并登记临时目录，适用于下载和解压 release 资产。
prepare_tmp_dir() {
    if [ -n "$TMP_DIR" ]; then
        local tmp_parent="${TMP_DIR%/}"
        [ -n "$tmp_parent" ] || die "--tmp-dir must not be /"
        [ "$tmp_parent" != "/" ] || die "--tmp-dir must not be /"
        mkdir -p "$tmp_parent"
        TMP_DIR="$(mktemp -d "${tmp_parent}/sbox-manager-install.XXXXXX")"
    else
        TMP_DIR="$(mktemp -d)"
    fi
    trap 'rm -rf "$TMP_DIR"' EXIT
}

# 下载单个 URL 到目标文件，适用于 release 资产和 checksum。
download_file() {
    local url="$1"
    local output="$2"
    log_info "Download: $url"
    curl -fL --proto '=https' --tlsv1.2 -o "$output" "$url"
}

# 校验 sha256，适用于安装前确认二进制包未被篡改。
verify_checksum() {
    local asset="$1"
    local checksums="$2"
    if [ "$NO_CHECKSUM" -eq 1 ]; then
        log_info "Checksum verification skipped"
        return 0
    fi
    if command -v sha256sum >/dev/null 2>&1; then
        (cd "$(dirname "$asset")" && grep "  $(basename "$asset")$" "$checksums" | sha256sum -c -)
    else
        local expected
        local actual
        expected="$(grep "  $(basename "$asset")$" "$checksums" | awk '{print $1}')"
        [ -n "$expected" ] || die "Checksum entry not found: $(basename "$asset")"
        actual="$(shasum -a 256 "$asset" | awk '{print $1}')"
        [ "$expected" = "$actual" ] || die "Checksum mismatch: $(basename "$asset")"
    fi
}

# 校验 tar 成员路径，适用于解压前阻止路径穿越和未知成员。
validate_archive_members() {
    local asset="$1"
    local pkg_prefix="$2"
    local member
    local has_sboxctl=0
    local has_sboxsub=0
    while IFS= read -r member; do
        case "$member" in
        /* | *..* | *\\*) die "Unsafe archive member: $member" ;;
        esac
        case "$member" in
        "$pkg_prefix/" | "$pkg_prefix/bin/" | "$pkg_prefix/bin/sboxctl" | "$pkg_prefix/bin/sboxsub" | "$pkg_prefix/README.md" | "$pkg_prefix/LICENSE" | "$pkg_prefix/templates/" | "$pkg_prefix/templates/"*) ;;
        *) die "Unknown archive member: $member" ;;
        esac
        case "$member" in
        "$pkg_prefix/bin/sboxctl") has_sboxctl=1 ;;
        "$pkg_prefix/bin/sboxsub") has_sboxsub=1 ;;
        esac
    done < <(tar -tzf "$asset")
    [ "$has_sboxctl" -eq 1 ] || die "sboxctl not found in archive"
    [ "$has_sboxsub" -eq 1 ] || die "sboxsub not found in archive"
}

# 安装单个二进制，适用于原子写入目标目录。
install_binary() {
    local source="$1"
    local name="$2"
    local target="${INSTALL_DIR}/${name}"
    local tmp_target="${INSTALL_DIR}/.${name}.tmp.$$"
    if [ "$DRY_RUN" -eq 1 ]; then
        log_info "Would install $target"
        return 0
    fi
    mkdir -p "$INSTALL_DIR"
    if [ -d "$target" ]; then
        die "Refuse to overwrite directory: $target"
    fi
    if [ -e "$target" ] && [ "$FORCE" -ne 1 ]; then
        die "Refuse to overwrite existing file without --force: $target"
    fi
    cp "$source" "$tmp_target"
    chmod 0755 "$tmp_target"
    mv -f "$tmp_target" "$target"
    log_info "Installed: $target"
}

# 执行安装后的轻量验证，适用于确认二进制可执行。
verify_installed_binaries() {
    if [ "$DRY_RUN" -eq 1 ]; then
        return 0
    fi
    "${INSTALL_DIR}/sboxctl" version >/dev/null
    "${INSTALL_DIR}/sboxsub" version >/dev/null
}

# 主流程，串联参数解析、下载、校验、解压和安装。
main() {
    parse_args "$@"
    detect_os
    detect_arch
    validate_inputs

    if [ "$DRY_RUN" -eq 1 ]; then
        local dry_version="$VERSION"
        if [ "$dry_version" = "latest" ]; then
            log_info "Would resolve latest release for ${REPO}"
        fi
        log_info "Would download https://github.com/${REPO}/releases/download/${dry_version}/sbox-manager_${dry_version}_${OS}_${ARCH}.tar.gz"
        log_info "Would install sboxctl and sboxsub to ${INSTALL_DIR}"
        exit 0
    fi

    resolve_version
    validate_version
    require_command curl
    require_command tar
    prepare_tmp_dir

    local pkg="sbox-manager_${VERSION}_${OS}_${ARCH}"
    local asset_name="${pkg}.tar.gz"
    local asset="${TMP_DIR}/${asset_name}"
    local checksums="${TMP_DIR}/checksums.txt"
    local base_url="https://github.com/${REPO}/releases/download/${VERSION}"

    download_file "${base_url}/${asset_name}" "$asset"
    if [ "$NO_CHECKSUM" -eq 1 ]; then
        log_info "Checksum verification skipped"
    else
        download_file "${base_url}/checksums.txt" "$checksums"
        verify_checksum "$asset" "$checksums"
    fi
    validate_archive_members "$asset" "$pkg"
    tar -xzf "$asset" -C "$TMP_DIR"
    install_binary "${TMP_DIR}/${pkg}/bin/sboxctl" sboxctl
    install_binary "${TMP_DIR}/${pkg}/bin/sboxsub" sboxsub
    verify_installed_binaries
}

main "$@"
