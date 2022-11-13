{{ define "packages" -}}

{{ range .packages -}}

{{- end -}}

{{ range .packages }}

# {{ .Title }}
{{ .GetComment }}

**Resource Types:**

  {{- if ne .GroupName "" -}}
    {{- range .VisibleTypes -}}
      {{- if .IsExported }}
- [{{ .DisplayName }}]({{ .Link }})
      {{- end -}}
    {{- end -}}
  {{ end -}}
  {{ if ne .GroupName "" -}}

    {{/* For package with a group name, list all type definitions in it. */}}
    {{ range .VisibleTypes }}
      {{- if or .IsExported -}}
{{ template "type" . }}
      {{- end -}}
    {{ end }}

## Definitions

This section contains definitions for objects used in the {{ .Title }} API.

    {{ range .VisibleTypes }}
      {{- if and .Referenced (not .IsExported) -}}
{{ template "type" . }}
      {{- end -}}
    {{ end }}
  {{ else }}
    {{/* For package w/o group name, list only types referenced. */}}
    {{- range .VisibleTypes -}}
      {{- if .Referenced -}}
{{ template "type" . }}
      {{- end -}}
    {{- end }}
  {{- end }}
{{- end }}
{{- end }}
