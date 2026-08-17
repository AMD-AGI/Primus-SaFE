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
