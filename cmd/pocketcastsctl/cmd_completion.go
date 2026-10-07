package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func runCompletion(args []string) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printCompletionHelp()
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl completion <bash|zsh|fish>")
		return 2
	}
	shell := strings.ToLower(strings.TrimSpace(args[0]))
	script, ok := completionScripts()[shell]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown shell: %s (supported: bash, zsh, fish)\n", shell)
		return 2
	}
	fmt.Print(script)
	return 0
}

// completionCommand is deliberately private to shell completion. It does not
// dispatch commands or define their help; the parsers remain authoritative.
type completionCommand struct {
	name                string
	options             []completionOption
	children            []completionCommand
	values              []string
	flagsAfterArguments bool
}

type completionOption struct {
	name       string
	takesValue bool
	values     []string
}

func completionTree() completionCommand {
	browsers := []string{"safari", "chrome", "dia", "arc", "brave", "edge"}
	imports := []string{"chrome", "dia", "safari"}
	flags := func(names ...string) []completionOption {
		out := make([]completionOption, len(names))
		for i, name := range names {
			out[i] = completionOption{name: name}
		}
		return out
	}
	value := func(name string, choices ...string) completionOption {
		return completionOption{name: name, takesValue: true, values: choices}
	}
	leaf := func(name string, options ...completionOption) completionCommand {
		return completionCommand{name: name, options: options}
	}
	combine := func(groups ...[]completionOption) []completionOption {
		var options []completionOption
		for _, group := range groups {
			options = append(options, group...)
		}
		return options
	}
	output := flags("json", "plain")
	onboarding := flags("json", "plain", "no-input")
	browser := []completionOption{value("browser", browsers...), value("browser-app"), value("url-contains")}
	importOptions := append([]completionOption{value("browser", imports...), value("profile")}, flags("force", "no-input", "json", "plain")...)
	syncOptions := combine(importOptions, []completionOption{value("browser-app"), value("url-contains"), value("header"), value("prefix"), value("key-contains")}, flags("dry-run"))
	tabs := []completionOption{value("browser", browsers...), value("browser-app"), {name: "json"}, {name: "plain"}}
	reorder := flags("dry-run", "json", "raw")
	picker := []completionOption{value("search"), value("limit"), {name: "recent"}, {name: "unplayed"}, {name: "in-progress"}}
	var doctorCodes []string
	for code := range doctorCodeCatalog("") {
		doctorCodes = append(doctorCodes, code)
	}
	sort.Strings(doctorCodes)
	return completionCommand{children: []completionCommand{
		{name: "help"}, {name: "version"},
		{name: "completion", values: []string{"bash", "zsh", "fish"}},
		leaf("now", append(flags("json", "plain", "watch", "interactive", "verify-auth"), value("interval"), value("max-updates"))...),
		{name: "doctor", options: flags("json", "plain", "quick", "full", "fix", "apply"), children: []completionCommand{
			{name: "explain", options: flags("json"), flagsAfterArguments: true, values: doctorCodes},
		}},
		{name: "setup", options: onboarding, children: []completionCommand{leaf("run", onboarding...), leaf("check", onboarding...), leaf("auth", onboarding...), leaf("verify", onboarding...)}},
		leaf("start", onboarding...),
		{name: "config", children: []completionCommand{
			leaf("init", flags("force")...), leaf("path"), leaf("show", flags("json", "reveal-secrets", "saved")...),
			{name: "set", children: []completionCommand{{name: "browser", values: browsers}}},
		}},
		{name: "auth", children: []completionCommand{
			leaf("login", append([]completionOption{value("email")}, flags("password-stdin", "force", "no-input", "json", "plain")...)...),
			leaf("import-browser", importOptions...), leaf("refresh", output...), leaf("status", output...), leaf("verify", output...), leaf("logout", output...),
			leaf("sync", syncOptions...), leaf("tabs", tabs...), leaf("clear", output...),
		}},
		{name: "web", children: []completionCommand{
			leaf("login", value("browser", browsers...), value("browser-app"), value("url")), leaf("tabs", tabs...),
			leaf("play", browser...), leaf("pause", browser...), leaf("toggle", browser...), leaf("next", browser...), leaf("prev", browser...),
			leaf("status", combine(browser, flags("details", "json", "plain"))...),
		}},
		{name: "queue", children: []completionCommand{
			leaf("ls", combine(flags("json", "plain"), []completionOption{value("search"), value("limit")}, browser)...),
			{name: "api", children: []completionCommand{
				leaf("ls", append(flags("json", "raw", "plain"), value("search"), value("limit"))...),
				leaf("add", value("episode-json"), value("uuid"), value("podcast"), value("title"), value("published"), value("url"), completionOption{name: "raw"}),
				leaf("rm", flags("dry-run", "force", "no-input", "raw")...), leaf("remove", flags("dry-run", "force", "no-input", "raw")...),
				leaf("play", combine([]completionOption{value("search"), value("web-base")}, flags("dry-run"), browser)...),
				leaf("pick", combine(picker, flags("no-play"), browser, []completionOption{value("web-base")})...),
				leaf("bump", reorder...), leaf("move", reorder...), leaf("dedupe", reorder...),
			}},
		}},
		{name: "local", children: []completionCommand{
			leaf("pick", combine(picker, flags("from-start"))...),
			leaf("play", flags("from-start", "dry-run")...), leaf("pause"), leaf("resume"), leaf("stop"), leaf("status", output...),
		}},
		{name: "har", children: []completionCommand{leaf("summarize", value("host"), completionOption{name: "json"}), leaf("graphql", value("host"), completionOption{name: "json"}), leaf("redact")}},
	}}
}

