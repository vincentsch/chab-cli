#!/bin/sh
set -eu

# Install chab from GitHub release archives.
#
# Test-only seams:
#   CHAB_INSTALL_BASE_URL       default https://github.com
#   CHAB_INSTALL_OS             override detected OS token
#   CHAB_INSTALL_ARCH           override detected arch token
#   CHAB_INSTALL_DISABLE_TOOLS  space-separated subset of downloader tar checksum

REPO="vincentsch/chab-cli"
BASE_URL="${CHAB_INSTALL_BASE_URL:-https://github.com}"
VERSION="latest"
INSTALL_DIR="${HOME:-.}/.local/bin"

usage() {
	cat <<'EOF'
Usage: scripts/install.sh [--version <version|latest>] [--dir <install-dir>]

Options:
  --version <version|latest>  Release version to install, such as 0.6.0 or v0.6.0. Defaults to latest.
  --dir <install-dir>        Directory for the chab binary. Defaults to $HOME/.local/bin.
  --help                     Show this help.
EOF
}

die() {
	printf '%s\n' "$*" >&2
	exit 1
}

usage_error() {
	usage >&2
	exit 2
}

valid_segment() {
	case "$1" in
		""|*[!0123456789]*)
			return 1
			;;
		0)
			return 0
			;;
		0*)
			return 1
			;;
		*)
			return 0
			;;
	esac
}

normalize_version() {
	value=$1
	case "$value" in
		v*) value=${value#v} ;;
	esac

	old_ifs=$IFS
	IFS=.
	set -- $value
	IFS=$old_ifs
	if [ "$#" -ne 3 ]; then
		return 1
	fi
	if ! valid_segment "$1" || ! valid_segment "$2" || ! valid_segment "$3"; then
		return 1
	fi
	printf '%s.%s.%s\n' "$1" "$2" "$3"
}

is_disabled() {
	for disabled in ${CHAB_INSTALL_DISABLE_TOOLS:-}; do
		if [ "$disabled" = "$1" ]; then
			return 0
		fi
	done
	return 1
}

command_exists() {
	command -v "$1" >/dev/null 2>&1
}

detect_os() {
	if [ -n "${CHAB_INSTALL_OS:-}" ]; then
		printf '%s\n' "$CHAB_INSTALL_OS"
		return
	fi
	case "$(uname -s 2>/dev/null || printf unknown)" in
		Linux) printf '%s\n' linux ;;
		Darwin) printf '%s\n' darwin ;;
		MINGW*|MSYS*|CYGWIN*|Windows_NT) printf '%s\n' windows ;;
		*) printf '%s\n' unsupported ;;
	esac
}

detect_arch() {
	if [ -n "${CHAB_INSTALL_ARCH:-}" ]; then
		printf '%s\n' "$CHAB_INSTALL_ARCH"
		return
	fi
	case "$(uname -m 2>/dev/null || printf unknown)" in
		x86_64|amd64) printf '%s\n' amd64 ;;
		arm64|aarch64) printf '%s\n' arm64 ;;
		*) printf '%s\n' unsupported ;;
	esac
}

select_tools() {
	DOWNLOADER=""
	CHECKSUM_TOOL=""

	if ! is_disabled downloader; then
		if command_exists curl; then
			DOWNLOADER="curl"
		elif command_exists wget; then
			DOWNLOADER="wget"
		fi
	fi
	if [ -z "$DOWNLOADER" ]; then
		die "Missing downloader: install curl or wget before running this script."
	fi

	if is_disabled tar || ! command_exists tar; then
		die "Missing tar: install tar before running this script."
	fi

	if ! is_disabled checksum; then
		if command_exists sha256sum; then
			CHECKSUM_TOOL="sha256sum"
		elif command_exists shasum; then
			CHECKSUM_TOOL="shasum"
		fi
	fi
	if [ -z "$CHECKSUM_TOOL" ]; then
		die "Missing checksum tool: install sha256sum or shasum before running this script."
	fi
}

download_file() {
	url=$1
	dest=$2
	if [ "$DOWNLOADER" = "curl" ]; then
		curl -fsSL -o "$dest" "$url"
	else
		wget -q -O "$dest" "$url"
	fi
}

