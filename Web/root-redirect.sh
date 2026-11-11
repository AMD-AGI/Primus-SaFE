#!/bin/bash
# Writes the nginx rule that redirects a browser opening the bare / of the console's host, to the file named by
# the first argument (nginx.conf includes it in its location for /). It is driven by ROOT_REDIRECT (chart:
# web.root_redirect), for example /portal/. Unset or empty, the file is empty and / serves the console.
#
# Only a GET or HEAD of / without a query string is redirected ($bare_root_request in nginx.conf): a sign-in comes
# back to / with one (an OAuth callback's ?code=...&state=..., an identity provider's ?error=...), and the console
# must load to finish it.
set -eu

out="${1:?usage: root-redirect.sh <output file>}"
target="${ROOT_REDIRECT:-}"

if [ -z "$target" ]; then
  : > "$out"
  exit 0
fi

# A path on this host, so the redirect cannot leave it: the leading / is followed by something other than a /
# (//host is another host, and / alone would redirect to itself). The value is written into nginx configuration, so
# it may hold nothing nginx reads as the end of the directive, a block, a quoted string, a variable or an escape;
# a \ is refused for that reason, and because a browser reads /\host as //host. Searched in the whole string (a
# line-by-line grep would miss a second line).
shape='^/[^/]'
unsafe='[[:space:][:cntrl:]"'\'';{}$\\]'
if ! [[ "$target" =~ $shape ]] || [[ "$target" =~ $unsafe ]]; then
  echo "ERROR: ROOT_REDIRECT must be a path on this host such as /portal/, without whitespace, quotes, ;, {, }, \$ or \\, got: $target" >&2
  exit 1
fi

# A browser resolves the . and .. segments of the path (%2e is a dot there too) before it follows the redirect, so
# /. or /portal/.. would land on / and redirect again.
path="${target%%[?#]*}"
dot_segment='(^|/)(\.|%2e){1,2}(/|$)'
if [[ "${path,,}" =~ $dot_segment ]]; then
  echo "ERROR: ROOT_REDIRECT must not have a . or .. segment, which a browser resolves (/portal/.. is /), got: $target" >&2
  exit 1
fi

cat > "$out" <<CONF
if (\$bare_root_request) { return 302 ${target}; }
CONF
