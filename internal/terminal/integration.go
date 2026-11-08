package terminal

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func timeAfter() <-chan time.Time { return time.After(500 * time.Millisecond) }

// goCommands are `go` subcommands with their common flags, for completion.
var goCommands = map[string][]string{
	"build":     buildFlags("-o"),
	"clean":     {"-i", "-n", "-r", "-x", "-cache", "-testcache", "-modcache", "-fuzzcache"},
	"doc":       {"-all", "-c", "-cmd", "-short", "-src", "-u", "-http"},
	"env":       {"-json", "-u", "-w", "-changed"},
	"fix":       {"-diff"},
	"fmt":       {"-n", "-x", "-mod"},
	"generate":  {"-run", "-skip", "-n", "-v", "-x"},
	"get":       {"-t", "-u", "-tool", "-x"},
	"help":      {},
	"install":   buildFlags(),
	"list":      append(buildFlags(), "-f", "-json", "-m", "-u", "-versions", "-deps", "-test", "-e", "-export", "-find", "-compiled", "-retracted", "-reuse"),
	"mod":       {"download", "edit", "graph", "init", "tidy", "vendor", "verify", "why"},
	"run":       append(buildFlags(), "-exec"),
	"telemetry": {"on", "off", "local"},
	"test":      append(buildFlags("-o", "-c", "-json", "-exec"), "-run", "-skip", "-bench", "-benchmem", "-benchtime", "-count", "-cover", "-covermode", "-coverprofile", "-cpu", "-cpuprofile", "-memprofile", "-failfast", "-fuzz", "-fuzztime", "-list", "-parallel", "-short", "-shuffle", "-timeout", "-vet", "-fullpath"),
	"tool":      {"-n", "-modfile"},
	"version":   {"-m", "-v", "-json"},
	"vet":       append(buildFlags(), "-vettool", "-json"),
	"work":      {"edit", "init", "sync", "use", "vendor"},
	"bug":       {},
}

func buildFlags(extra ...string) []string {
	return append([]string{"-a", "-n", "-p", "-race", "-msan", "-asan", "-cover", "-coverpkg", "-v", "-work", "-x",
		"-asmflags", "-buildmode", "-buildvcs", "-compiler", "-gccgoflags", "-gcflags", "-installsuffix", "-ldflags",
		"-linkshared", "-mod", "-modcacherw", "-modfile", "-overlay", "-pgo", "-pkgdir", "-tags", "-toolexec", "-trimpath"}, extra...)
}