resolve_latest() {
	latest_url="$BASE_URL/$REPO/releases/latest"
	if [ "$DOWNLOADER" = "curl" ]; then
		effective=$(curl -fsSL -o /dev/null -w '%{url_effective}' "$latest_url")
	else
		headers=$(wget -S --spider "$latest_url" 2>&1 >/dev/null)
		# GNU wget appends " [following]" to the Location line it follows.
		# Keep the last redirect target, then strip that annotation below.
		effective=$(printf '%s\n' "$headers" | sed -n 's/^[	 ]*[Ll]ocation:[	 ]*//p' | sed -n '$p')
	fi
	case "$effective" in
		*/releases/tag/v*)
			tag=${effective##*/releases/tag/}
			;;
		*)
			die "Could not resolve latest chab release from $latest_url."
			;;
	esac
	tag=${tag%% *}
	norm=$(normalize_version "$tag") || die "Latest release tag is not a stable SemVer version: $tag"
	TAG="v$norm"
	NORM="$norm"
}

compute_sha256() {
	if [ "$CHECKSUM_TOOL" = "sha256sum" ]; then
		set -- $(sha256sum "$1")
	else
		set -- $(shasum -a 256 "$1")
	fi
	printf '%s\n' "$1"
}

extract_expected_sha256() {
	file=$1
	archive=$2
	expected=""
	while read -r sum name rest; do
		if [ "$name" = "$archive" ]; then
			expected=$sum
			break
		fi
	done < "$file"
	printf '%s\n' "$expected"
}

path_has_parent_reference() {
	case "$1" in
		..|../*|*/..|*/../*)
			return 0
			;;
	esac
	return 1
}

archive_member_is_safe_path() {
	member=$1
	case "$member" in
		""|/*)
			return 1
			;;
	esac
	if path_has_parent_reference "$member"; then
		return 1
	fi
	return 0
}

trim_trailing_slashes() {
	value=$1
	while [ "${value%/}" != "$value" ]; do
		value=${value%/}
	done
	printf '%s\n' "$value"
}

validate_archive() {
	archive=$1
	members_file=$2
	details_file=$3

	if ! tar -tzPf "$archive" > "$members_file"; then
		die "Could not inspect archive member names."
	fi
	if ! tar -tzvPf "$archive" > "$details_file"; then
		die "Could not inspect archive member types."
	fi

	top_level_binary_count=0
	binary_candidate_count=0
	while IFS= read -r member; do
		if ! archive_member_is_safe_path "$member"; then
			die "Archive contains unsafe member path: $member"
		fi
		trimmed=$(trim_trailing_slashes "$member")
		base=${trimmed##*/}
		if [ "$base" = "chab" ]; then
			binary_candidate_count=$((binary_candidate_count + 1))
		fi
		if [ "$member" = "chab" ]; then
			top_level_binary_count=$((top_level_binary_count + 1))
		fi
	done < "$members_file"

	regular_top_level_binary_count=0
	while IFS= read -r line; do
		case "$line" in
			l*)
				die "Archive contains a symlink member; refusing to install."
				;;
			h*)
				die "Archive contains a hardlink member; refusing to install."
				;;
			[-]*)
				;;
			*)
				die "Archive contains a non-regular member; refusing to install."
				;;
		esac
		case "$line" in
			[-]*" chab")
				regular_top_level_binary_count=$((regular_top_level_binary_count + 1))
				;;
		esac
	done < "$details_file"

	if [ "$top_level_binary_count" -ne 1 ] ||
		[ "$binary_candidate_count" -ne 1 ] ||
		[ "$regular_top_level_binary_count" -ne 1 ]; then
		die "Archive must contain exactly one regular top-level chab binary."
	fi
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--version)
			shift
			[ "$#" -gt 0 ] || usage_error
			VERSION=$1
			;;
		--dir)
			shift
			[ "$#" -gt 0 ] || usage_error
			INSTALL_DIR=$1
			;;
		--help)
			usage
			exit 0
			;;
		*)
			usage_error
			;;
	esac
	shift