func walkCompletion(node completionCommand, path string, visit func(completionCommand, string)) {
	visit(node, path)
	for _, child := range node.children {
		childPath := strings.TrimSpace(path + " " + child.name)
		walkCompletion(child, childPath, visit)
	}
}

func completionBool(value bool) int {
	if value {
		return 1
	}
	return 0
}

func completionQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func completionFishQuote(value string) string {
	return "'" + strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(value) + "'"
}

func completionWords(words []string, quote func(string) string) string {
	quoted := make([]string, len(words))
	for i, word := range words {
		quoted[i] = quote(word)
	}
	return strings.Join(quoted, " ")
}

// Both array-based shells use the same resolver. The small adapters below
// handle their different word indexing and candidate-registration APIs.
func renderArrayCompletion(tree completionCommand) string {
	var b strings.Builder
	b.WriteString(`_pocketcastsctl_candidates() {
  local cur="$1" command_path='' pending='' word option candidate prefix='' positional=0 flags_started=0 after_arguments=0
  shift
  local -a children option_names value_options values choices
  for word in "$@" ''; do
    children=() option_names=() value_options=() values=()
    case "$command_path" in
`)
	walkCompletion(tree, "", func(node completionCommand, path string) {
		var children, options, valueOptions []string
		for _, child := range node.children {
			children = append(children, child.name)
		}
		for _, option := range node.options {
			options = append(options, "--"+option.name)
			if option.takesValue {
				valueOptions = append(valueOptions, "--"+option.name)
			}
		}
		fmt.Fprintf(&b, "      %s) children=(%s); option_names=(%s); value_options=(%s); values=(%s); after_arguments=%d ;;\n", completionQuote(path), completionWords(children, completionQuote), completionWords(options, completionQuote), completionWords(valueOptions, completionQuote), completionWords(node.values, completionQuote), completionBool(node.flagsAfterArguments))
	})
	b.WriteString(`      *) return ;;
    esac
    # Process one final iteration to load the current path's metadata.
    if [[ $# -eq 0 ]]; then break; fi
    shift
    if [[ -n "$pending" ]]; then pending=''; continue; fi
    [[ $positional -gt 0 && $after_arguments -eq 0 || $positional -eq 2 ]] && continue
    if [[ "$word" == -- ]]; then positional=2; continue; fi
    if [[ "$word" == -* ]]; then
      flags_started=1
      option="${word%%=*}"
      [[ "$option" == --* ]] || option="-$option"
      case " ${option_names[*]} " in *" $option "*) ;; *) return ;; esac
      case " ${value_options[*]} " in
        *" $option "*) [[ "$word" == *=* ]] || pending="$option" ;;
      esac
      continue
    fi
    if [[ $flags_started -eq 0 ]]; then
      case " ${children[*]} " in
        *" $word "*) command_path="${command_path:+$command_path }$word"; continue ;;
      esac
    fi
    positional=1
    values=()
  done
  if [[ $positional -gt 0 && $after_arguments -eq 0 || $positional -eq 2 ]]; then return; fi
  if [[ "$cur" == --*=* ]]; then
    pending="${cur%%=*}"; prefix="$pending="; cur="${cur#*=}"
  fi
  choices=("${children[@]}" "${option_names[@]}" "${values[@]}")
  if [[ -n "$pending" ]]; then
    choices=()
    case "$command_path|$pending" in
`)
	walkCompletion(tree, "", func(node completionCommand, path string) {
		for _, option := range node.options {
			if option.takesValue {
				fmt.Fprintf(&b, "      %s) choices=(%s) ;;\n", completionQuote(path+"|--"+option.name), completionWords(option.values, completionQuote))
			}
		}
	})
	b.WriteString(`    esac
  elif [[ $positional -eq 1 ]]; then
    choices=("${option_names[@]}")
  elif [[ $flags_started -eq 1 ]]; then
    choices=("${option_names[@]}" "${values[@]}")
  fi
  for candidate in "${choices[@]}"; do
    [[ "$candidate" == "$cur"* ]] && printf '%s\n' "$prefix$candidate"
  done
  return 0
}
`)
	return b.String()
}

