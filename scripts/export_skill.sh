#!/usr/bin/env bash
# Build and export the versioned, Go-only compare-figure Skill.
# The project root comes from this file, so builds do not depend on caller CWD.
set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
root=$(CDPATH= cd -- "${script_dir}/.." && pwd)
version=''
go_bin="${GO_BIN:-}"
platforms='linux/amd64,linux/arm64,windows/amd64,darwin/amd64,darwin/arm64'
out_dir="${root}/dist"
out_explicit=0
skip_build=0

usage() {
    cat <<'EOF'
Usage: ./scripts/export_skill.sh --version VERSION [OPTIONS]

Options:
  --version VERSION   strict semver x.y.z[-prerelease][+build], optional v
  --go PATH           Go executable (otherwise GO_BIN, then PATH lookup)
  --platforms LIST    comma-separated GOOS/GOARCH targets
  --out-dir DIR       destination directory (default: project-root/dist)
  --skip-build        use existing build/compare-figure and selected platform binaries
  -h, --help          show this help
EOF
}
fail() { printf 'export-skill: %s\n' "$*" >&2; exit 2; }

while (($#)); do
    case "$1" in
        --version) (($# >= 2)) || fail '--version requires a value'; version=$2; shift 2 ;;
        --go) (($# >= 2)) || fail '--go requires a path'; go_bin=$2; shift 2 ;;
        --platforms) (($# >= 2)) || fail '--platforms requires a value'; platforms=$2; shift 2 ;;
        --out-dir) (($# >= 2)) || fail '--out-dir requires a path'; out_dir=$2; out_explicit=1; shift 2 ;;
        --skip-build) skip_build=1; shift ;;
        -h|--help) usage; exit 0 ;;
        --) shift; (($# == 0)) || fail 'unexpected positional arguments'; break ;;
        -*) fail "unknown option: $1" ;;
        *) fail "unexpected positional argument: $1" ;;
    esac
done

if [[ -z "$version" ]]; then
    [[ -f "${root}/VERSION" ]] || fail 'missing --version and root VERSION file'
    version=$(tr -d '[:space:]' < "${root}/VERSION")
fi
[[ "$version" =~ ^v ]] && version=${version#v}
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]] || fail "invalid version: $version"
if [[ "$version" == *-* ]]; then
    prerelease=${version#*-}; prerelease=${prerelease%%+*}
    IFS='.' read -r -a prerelease_ids <<< "$prerelease"
    for id in "${prerelease_ids[@]}"; do
        if [[ "$id" =~ ^[0-9]+$ && "$id" =~ ^0[0-9]+$ ]]; then
            fail "invalid version: numeric prerelease identifier has leading zero: $id"
        fi
    done
fi
[[ -n "$platforms" ]] || fail 'platform list must not be empty'
[[ -n "$out_dir" ]] || fail 'output directory must not be empty'

# Relative explicit output is relative to caller CWD; the default is root/dist.
if [[ "$out_explicit" -eq 0 ]]; then
    out_dir="${root}/dist"
fi
mkdir -p -- "$out_dir"
out_parent=$(CDPATH= cd -- "$(dirname -- "$out_dir")" && pwd)
out_abs="${out_parent}/$(basename -- "$out_dir")"
final_name="compare-figure-skill-v${version}"
final_dir="${out_abs}/${final_name}"

IFS=',' read -r -a platform_items <<< "$platforms"
platform_args=()
for platform in "${platform_items[@]}"; do
    [[ "$platform" =~ ^[a-z0-9]+/[a-z0-9]+$ ]] || fail "invalid platform: $platform"
    platform_args+=("$platform")
done

build_stage=''; export_stage=''; package_parent=''; zip_tmp_dir=''
safe_cleanup_dir() {
    local target=${1:?} parent=${2:?} base=${3:?}
    [[ "$target" == "$parent/$base" ]] || return 0
    [[ "$base" == compare-figure-export.* || "$base" == .compare-figure-export.* ]] || return 0
    [[ -d "$target" && ! -L "$target" ]] || return 0
    local real_parent real_target
    real_parent=$(CDPATH= cd -P -- "$parent" && pwd) || return 0
    real_target=$(CDPATH= cd -P -- "$target" && pwd) || return 0
    [[ "$real_target" == "$real_parent/$base" ]] || return 0
    rm -rf -- "${target:?}"
}
cleanup() {
    [[ -n "$zip_tmp_dir" ]] && safe_cleanup_dir "$zip_tmp_dir" "$(dirname -- "$zip_tmp_dir")" "$(basename -- "$zip_tmp_dir")"
    [[ -n "$package_parent" ]] && safe_cleanup_dir "$package_parent" "$(dirname -- "$package_parent")" "$(basename -- "$package_parent")"
    [[ -n "$export_stage" ]] && safe_cleanup_dir "$export_stage" "$(dirname -- "$export_stage")" "$(basename -- "$export_stage")"
    [[ -n "$build_stage" ]] && safe_cleanup_dir "$build_stage" "$(dirname -- "$build_stage")" "$(basename -- "$build_stage")"
}
trap 'exit 130' INT TERM
trap cleanup EXIT

mkdir -p -- "${root}/build"
build_stage=$(mktemp -d "${root}/build/compare-figure-export.XXXXXX")
if [[ "$skip_build" -eq 0 ]]; then
    [[ -n "$go_bin" ]] || go_bin='go'
    command -v -- "$go_bin" >/dev/null 2>&1 || fail "Go executable not found: $go_bin"
    mkdir -p -- "${root}/build/go-cache"
    (cd -- "${root}/go" && CGO_ENABLED=0 GOCACHE="${root}/build/go-cache" "$go_bin" build -trimpath -ldflags='-s -w -buildid=' -o "${root}/build/compare-figure" ./cmd/compare-figure)
    chmod +x -- "${root}/build/compare-figure"
    CGO_ENABLED=0 GOCACHE="${root}/build/go-cache" "${root}/build/compare-figure" build --resource-root "$root" --go "$go_bin" --out "$build_stage" --platforms "$platforms"
else
    [[ -x "${root}/build/compare-figure" ]] || fail "missing executable: ${root}/build/compare-figure"
    for platform in "${platform_items[@]}"; do
        target=${platform//\//-}; name=compare-figure
        [[ "$platform" == windows/* ]] && name+=.exe
        source="${root}/build/platforms/${target}/${name}"
        [[ -f "$source" ]] || fail "missing requested platform binary: $source"
        mkdir -p -- "$build_stage/$target"
        cp -p -- "$source" "$build_stage/$target/$name"
    done
fi

export_stage=$(mktemp -d "${TMPDIR:-/tmp}/compare-figure-export.XXXXXX")
"${root}/build/compare-figure" export-skill --resource-root "$root" --binaries-dir "$build_stage" --out "$export_stage" --version "$version"

# Existing same-version directories may be reused, but unknown files are never packaged.
if [[ -L "$final_dir" ]];then fail "symlink export destination: $final_dir";fi
if [[ -e "$final_dir" && ! -d "$final_dir" ]]; then fail "destination is not a directory: $final_dir"; fi
if [[ -d "$final_dir" ]]; then
    while IFS= read -r -d '' path; do
        [[ -L "$path" ]] && fail "symlink in existing export: $path"
        rel=${path#"$final_dir/"}
        [[ -f "$export_stage/$rel" ]] || fail "unknown existing export file (refusing to delete/package): $rel"
    done < <(find "$final_dir" \( -type f -o -type l \) -print0)
fi
mkdir -p -- "$final_dir"
cp -a -- "${export_stage}/." "$final_dir/"

command -v zip >/dev/null 2>&1 || fail 'zip command is required'
package_parent=$(mktemp -d "${root}/build/compare-figure-export.XXXXXX")
package_dir="$package_parent/$final_name"
mkdir -p -- "$package_dir"
cp -a -- "${export_stage}/." "$package_dir/"
zip_tmp_dir=$(mktemp -d "${out_abs}/.compare-figure-export.XXXXXX")
zip_tmp="$zip_tmp_dir/archive.zip"
zip_path="$out_abs/$final_name.zip"
sha_path="$zip_path.sha256"
(
    cd -- "$package_parent"
    zip -q -r "$zip_tmp" "$final_name"
)
[[ -s "$zip_tmp" ]] || fail 'ZIP creation produced no archive'
mv -f -- "$zip_tmp" "$zip_path"
if command -v sha256sum >/dev/null 2>&1; then digest=$(sha256sum "$zip_path" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then digest=$(shasum -a 256 "$zip_path" | awk '{print $1}')
else fail 'sha256sum or shasum command is required'; fi
[[ "$digest" =~ ^[0-9a-fA-F]{64}$ ]] || fail 'invalid SHA-256 output'
printf '%s  %s\n' "$digest" "$(basename -- "$zip_path")" > "$sha_path"
printf 'exported %s\narchive %s\nsha256 %s\n' "$final_dir" "$zip_path" "$sha_path"
