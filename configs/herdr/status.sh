#!/bin/sh
# dots-managed Herdr tab-bar status: active CPU and GPU ratios plus RAM in GiB
# from one macmon sample. Herdr keeps only the last successful line, strips
# escape sequences, and clears the entry on empty output, failure, or timeout,
# so every missing or malformed input exits non-zero without printing.
set -u

command -v macmon >/dev/null 2>&1 || exit 1
payload=$(macmon pipe -s 1 2>/dev/null) || exit 1

printf '%s\n' "$payload" | awk '
function number(json, key,    prefix, value) {
  prefix = "\"" key "\"[[:space:]]*:[[:space:]]*"
  if (!match(json, prefix "[0-9]+(\\.[0-9]+)?([eE][-+]?[0-9]+)?[[:space:]]*[,}]")) return ""
  value = substr(json, RSTART, RLENGTH)
  sub(prefix, "", value)
  sub(/[[:space:]]*[,}]$/, "", value)
  return value
}
{ json = json $0 }
END {
  cpu = number(json, "cpu_active_ratio")
  gpu = number(json, "gpu_active_ratio")
  ram = number(json, "ram_usage")
  if (cpu == "" || gpu == "" || ram == "") exit 1
  printf "CPU %d%% GPU %d%% RAM %.1fGiB\n", cpu * 100 + 0.5, gpu * 100 + 0.5, ram / 1073741824
}'
