{{/*
The session a component's database driver must be given.

"read-write" makes the driver check, at connect time, that the session it opened
can write, and refuse it when it cannot -- so a connection reaching a replica
fails there instead of failing every write later with "cannot execute INSERT in a
read-only transaction". Every component reading this writes, so on is the right
default.

hasKey, not default: an explicitly empty value is how a deployment whose db host
resolves to a replica turns this off, and `default` reads empty as unset and would
put "read-write" back -- the escape hatch would render as the thing it escapes.
The Go side honours the empty because getString tests viper.IsSet rather than the
value.

Shared rather than repeated in each component's config, because three copies of a
condition this non-obvious drift apart.
*/}}
{{- define "primus-safe.dbTargetSessionAttrs" -}}
{{- if hasKey (default dict .Values.db) "target_session_attrs" -}}
{{ (default dict .Values.db).target_session_attrs | quote }}
{{- else -}}
"read-write"
{{- end -}}
{{- end -}}

{{/*
The Secret sandboxes receive environment variables from (sandbox.env_secret), as a string; ""
when unset. Not `default`: a value helm parses as a number or boolean (0, false) is still set,
and is refused by the sandbox template rather than read as unset.
*/}}
{{- define "primus-safe.sandboxEnvSecret" -}}
{{- $raw := (default dict .Values.sandbox).env_secret -}}
{{- if not (kindIs "invalid" $raw) -}}{{- toString $raw -}}{{- end -}}
{{- end -}}

{{/*
The users block of the apiserver and webhooks configuration: what every user holds when an
administrator has granted nothing. One definition for both, because the admission webhook (what
is stored) and the apiserver (what a request sees in external mode) must agree on it.
base_roles: an empty list is the default, not "no roles": a user cannot be created without one.
join_default_workspaces: hasKey rather than `default`, which treats false as absent.
*/}}
{{- define "primus-safe.usersConfig" -}}
{{- $u := default dict .Values.users -}}
base_roles: {{ toJson (default (list "default") $u.base_roles) }}
join_default_workspaces: {{ if hasKey $u "join_default_workspaces" }}{{ $u.join_default_workspaces }}{{ else }}true{{ end }}
{{- end -}}

{{/*
The secret that copy-opensearch-secret copies into the release namespace, or "" when that job is
not rendered. The hook Role secret-manager grants access to it by name.
*/}}
{{- define "primus-safe.openSearchCopySecret" -}}
{{- if and .Values.opensearch.enable (or (empty .Values.opensearch.username) (empty .Values.opensearch.password)) -}}
{{- .Values.opensearch.secret | default "" -}}
{{- end -}}
{{- end -}}

{{/*
The namespace copy-opensearch-secret reads from when it is not the release namespace, else "".
The hook RBAC then includes a Role secret-manager there.
*/}}
{{- define "primus-safe.openSearchCopySourceNamespace" -}}
{{- if include "primus-safe.openSearchCopySecret" . -}}
{{- $ns := .Values.opensearch.secret_namespace | default .Release.Namespace -}}
{{- if ne $ns .Release.Namespace -}}
{{- $ns -}}
{{- end -}}
{{- end -}}
{{- end -}}
