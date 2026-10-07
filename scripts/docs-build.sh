#!/usr/bin/env bash
# docs-build.sh builds the documentation site into build/site.
#
# The script copies the brand fonts and the lime mark from internal/ui/static/brand/ into
# docs/site/assets/brand/, so the repository holds one copy of each file. It then runs
# mkdocs build --strict, which turns each warning, such as a link to a missing page, into
# a failure. Last, it fails when a built file names a host other than the site host,
# because the site makes no request to another host (FR-site-38).
#
# The script exits 0 when the build is clean, and 1 when it is not.
set -euo pipefail

# The script finds the repository root from its own path, so it runs from any directory
# and in a tree that holds no .git directory.
cd "$(dirname "${BASH_SOURCE[0]}")/.."

if ! command -v mkdocs >/dev/null 2>&1; then
	echo "docs-build: mkdocs is not on the path. Run: pip install -r docs/site/requirements.txt" >&2
	exit 1
fi

brand=internal/ui/static/brand
assets=docs/site/assets/brand

# The script removes the old copy first, so a renamed font file leaves no stale copy.
rm -rf "$assets"
mkdir -p "$assets/fonts"
cp "$brand"/fonts/* "$assets/fonts/"
cp "$brand/logo-lime.svg" "$assets/"

mkdocs build --strict

site=build/site
site_host=$(sed -n 's|^site_url: *https\{0,1\}://\([^/]*\).*|\1|p' mkdocs.yml)
if [ -z "$site_host" ]; then
	echo "docs-build: mkdocs.yml holds no site_url, so the host check has no site host" >&2
	exit 1
fi

status=0

# theme.font: false stops the Google Fonts request. A reference that remains shows a
# changed theme or a page that loads a font of its own.
while IFS= read -r file; do
	[ -n "$file" ] || continue
	echo "docs-build: $file names fonts.googleapis.com"
	status=1
done < <(grep -rl 'fonts.googleapis.com' "$site" || true)

# A resource element that names an absolute URL makes the browser request that host. An
# anchor is not a resource, so the check reads no <a> element.
while IFS= read -r hit; do
	[ -n "$hit" ] || continue
	file=${hit%%:*}
	element=${hit#*:}
	url=$(printf '%s' "$element" | grep -oE '(src|href)="(https?:)?//[^"]+"' | head -n 1 | sed -E 's/^(src|href)="//; s/"$//')
	[ -n "$url" ] || continue
	host=$(printf '%s' "$url" | sed -E 's|^(https?:)?//||; s|[/:?#].*$||')
	if [ "$host" != "$site_host" ]; then
		echo "docs-build: $file requests $url from another host"
		status=1
	fi
done < <(grep -roE --include='*.html' '<(script|link|img|iframe|source|video|audio)[^>]*(src|href)="(https?:)?//[^"]+"[^>]*>' "$site" || true)

if [ "$status" -ne 0 ]; then
	echo "docs-build: the site requests a resource from another host" >&2
fi

exit "$status"