done

if [ "$VERSION" != "latest" ]; then
	NORM=$(normalize_version "$VERSION") || usage_error
	TAG="v$NORM"
fi

OS=$(detect_os)
ARCH=$(detect_arch)
case "$OS" in
	linux|darwin)
		;;
	windows)
		die "This POSIX install script does not install Windows archives. See docs/install.md for manual Windows installation."
		;;
	*)
		die "Unsupported operating system for chab install script."
		;;
esac
case "$ARCH" in
	amd64|arm64)
		;;
	*)
		die "Unsupported architecture for chab install script."
		;;
esac

select_tools

if [ "$VERSION" = "latest" ]; then
	resolve_latest
fi

ARCHIVE="chab_${NORM}_${OS}_${ARCH}.tar.gz"
CHECKSUMS="chab_${NORM}_checksums.txt"
ARCHIVE_URL="$BASE_URL/$REPO/releases/download/$TAG/$ARCHIVE"
CHECKSUMS_URL="$BASE_URL/$REPO/releases/download/$TAG/$CHECKSUMS"

tmpdir=$(mktemp -d 2>/dev/null || mktemp -d -t chab-install)

archive_path="$tmpdir/$ARCHIVE"
checksums_path="$tmpdir/$CHECKSUMS"
extract_dir="$tmpdir/extract"
members_path="$tmpdir/members.txt"
details_path="$tmpdir/members.detail.txt"
install_tmp=""
cleanup() {
	if [ -n "$install_tmp" ]; then
		rm -f "$install_tmp"
	fi
	rm -rf "$tmpdir"
}
trap cleanup EXIT HUP INT TERM
mkdir -p "$extract_dir"

download_file "$ARCHIVE_URL" "$archive_path"
download_file "$CHECKSUMS_URL" "$checksums_path"

expected=$(extract_expected_sha256 "$checksums_path" "$ARCHIVE")
if [ -z "$expected" ]; then
	die "Checksum file does not list $ARCHIVE."
fi
actual=$(compute_sha256 "$archive_path")
if [ "$actual" != "$expected" ]; then
	die "Checksum verification failed for $ARCHIVE."
fi

validate_archive "$archive_path" "$members_path" "$details_path"
if ! tar -xzf "$archive_path" -C "$extract_dir" chab; then
	die "Could not extract chab binary from archive."
fi
binary="$extract_dir/chab"
if [ -L "$binary" ] || [ ! -f "$binary" ]; then
	die "Extracted chab member is not a regular file."
fi

if [ -e "$INSTALL_DIR" ] && [ ! -d "$INSTALL_DIR" ]; then
	die "Install target exists and is not a directory: $INSTALL_DIR"
fi
if ! mkdir -p "$INSTALL_DIR"; then
	die "Could not create install directory: $INSTALL_DIR"
fi
if [ ! -w "$INSTALL_DIR" ]; then
	die "Install directory is not writable: $INSTALL_DIR"
fi

target="$INSTALL_DIR/chab"
if [ -e "$target" ] && [ ! -f "$target" ]; then
	die "Install target exists and is not a regular file: $target"
fi
install_tmp=$(mktemp "$INSTALL_DIR/.chab.tmp.XXXXXX") || die "Could not create temporary install file in $INSTALL_DIR."
if ! cp "$binary" "$install_tmp"; then
	rm -f "$install_tmp"
	install_tmp=""
	die "Could not copy chab binary into $INSTALL_DIR."
fi
if ! chmod 755 "$install_tmp"; then
	rm -f "$install_tmp"
	install_tmp=""
	die "Could not set executable permissions on temporary chab binary."
fi
if ! mv -f "$install_tmp" "$target"; then
	rm -f "$install_tmp"
	install_tmp=""
	die "Could not install chab atomically into $target."
fi
install_tmp=""

printf 'Installed chab %s to %s/chab\n' "$NORM" "$INSTALL_DIR"
printf 'Make sure %s is on your PATH.\n' "$INSTALL_DIR"
printf 'When --version latest is used, GitHub selects the latest release redirect.\n'