func sortedCommands() []string {
	out := make([]string, 0, len(goCommands))
	for k := range goCommands {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// zshCompletion is the zsh completion function for go.
func zshCompletion() string {
	var b strings.Builder
	b.WriteString("_goi_go() {\n  if (( CURRENT == 2 )); then\n    compadd -- " + strings.Join(sortedCommands(), " ") + "\n    return\n  fi\n  local -a opts\n  case ${words[2]} in\n")
	for _, c := range sortedCommands() {
		if len(goCommands[c]) == 0 {
			continue
		}
		fmt.Fprintf(&b, "    %s) opts=(%s) ;;\n", c, strings.Join(goCommands[c], " "))
	}
	b.WriteString("  esac\n  if [[ $PREFIX == -* || ${words[2]} == (mod|work|telemetry) ]] && (( ${#opts} )); then\n    compadd -- $opts\n  else\n    _files\n  fi\n}\n")
	return b.String()
}

// bashCompletion is the bash completion function for go.
func bashCompletion() string {
	var b strings.Builder
	b.WriteString("_goi_go() {\n  local cur=${COMP_WORDS[COMP_CWORD]}\n  if [ \"$COMP_CWORD\" -eq 1 ]; then\n    COMPREPLY=($(compgen -W \"" + strings.Join(sortedCommands(), " ") + "\" -- \"$cur\"))\n    return\n  fi\n  local opts=\"\"\n  case ${COMP_WORDS[1]} in\n")
	for _, c := range sortedCommands() {
		if len(goCommands[c]) == 0 {
			continue
		}
		fmt.Fprintf(&b, "    %s) opts=\"%s\" ;;\n", c, strings.Join(goCommands[c], " "))
	}
	b.WriteString("  esac\n  case ${COMP_WORDS[1]} in mod|work|telemetry) [ \"$COMP_CWORD\" -eq 2 ] && { COMPREPLY=($(compgen -W \"$opts\" -- \"$cur\")); return; } ;; esac\n  if [[ $cur == -* ]]; then\n    COMPREPLY=($(compgen -W \"$opts\" -- \"$cur\"))\n  else\n    COMPREPLY=($(compgen -f -- \"$cur\"))\n  fi\n}\ncomplete -o default -o filenames -F _goi_go go\n")
	return b.String()
}

// zsh startup files: each sources the user's file with ZDOTDIR pointing at
// the user's directory, then restores Goi's so zsh keeps reading our files.
func zshStartup(name string) string {
	return fmt.Sprintf(`# Goi shell integration (generated)
_goi_zd="$ZDOTDIR"
ZDOTDIR="${GOI_USER_ZDOTDIR:-$HOME}"
[[ -f "$ZDOTDIR/%[1]s" ]] && source "$ZDOTDIR/%[1]s"
GOI_USER_ZDOTDIR="$ZDOTDIR"
ZDOTDIR="$_goi_zd"
unset _goi_zd
`, name)
}

func zshRC() string {
	return `# Goi shell integration (generated)
ZDOTDIR="${GOI_USER_ZDOTDIR:-$HOME}"
[[ -f "$ZDOTDIR/.zshrc" ]] && source "$ZDOTDIR/.zshrc"
_goi_osc7() { printf '\e]7;file://%s%s\a' "${HOST}" "${PWD// /%20}" }
autoload -Uz add-zsh-hook 2>/dev/null && add-zsh-hook precmd _goi_osc7 || precmd_functions+=(_goi_osc7)
_goi_osc7
if ! (( $+functions[compdef] )); then
  autoload -Uz compinit && compinit -i -d "${XDG_CACHE_HOME:-$HOME/.cache}/goi-zcompdump"
fi
` + zshCompletion() + `(( $+functions[compdef] )) && compdef _goi_go go
`
}

func bashRC() string {
	return `# Goi shell integration (generated)
# Emulate a login shell: profile files usually source ~/.bashrc themselves.
if [ -f "$HOME/.bash_profile" ]; then . "$HOME/.bash_profile"
elif [ -f "$HOME/.bash_login" ]; then . "$HOME/.bash_login"
elif [ -f "$HOME/.profile" ]; then . "$HOME/.profile"
elif [ -f "$HOME/.bashrc" ]; then . "$HOME/.bashrc"
fi
__goi_osc7() { printf '\e]7;file://%s%s\a' "$HOSTNAME" "${PWD// /%20}"; }
case ";${PROMPT_COMMAND};" in *";__goi_osc7;"*) ;; *) PROMPT_COMMAND="__goi_osc7${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;; esac
` + bashCompletion()
}

// Integration prepares shell integration for shell under dir and returns
// the arguments and environment to start it with. Shells other than zsh and
// bash get login arguments only.
func Integration(dir, shell string, env []string) ([]string, []string, error) {
	switch ShellKind(shell) {
	case "zsh":
		zdir := filepath.Join(dir, "zsh")
		files := map[string]string{
			".zshenv":   zshStartup(".zshenv"),
			".zprofile": zshStartup(".zprofile"),
			".zshrc":    zshRC(),
		}
		if err := writeFiles(zdir, files); err != nil {
			return nil, nil, err
		}
		user := lookupEnv(env, "ZDOTDIR")
		if user == "" {
			user = lookupEnv(env, "HOME")
		}
		env = setEnv(env, "GOI_USER_ZDOTDIR", user)
		env = setEnv(env, "ZDOTDIR", zdir)
		return []string{"-l"}, env, nil
	case "bash":
		bdir := filepath.Join(dir, "bash")
		if err := writeFiles(bdir, map[string]string{"goi.bashrc": bashRC()}); err != nil {
			return nil, nil, err
		}
		return []string{"--rcfile", filepath.Join(bdir, "goi.bashrc"), "-i"}, env, nil
	}
	return LoginArgs(shell), env, nil
}

// LoginArgs are the arguments that start shell as an interactive login
// shell (none for shells that don't support it).
func LoginArgs(shell string) []string {
	switch ShellKind(shell) {
	case "zsh", "bash", "fish", "sh", "ksh":
		return []string{"-l"}
	case "pwsh", "powershell":
		return []string{"-NoLogo"}
	}
	return nil
}

func writeFiles(dir string, files map[string]string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("terminal: shell integration: %w", err)
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if b, err := os.ReadFile(p); err == nil && string(b) == content {
			continue
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return fmt.Errorf("terminal: shell integration: %w", err)
		}
	}
	return nil
}

func lookupEnv(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(env[i], "="); ok && k == key {
			return v
		}
	}
	return ""
}

// SetEnv replaces or appends key=val.
func setEnv(env []string, key, val string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok && k == key {
			continue
		}
		out = append(out, kv)
	}
	return append(out, key+"="+val)
}

// Environ returns env prepared for a terminal: TERM, COLORTERM and
// TERM_PROGRAM set.
func Environ(env []string) []string {
	env = setEnv(env, "TERM", "xterm-256color")
	env = setEnv(env, "COLORTERM", "truecolor")
	env = setEnv(env, "TERM_PROGRAM", "Goi")
	return env
}