func renderFishCompletion(tree completionCommand) string {
	var b strings.Builder
	b.WriteString(`function __pocketcastsctl_candidates
    set -l tokens (commandline -opc)
    set -e tokens[1]
    set -l cur (commandline -ct)
    set -l command_path ''
    set -l pending ''
    set -l prefix ''
    set -l positional 0
    set -l flags_started 0
    set -l after_arguments 0
    set -l children
    set -l option_names
    set -l value_options
    set -l values
    set -l remaining (count $tokens)
    for word in $tokens ''
        set children
        set option_names
        set value_options
        set values
        switch $command_path
`)
	walkCompletion(tree, "", func(node completionCommand, path string) {
		var children, options, valueOptions []string
		for _, child := range node.children {
			children = append(children, child.name)
		}
		for _, option := range node.options {
			options = append(options, "--"+option.name)
			if option.takesValue {
				valueOptions = append(valueOptions, "--"+option.name)
			}
		}
		fmt.Fprintf(&b, "            case %s\n                set children %s\n                set option_names %s\n                set value_options %s\n                set values %s\n                set after_arguments %d\n", completionFishQuote(path), completionWords(children, completionFishQuote), completionWords(options, completionFishQuote), completionWords(valueOptions, completionFishQuote), completionWords(node.values, completionFishQuote), completionBool(node.flagsAfterArguments))
	})
	b.WriteString(`            case '*'
                return
        end
        if test $remaining -eq 0
            break
        end
        set remaining (math $remaining - 1)
        if test -n "$pending"
            set pending ''
            continue
        end
        if test $positional -eq 2; or begin; test $positional -eq 1; and test $after_arguments -eq 0; end
            continue
        end
        if test "$word" = --
            set positional 2
            continue
        end
        if string match -q -- '-*' "$word"
            set flags_started 1
            set -l option (string split -m1 = -- "$word")[1]
            if not string match -q -- '--*' "$option"
                set option "-$option"
            end
            contains -- "$option" $option_names; or return
            if contains -- "$option" $value_options; and not string match -q '*=*' -- "$word"
                set pending "$option"
            end
            continue
        end
        if test $flags_started -eq 0; and contains -- "$word" $children
            set command_path (string trim -- "$command_path $word")
            continue
        end
        set positional 1
    end
    if test $positional -eq 2; or begin; test $positional -eq 1; and test $after_arguments -eq 0; end
        return
    end
    if string match -q -- '--*=*' "$cur"
        set -l pair (string split -m1 = -- "$cur")
        set pending $pair[1]
        set prefix "$pending="
        set cur $pair[2]
    end
    set -l choices $children $option_names $values
    if test -n "$pending"
        set choices
        switch "$command_path|$pending"
`)
	walkCompletion(tree, "", func(node completionCommand, path string) {
		for _, option := range node.options {
			if option.takesValue {
				fmt.Fprintf(&b, "            case %s\n                set choices %s\n", completionFishQuote(path+"|--"+option.name), completionWords(option.values, completionFishQuote))
			}
		}
	})
	b.WriteString(`        end
    else if test $positional -eq 1
        set choices $option_names
    else if test $flags_started -eq 1
        set choices $option_names $values
    end
    for candidate in $choices
        if test -z "$cur"; or string match -q -- "$cur*" "$candidate"
            printf '%s\n' "$prefix$candidate"
        end
    end
end
complete -c pocketcastsctl -f -a '(__pocketcastsctl_candidates)'
`)
	return b.String()
}

