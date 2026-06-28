#!/usr/bin/env bash
set -euo pipefail

SOURCE=""
INSTALL_DIR="/usr/local/bin"
DRY_RUN=0
FORCE=0
WORK_DIR=""

# 输出帮助信息，适用于本地安装脚本直接调用。
usage() {
    cat <<'EOF'
Usage:
  install-local.sh --from PATH [--install-dir DIR] [--dry-run] [--force]

Options:
  --from PATH          Source tar.gz package or directory containing sboxctl and sboxsub
  --install-dir DIR    Install directory, default /usr/local/bin
  --dry-run            Print actions without writing files
  --force              Overwrite existing binaries
  -h, --help           Show help
EOF
}

# 输出流程日志，适用于本地安装过程定位。
log_info() {
    printf 'INFO %s\n' "$*" >&2
}

# 输出错误并退出，适用于参数、路径和安全校验失败。
die() {
    printf 'ERROR %s\n' "$*" >&2
    exit 1
}

# 解析参数，适用于 install-local.sh 的单入口调用。
parse_args() {
    while [ "$#" -gt 0 ]; do
        case "$1" in
        --from)
            shift
            [ "$#" -gt 0 ] || die "Missing value for --from"
            SOURCE="$1"
            ;;
        --install-dir)
            shift
            [ "$#" -gt 0 ] || die "Missing value for --install-dir"
            INSTALL_DIR="$1"
            ;;
        --dry-run)
            DRY_RUN=1
            ;;
        --force)
            FORCE=1
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

# 校验输入路径和安装目录，适用于任何写入前置检查。
validate_inputs() {
    [ -n "$SOURCE" ] || die "--from is required"
    [ -e "$SOURCE" ] || die "Source not found: $SOURCE"
    [ -n "$INSTALL_DIR" ] || die "--install-dir must not be empty"
}

# 创建临时目录，适用于解压本地 release 包。
prepare_work_dir() {
    WORK_DIR="$(mktemp -d)"
    trap 'rm -rf "$WORK_DIR"' EXIT
}

# 输出 dry-run 安装目标，适用于无需解压即可预览的场景。
print_install_plan() {
    log_info "Would install ${INSTALL_DIR}/sboxctl"
    log_info "Would install ${INSTALL_DIR}/sboxsub"
}

# 校验 tar 成员路径，适用于解压前阻止路径穿越和未知成员。
validate_archive_members() {
    local asset="$1"
    local member
    local has_sboxctl=0
    local has_sboxsub=0
    while IFS= read -r member; do
        case "$member" in
        /* | *..* | *\\*) die "Unsafe archive member: $member" ;;
        esac
        if [[ "$member" == */ ]]; then
            continue
        fi
        case "$member" in
        */bin/sboxctl) has_sboxctl=1 ;;
        */bin/sboxsub) has_sboxsub=1 ;;
        */README.md | */LICENSE | */templates/*) ;;
        *) die "Unknown archive member: $member" ;;
        esac
    done < <(tar -tzf "$asset")
    [ "$has_sboxctl" -eq 1 ] || die "sboxctl not found in archive"
    [ "$has_sboxsub" -eq 1 ] || die "sboxsub not found in archive"
}

# 从目录解析二进制路径，适用于 dist/bin 或解压后的 release 根目录。
find_binary_dir() {
    local root="$1"
    if [ -x "${root}/sboxctl" ] && [ -x "${root}/sboxsub" ]; then
        printf '%s\n' "$root"
        return 0
    fi
    if [ -x "${root}/bin/sboxctl" ] && [ -x "${root}/bin/sboxsub" ]; then
        printf '%s\n' "${root}/bin"
        return 0
    fi
    local match
    match="$(find "$root" -path '*/bin/sboxctl' -type f -perm -111 | head -n 1 || true)"
    [ -n "$match" ] || die "sboxctl not found under: $root"
    local dir
    dir="$(dirname "$match")"
    [ -x "${dir}/sboxsub" ] || die "sboxsub not found beside: $match"
    printf '%s\n' "$dir"
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

# 主流程，串联本地源解析、归档校验、解压和安装。
main() {
    parse_args "$@"
    validate_inputs

    local source_dir=""
    if [ -d "$SOURCE" ]; then
        source_dir="$(find_binary_dir "$SOURCE")"
    else
        validate_archive_members "$SOURCE"
        if [ "$DRY_RUN" -eq 1 ]; then
            print_install_plan
            return 0
        fi
        prepare_work_dir
        tar -xzf "$SOURCE" -C "$WORK_DIR"
        source_dir="$(find_binary_dir "$WORK_DIR")"
    fi

    install_binary "${source_dir}/sboxctl" sboxctl
    install_binary "${source_dir}/sboxsub" sboxsub
}

main "$@"
