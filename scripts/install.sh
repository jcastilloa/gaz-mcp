#!/usr/bin/env sh
set -eu

REPO="${REPO:-jcastilloa/gaz-mcp}"
SERVICE_NAME="${SERVICE_NAME:-gaz-mcp}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${VERSION:-}"

need_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "error: required command not found: $1" >&2
		exit 1
	fi
}

detect_os() {
	case "$(uname -s)" in
	Linux) echo "linux" ;;
	Darwin) echo "darwin" ;;
	*)
		echo "error: unsupported OS $(uname -s) (supported: Linux, Darwin)" >&2
		exit 1
		;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
	x86_64 | amd64) echo "amd64" ;;
	arm64 | aarch64) echo "arm64" ;;
	*)
		echo "error: unsupported architecture $(uname -m) (supported: amd64, arm64)" >&2
		exit 1
		;;
	esac
}

resolve_version() {
	if [ -n "$VERSION" ]; then
		echo "$VERSION"
		return
	fi

	api_url="https://api.github.com/repos/$REPO/releases/latest"
	response="$(curl -fsSL "$api_url")" || {
		echo "error: could not fetch latest release metadata from $api_url" >&2
		echo "hint: check network/GitHub rate limit or set VERSION manually" >&2
		exit 1
	}

	tag="$(printf '%s' "$response" | tr -d '\n' | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
	if [ -z "$tag" ]; then
		echo "error: could not resolve latest release tag from $api_url" >&2
		echo "hint: set VERSION manually, for example VERSION=vX.Y.Z" >&2
		exit 1
	fi

	echo "$tag"
}

install_binary() {
	resolved_version="$(resolve_version)"
	os="$(detect_os)"
	arch="$(detect_arch)"
	asset="${SERVICE_NAME}_${resolved_version}_${os}_${arch}.tar.gz"
	url="https://github.com/${REPO}/releases/download/${resolved_version}/${asset}"

	tmpdir="$(mktemp -d)"
	trap 'rm -rf "$tmpdir"' EXIT INT TERM

	archive_path="$tmpdir/$asset"
	echo "downloading $url"
	curl -fL "$url" -o "$archive_path"

	echo "extracting $asset"
	tar -xzf "$archive_path" -C "$tmpdir"

	binary_path="$(find "$tmpdir" -type f -name "$SERVICE_NAME" | head -n 1 || true)"
	if [ -z "$binary_path" ]; then
		echo "error: binary $SERVICE_NAME not found inside archive" >&2
		exit 1
	fi

	mkdir -p "$INSTALL_DIR"
	target="$INSTALL_DIR/$SERVICE_NAME"

	if [ -w "$INSTALL_DIR" ]; then
		install -m 0755 "$binary_path" "$target"
	else
		if command -v sudo >/dev/null 2>&1; then
			sudo install -m 0755 "$binary_path" "$target"
		else
			echo "error: cannot write to $INSTALL_DIR and sudo is not available" >&2
			exit 1
		fi
	fi

	echo "installed: $target"
	if ! echo ":$PATH:" | grep -q ":$INSTALL_DIR:"; then
		echo "warning: $INSTALL_DIR is not in PATH"
	fi

	"$target" version || true
}

need_cmd curl
need_cmd tar
need_cmd find
need_cmd install

install_binary
