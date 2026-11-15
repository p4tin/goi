#!/bin/sh
# Prints an app's product version: info.version from apps/<app>/build/config.yml.
#   scripts/version.sh [app] [tag]
# app defaults to goi. With a tag (refs/tags/ optional) it fails unless the tag
# is that app's tag for this version: <app>-v<version>, or v<version> for Goi
# (the tags Goi used before there were several apps).
set -e
cd "$(dirname "$0")/.."

app=goi
if [ -n "$1" ] && [ -d "apps/$1" ]; then
  app=$1
  shift
fi
v=$(scripts/app-info.sh "$app" version)
if [ -z "$v" ]; then
  echo "no product version found in apps/$app/build/config.yml" >&2
  exit 1
fi

if [ -n "$1" ]; then
  tag="${1#refs/tags/}"
  if [ "$tag" != "$app-v$v" ] && { [ "$app" != goi ] || [ "$tag" != "v$v" ]; }; then
    echo "tag $tag does not match apps/$app/build/config.yml info.version $v" >&2
    exit 1
  fi
fi
echo "$v"
