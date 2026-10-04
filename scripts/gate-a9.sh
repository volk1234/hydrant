#!/usr/bin/env bash
# Gate A9: valuesObject in the product must equal what Helm builds from
#   -f values.yaml -f values.{env}.yaml -f values.{cluster}.yaml -f <inline>
# Usage: scripts/gate-a9.sh <repo> <env> <out> [app-dir...]

set -euo pipefail

repo=$1; env=$2; out=$3; shift 3
prefix=${LABEL_PREFIX:-hydrant.sh/}
dump=$(cd "$(dirname "$0")/.." && pwd)/testdata/dumpchart
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT

apps=("$@")
if [ ${#apps[@]} -eq 0 ]; then
  mapfile -t apps < <(cd "$repo" && find cluster-init/apps -name "config-$env.yaml" -printf '%h\n' | sort)
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
      secret=$(grep -l "^  name: $cluster\$" "$repo/generated/clusters/$env"/*.yaml | head -1)
      sedexpr="s/\\[CLUSTER_NAME\\]/$cluster/g;"
      while IFS=$'\t' read -r key val; do
        tok=$(echo "${key#"$prefix"}" | tr '[:lower:]' '[:upper:]')
        sedexpr+="s|\\[$tok\\]|$val|g;"
      done < <(PREFIX="$prefix" yq -r '(.metadata.labels // {}) + (.metadata.annotations // {}) | to_entries[] | select(.key | test("^" + strenv(PREFIX))) | [.key, .value] | @tsv' "$secret")
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
