#!/usr/bin/env bash
# Gate A9: valuesObject in the product must equal what Helm builds from
#   -f values.yaml -f values.{env}.yaml -f values.{cluster}.yaml -f <inline>
# Tokens in the inline are substituted here independently (yq + sed over the
# cluster secret), not by the renderer, so the gate never checks the renderer
# against itself. labelPrefix, apps and clusters come from <repo>/hydrant.yaml,
# the same file the renderer reads; there are no defaults.
# Usage: scripts/gate-a9.sh <repo> <env> <out> [app-dir...]

set -euo pipefail

repo=$1; env=$2; out=$3; shift 3
settings="$repo/hydrant.yaml"
[ -f "$settings" ] || { echo "gate: $settings not found" >&2; exit 1; }
prefix=$(yq -r '.labelPrefix // ""' "$settings")
appsdir=$(yq -r '.apps // ""' "$settings")
clustersdir=$(yq -r '.clusters // ""' "$settings")
for v in prefix appsdir clustersdir; do
  [ -n "${!v}" ] || { echo "gate: $v is empty in $settings" >&2; exit 1; }
done
dump=$(cd "$(dirname "$0")/.." && pwd)/testdata/dumpchart
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT

apps=("$@")
if [ ${#apps[@]} -eq 0 ]; then
  mapfile -t apps < <(cd "$repo" && find "$appsdir" -name "config-$env.yaml" -printf '%h\n' | sort)
fi

fail=0; checked=0
for dir in "${apps[@]}"; do
  app=$(basename "$dir")
  cfg="$repo/$dir/config-$env.yaml"
  for product in "$out/$env"/*/"$app.yaml"; do
    [ -f "$product" ] || continue
    cluster=$(basename "$(dirname "$product")")

    left=()
    for f in "$repo/$dir/values.yaml" "$repo/$dir/values.$env.yaml" "$repo/$dir/values.$cluster.yaml"; do
      [ -f "$f" ] && left+=(-f "$f")
    done

    inline=$(yq '.inline_values // ""' "$cfg")
    if [ -n "$inline" ]; then
      # Secret keeps the name in stringData, ExternalSecret in its template (R4.25).
      secret=""
      for f in "$repo/$clustersdir/$env"/*.yaml; do
        if [ "$(yq -r '.stringData.name // .spec.target.template.data.name // ""' "$f")" = "$cluster" ]; then
          secret=$f; break
        fi
      done
      [ -n "$secret" ] || { echo "gate: no cluster file with name $cluster" >&2; exit 1; }
      meta='.metadata'
      [ "$(yq -r '.kind' "$secret")" = "ExternalSecret" ] && meta='.spec.target.template.metadata'
      sedexpr="s/\\[CLUSTER_NAME\\]/$cluster/g;"
      while IFS=$'\t' read -r key val; do
        tok=$(echo "${key#"$prefix"}" | tr '[:lower:]' '[:upper:]')
        # Token names may contain "." (R8.1): literal in the pattern, not "any char".
        tok=${tok//./\\.}
        # In the replacement "&", "\\" and the "|" delimiter are special.
        val=$(printf '%s' "$val" | sed -e 's/[\\&|]/\\&/g')
        sedexpr+="s|\\[$tok\\]|$val|g;"
      done < <(PREFIX="$prefix" yq -r "($meta.labels // {}) + ($meta.annotations // {}) | to_entries[] | select(.key | test(\"^\" + strenv(PREFIX))) | [.key, .value] | @tsv" "$secret")
      printf '%s\n' "$inline" | sed "$sedexpr" > "$tmp/inline.yaml"
      left+=(-f "$tmp/inline.yaml")
    fi

    yq '.spec.source.helm.valuesObject // {}' "$product" > "$tmp/merged.yaml"
    helm template dump "$dump" "${left[@]}" > "$tmp/a.yaml"
    helm template dump "$dump" -f "$tmp/merged.yaml" > "$tmp/b.yaml"
    checked=$((checked + 1))
    if diff -u "$tmp/a.yaml" "$tmp/b.yaml" > "$tmp/diff.txt"; then
      echo "ok   $app @ $cluster"
    else
      echo "FAIL $app @ $cluster"; cat "$tmp/diff.txt"; fail=1
    fi
  done
done

if [ "$checked" -eq 0 ]; then
  echo "gate: no objects checked — is $out/$env empty?" >&2
  exit 1
fi
[ "$fail" -eq 0 ] && echo "gate: OK, $checked object(s)"
exit $fail
