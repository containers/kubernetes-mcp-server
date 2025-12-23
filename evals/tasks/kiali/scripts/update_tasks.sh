#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
README_FILE="${1:-$ROOT_DIR/README.md}"
sanitize_section() {
  local start_mark="$1"
  local end_mark="$2"
  local infile="$3"
  local outfile="$4"
  awk -v start="$start_mark" -v end="$end_mark" '
    BEGIN { inblock=0 }
    {
      if ($0 == start) {
        print;
        inblock=1;
        next
      }
      if ($0 == end) {
        print;
        inblock=0;
        next
      }
      if (inblock==1) next;
      print
    }
  ' "$infile" > "$outfile"
}

TMP_TASKS_CONTENT="$(mktemp)"
trap 'rm -f "$TMP_TASKS_CONTENT"' EXIT
TMP_TASKS_RAW="$(mktemp)"
trap 'rm -f "$TMP_TASKS_RAW"' EXIT
# Build Tasks summary section (grouped by category, ordered by difficulty)
{
  TASKS_DIR="$ROOT_DIR/"
  if [[ ! -d "$TASKS_DIR" ]]; then
    echo "_No tasks directory found at tests/tasks._"
  else
    : > "$TMP_TASKS_RAW"
    for d in "$TASKS_DIR"/*; do
      [[ -d "$d" ]] || continue
      base="$(basename "$d")"
      shopt -s nullglob
      yaml_candidates=("$d"/*.yaml)
      shopt -u nullglob
      if [[ ${#yaml_candidates[@]} -eq 0 ]]; then
        continue
      fi
      yaml="${yaml_candidates[0]}"
      metaName="$(awk '
        /^[ \t]*metadata:/ {inm=1; next}
        /^[^ \t]/ && inm==1 {inm=0}
        inm && $1 == "name:" {
          $1=""; sub(/^[ \t]+/, "");
          gsub(/^"|"$/, "", $0);
          print; exit
        }
      ' "$yaml" 2>/dev/null || true)"
      category="$(awk '
        /^[ \t]*metadata:/ {inm=1; next}
        /^[^ \t]/ && inm==1 {inm=0}
        inm && $1 == "category:" {
          $1=""; sub(/^[ \t]+/, "");
          gsub(/^"|"$/, "", $0);
          print; exit
        }
      ' "$yaml" 2>/dev/null || true)"
      difficulty="$(awk '
        /^[ \t]*metadata:/ {inm=1; next}
        /^[^ \t]/ && inm==1 {inm=0}
        inm && $1 == "difficulty:" {
          $1=""; sub(/^[ \t]+/, "");
          gsub(/^"|"$/, "", $0);
          print; exit
        }
      ' "$yaml" 2>/dev/null || true)"
      if [[ -z "$difficulty" ]]; then
        difficulty="$(awk '
          /^[ \t]*difficulty:[ \t]*/ { sub(/^[ \t]*difficulty:[ \t]*/, ""); gsub(/^"|"$/, "", $0); print; exit }
        ' "$yaml" 2>/dev/null || true)"
      fi
      prompt="$(awk '
        /^[ \t]*prompt:/ {pin=1; next}
        pin && /^[^ \t]/ {pin=0}
        pin && /^[ \t]*inline:/ {
          sub(/^[ \t]*inline:[ \t]*/, "");
          print; exit
        }
      ' "$yaml" 2>/dev/null || true)"
      [[ -n "$metaName" ]] || metaName="$base"
      [[ -n "$prompt" ]] || prompt="(no prompt)"
      [[ -n "$category" ]] || category="Uncategorized"
      dnorm="$(echo "$difficulty" | tr '[:upper:]' '[:lower:]')"
      case "$dnorm" in
        easy) dorder=1 ;;
        medium) dorder=2 ;;
        hard) dorder=3 ;;
        *) dorder=9 ;;
      esac
      printf "%s\t%02d\t%s\t%s\t%s\t%s\n" "$category" "$dorder" "$dnorm" "$base" "$metaName" "$prompt" >> "$TMP_TASKS_RAW"
    done
    current_cat=""
    sort -t $'\t' -k1,1 -k2,2n -k4,4 "$TMP_TASKS_RAW" | while IFS=$'\t' read -r category dorder dlabel base metaName prompt; do
      if [[ "$category" != "$current_cat" ]]; then
        echo "- $category"
        current_cat="$category"
      fi
      echo "  - [${dlabel:-easy}] ${base} (${metaName})"
      echo "        **Prompt:** *${prompt}*"
    done
  fi
} > "$TMP_TASKS_CONTENT"

# Update Tasks section in README
TASKS_START="<!-- TASKS-START -->"
TASKS_END="<!-- TASKS-END -->"
if ! grep -q "$TASKS_START" "$README_FILE" || ! grep -q "$TASKS_END" "$README_FILE"; then
  echo "README tasks markers not found. Please ensure $TASKS_START and $TASKS_END exist."
  exit 1
fi
sanitize_section "$TASKS_START" "$TASKS_END" "$README_FILE" "$README_FILE.tmp"
mv "$README_FILE.tmp" "$README_FILE"
awk -v start="$TASKS_START" -v end="$TASKS_END" -v content_file="$TMP_TASKS_CONTENT" '
  BEGIN { inblock=0 }
  {
    if ($0 == start) {
      print;
      while ((getline line < content_file) > 0) print line;
      inblock=1;
      next
    }
    if ($0 == end) {
      print;
      inblock=0;
      next
    }
    if (inblock==1) next;
    print
  }
' "$README_FILE" > "$README_FILE.tmp"
mv "$README_FILE.tmp" "$README_FILE"
echo "Updated tasks section in $README_FILE"