func completionScripts() map[string]string {
	tree := completionTree()
	resolver := renderArrayCompletion(tree)
	return map[string]string{
		"bash": "#!/usr/bin/env bash\n" + resolver + `_pocketcastsctl_completions() {
  local LC_ALL=C
  local candidate cur i word text="$COMP_LINE" prefix start end
  local -a prior starts ends
  # Bash versions differ in whether COMP_WORDS splits on readline word breaks.
  # Locate fragments in the original line, then join only adjacent fragments.
  # Work backwards to keep repeated words and preceding commands unambiguous.
  for ((i=${#COMP_WORDS[@]}-1; i>=0; i--)); do
    word="${COMP_WORDS[i]}"
    starts[i]=-1
    ends[i]=-1
    if [[ -n "$word" && "$text" == *"$word"* ]]; then
      prefix="${text%"$word"*}"
      starts[i]=${#prefix}
      ends[i]=$((${#prefix}+${#word}))
      text="$prefix"
    fi
  done
  if [[ -z "${COMP_WORDS[COMP_CWORD]}" ]]; then
    starts[COMP_CWORD]=$COMP_POINT
    ends[COMP_CWORD]=$COMP_POINT
  fi
  for ((i=1; i<=COMP_CWORD; i++)); do
    word="${COMP_WORDS[i]}"
    start=${starts[i]}
    end=${ends[i]}
    if (( i == COMP_CWORD && start >= 0 && COMP_POINT >= start && COMP_POINT < end )); then
      word="${word:0:COMP_POINT-start}"
    fi
    if (( i > 1 && start >= 0 && start == ends[i-1] )); then
      prior[${#prior[@]}-1]+="$word"
    else
      prior+=("$word")
    fi
  done
  cur="${prior[${#prior[@]}-1]}"
  unset 'prior[${#prior[@]}-1]'
  COMPREPLY=()
  while IFS= read -r candidate; do
    if [[ "$cur" == --*=* && "$COMP_WORDBREAKS" == *"="* ]]; then candidate="${candidate#*=}"; fi
    COMPREPLY+=("$candidate")
  done < <(
    _pocketcastsctl_candidates "$cur" "${prior[@]}"
  )
}
complete -F _pocketcastsctl_completions pocketcastsctl
`,
		"zsh": "#compdef pocketcastsctl\n" + resolver + `_pocketcastsctl_completions() {
  local output
  local -a candidates prior
  if (( CURRENT > 2 )); then prior=("${words[@]:1:$((CURRENT-2))}"); fi
  output="$(_pocketcastsctl_candidates "${words[CURRENT]}" "${prior[@]}")"
  [[ -n "$output" ]] || return 0
  candidates=("${(@f)output}")
  compadd -a candidates
}
_pocketcastsctl_completions "$@"
`,
		"fish": renderFishCompletion(tree),
	}
}
