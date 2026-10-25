#!/usr/bin/env bash
# The Go scaffold has no pages yet. Probe its public HTTP boundary and every
# embedded asset referenced by the committed manifest until the cutover.
set -euo pipefail

base_url="${1:?usage: smoke-go-container.sh BASE_URL}"
base_url="${base_url%/}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

curl_args=(
	--fail --silent --show-error
	--connect-timeout 5 --max-time 20
	--retry 3 --retry-connrefused --retry-delay 1
	--cookie "$work_dir/cookies" --cookie-jar "$work_dir/cookies"
)

for probe in healthz readyz dependencyz; do
	curl "${curl_args[@]}" --output "$work_dir/body" "$base_url/$probe"
	test "$(cat "$work_dir/body")" = "OK"
done

status="$(curl "${curl_args[@]}" --dump-header "$work_dir/headers" --output /dev/null --write-out '%{http_code}' "$base_url/")"
test "$status" = 302
grep -Eiq '^location:[[:space:]]*/en-BE/[[:space:]]*$' "$work_dir/headers"

curl "${curl_args[@]}" --output /dev/null "$base_url/static/favicon.svg"
grep -oE '/static/[^" ,]+' "$repo_root/internal/assets/manifest_gen.go" | sort -u > "$work_dir/assets"
test -s "$work_dir/assets"

while IFS= read -r asset; do
	encoding=(--header 'Accept-Encoding: identity')
	compressed=false
	case "$asset" in
		*.css|*.js|*.svg)
			encoding=(--compressed --header 'Accept-Encoding: gzip')
			compressed=true
			;;
	esac
	status="$(curl "${curl_args[@]}" "${encoding[@]}" --dump-header "$work_dir/headers" --output "$work_dir/body" --write-out '%{http_code}' "$base_url$asset")"
	test "$status" = 200
	test -s "$work_dir/body"
	grep -Eiq '^cache-control:.*immutable' "$work_dir/headers"
	grep -Eiq '^x-content-type-options:[[:space:]]*nosniff' "$work_dir/headers"
	grep -Eiq '^vary:.*Accept-Encoding' "$work_dir/headers"
	if [[ "$compressed" == true ]]; then
		grep -Eiq '^content-encoding:[[:space:]]*gzip' "$work_dir/headers"
	fi
	case "$asset" in
		*.css) grep -Eiq '^content-type:[[:space:]]*text/css' "$work_dir/headers" ;;
		*.js) grep -Eiq '^content-type:.*javascript' "$work_dir/headers" ;;
		*.svg) grep -Eiq '^content-type:[[:space:]]*image/svg\+xml' "$work_dir/headers" ;;
		*.webp) grep -Eiq '^content-type:[[:space:]]*image/webp' "$work_dir/headers" ;;
		*.woff2|*.ttf) grep -Eiq '^content-type:[[:space:]]*font/' "$work_dir/headers" ;;
	esac
done < "$work_dir/assets"

video="$(grep -m1 '\.mp4$' "$work_dir/assets")"
status="$(curl "${curl_args[@]}" --range 0-15 --dump-header "$work_dir/headers" --output "$work_dir/body" --write-out '%{http_code}' "$base_url$video")"
test "$status" = 206
test "$(wc -c < "$work_dir/body" | tr -d ' ')" = 16
grep -Eiq '^content-range:[[:space:]]*bytes 0-15/' "$work_dir/headers"
printf 'Go container smoke passed (%s assets, compressed text and video ranges).\n' "$(wc -l < "$work_dir/assets" | tr -d ' ')"
