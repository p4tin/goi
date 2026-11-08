#!/bin/sh
# Creates the sample used by m4.json: tools/e2e/m4-sample.sh <dir>
# <dir>/work is a git repository (remote: <dir>/remote.git) with a branch
# "feature" that conflicts with main, a lint issue, an unstaged two-hunk
# change and an untracked file. <dir>/gitconfig isolates git from the
# user's configuration: run Goi with GIT_CONFIG_GLOBAL=<dir>/gitconfig.
set -e
d="${1:?usage: m4-sample.sh <dir>}"
rm -rf "$d"
mkdir -p "$d/work"
cat > "$d/gitconfig" <<'CFG'
[user]
	name = Goi Test
	email = goi@example.com
[init]
	defaultBranch = main
[commit]
	gpgsign = false
CFG
export GIT_CONFIG_GLOBAL="$d/gitconfig" GIT_CONFIG_NOSYSTEM=1
git init -q --bare "$d/remote.git"
cd "$d/work"
git init -q -b main
cat > go.mod <<'GO'
module example.com/m4

go 1.22
GO
cat > main.go <<'GO'
package main

import "fmt"

func main() {
	fmt.Println("one")
	fmt.Println("two")
	fmt.Println("three")
	fmt.Println("four")
	fmt.Println("five")
	fmt.Println("six")
	fmt.Println("seven")
	fmt.Println("eight")
	fmt.Println("nine")
	fmt.Println("ten")
	fmt.Println(greet("goi"))
}
GO
cat > util.go <<'GO'
package main

import "os"

func greet(name string) string {
	return "hello " + name
}

func cleanup() {
	os.Remove("tmp.txt")
}
GO
git add .
git commit -q -m "initial"
git remote add origin "$d/remote.git"
git push -q -u origin main
git checkout -q -b feature
sed -i.bak 's/"hello " + name/"hi " + name/' util.go && rm util.go.bak
git commit -q -am "feature: say hi"
git checkout -q main
sed -i.bak 's/"hello " + name/"hey " + name/' util.go && rm util.go.bak
git commit -q -am "main: say hey"
# Unstaged change with two separate hunks, and an untracked file.
sed -i.bak 's/"one"/"ONE"/; s/"ten"/"TEN"/' main.go && rm main.go.bak
echo "scratch notes" > notes.txt
echo "sample repository in $d/work (GIT_CONFIG_GLOBAL=$d/gitconfig)"
