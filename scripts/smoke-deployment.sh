#!/usr/bin/env bash
set -euo pipefail

base_url="${1:?usage: smoke-deployment.sh BASE_URL}"
base_url="${base_url%/}"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

curl_args=(
	--fail
	--show-error
	--silent
	--connect-timeout 5
	--max-time 20
	--retry 3
	--retry-all-errors
	--retry-delay 2
	--header "Cache-Control: no-cache"
	--cookie "${work_dir}/cookies"
	--cookie-jar "${work_dir}/cookies"
)

curl "${curl_args[@]}" "${base_url}/readyz" > /dev/null

if [[ "${SMOKE_SKIP_DEPENDENCY:-false}" != "true" ]]; then
	curl "${curl_args[@]}" "${base_url}/dependencyz" > /dev/null
fi

for path in /en-BE/ /fr-BE/pricelist/; do
	name="$(printf '%s' "$path" | tr '/-' '__')"
	curl "${curl_args[@]}" \
		--dump-header "${work_dir}/${name}.headers" \
		--output "${work_dir}/${name}.body" \
		"${base_url}${path}"

	grep --fixed-strings --quiet "Aesthetic Lab" "${work_dir}/${name}.body"
	grep --extended-regexp --ignore-case --quiet \
		'^x-content-type-options:[[:space:]]*nosniff' \
		"${work_dir}/${name}.headers"
	# Qwik renders images under /assets/, the Go site under /static/. Until
	# Qwik is removed, this script checks both: the production canary for the
	# cutover release serves pages from either stack.
	grep --only-matching --extended-regexp \
		'/(assets|static)/[A-Za-z0-9._/-]+\.(webp|avif|png|jpe?g|svg)' \
		"${work_dir}/${name}.body" >> "${work_dir}/images" || true
done

# Validate every responsive image variant referenced by server-rendered HTML.
# The page and its images share the cookie jar, so session affinity keeps
# them on one revision. Client/server image deduplication must never produce
# URLs absent from the build.
test -s "${work_dir}/images"
sort -u "${work_dir}/images" > "${work_dir}/unique-images"
while IFS= read -r image_path; do
	# During mixed traffic only, the caller may supply the byte-verified baseline
	# of old HTML defects. Candidate and final smoke tests never use this list.
	if [[ -n "${SMOKE_REPAIRED_LEGACY_IMAGES:-}" ]] \
		&& grep -Fxq "$image_path" "$SMOKE_REPAIRED_LEGACY_IMAGES"; then
		continue
	fi
	if ! content_type=$(curl "${curl_args[@]}" --output /dev/null \
		--write-out '%{content_type}' "${base_url}${image_path}"); then
		echo "Rendered image request failed: $image_path" >&2
		exit 1
	fi
	if [[ "$content_type" != image/* ]]; then
		echo "Rendered image did not return image content: $image_path ($content_type)" >&2
		exit 1
	fi
done < "${work_dir}/unique-images"
