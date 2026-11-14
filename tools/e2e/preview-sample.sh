#!/bin/sh
# Creates the project and Goi profile used by preview.json:
#   tools/e2e/preview-sample.sh <dir>
# <dir>/proj has a README with a script tag, Mermaid diagrams (one invalid),
# a local image and a relative link; <dir>/home is an empty GOI_HOME.
set -e
d="${1:?usage: preview-sample.sh <dir>}"
rm -rf "$d"
mkdir -p "$d/proj/docs" "$d/proj/img" "$d/home/config"
printf 'module example.com/previewsample\n\ngo 1.22\n' > "$d/proj/go.mod"
# A 1x1 PNG.
printf 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==' | base64 --decode > "$d/proj/img/dot.png"
cat > "$d/proj/docs/other.md" <<'MD'
# Other

## Details

Linked from the README.
MD
cat > "$d/proj/README.md" <<'MD'
Intro paragraph with **bold** and a [relative link](docs/other.md#details).

<script>window.__xss = 1</script>

![a dot](img/dot.png) ![missing image](img/none.png)

```go
func main() {}
```

```mermaid
graph TD; A-->B
```

```mermaid
graph TD; A-->
```

After the diagrams.
MD
echo "sample ready: $d/proj (GOI_HOME=$d/home)"
