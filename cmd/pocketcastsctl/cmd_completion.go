package main

import (
	"fmt"
	"os"
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

func completionScripts() map[string]string {
	return map[string]string{
		"bash": `#!/usr/bin/env bash
_pocketcastsctl_completions() {
  local cur prev cmd sub
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  cmd="${COMP_WORDS[1]}"
  sub="${COMP_WORDS[2]}"

  if [[ $COMP_CWORD -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "help version completion now doctor setup start config auth web queue local har" -- "$cur") )
    return 0
  fi

  case "$cmd" in
    completion)
      COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") )
      return 0
      ;;
    now)
      COMPREPLY=( $(compgen -W "--json --plain --watch --interactive --verify-auth --interval --max-updates" -- "$cur") )
      return 0
      ;;
    setup)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "run check auth verify --json --plain --no-input" -- "$cur") )
      else
        COMPREPLY=( $(compgen -W "--json --plain --no-input" -- "$cur") )
      fi
      return 0
      ;;
    start)
      COMPREPLY=( $(compgen -W "--json --plain --no-input" -- "$cur") )
      return 0
      ;;
    doctor)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "explain --json --plain --quick --full --fix --apply" -- "$cur") )
      else
        COMPREPLY=( $(compgen -W "--json --plain --quick --full --fix --apply doctor.auth.invalid doctor.auth.unverified doctor.auth.session_missing doctor.auth.legacy_config" -- "$cur") )
      fi
      return 0
      ;;
    auth)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "login import-browser refresh status verify logout sync tabs clear" -- "$cur") )
      else
        case "$sub" in
          login) COMPREPLY=( $(compgen -W "--email --password-stdin --force --no-input --json --plain" -- "$cur") ) ;;
          import-browser) COMPREPLY=( $(compgen -W "--browser --profile --force --no-input --json --plain" -- "$cur") ) ;;
          refresh|status|verify|logout) COMPREPLY=( $(compgen -W "--json --plain" -- "$cur") ) ;;
          sync) COMPREPLY=( $(compgen -W "--browser --profile --force --no-input --json --plain" -- "$cur") ) ;;
          tabs) COMPREPLY=( $(compgen -W "--browser --browser-app --json --plain" -- "$cur") ) ;;
          clear) COMPREPLY=() ;;
        esac
      fi
      return 0
      ;;
    config)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "init path show" -- "$cur") )
      else
        [[ "$sub" == "show" ]] && COMPREPLY=( $(compgen -W "--json --reveal-secrets" -- "$cur") ) || COMPREPLY=()
      fi
      return 0
      ;;
    web)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "login tabs play pause toggle next prev status" -- "$cur") )
      else
        COMPREPLY=( $(compgen -W "--browser --browser-app --url-contains --json --plain" -- "$cur") )
      fi
      return 0
      ;;
    queue)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "ls api" -- "$cur") )
      elif [[ "$sub" == "ls" ]]; then
        COMPREPLY=( $(compgen -W "--json --plain --search --limit --browser --browser-app --url-contains" -- "$cur") )
      elif [[ "$sub" == "api" ]]; then
        local api_cmd="${COMP_WORDS[3]}"
        if [[ $COMP_CWORD -eq 3 ]]; then
          COMPREPLY=( $(compgen -W "ls add rm play pick bump move dedupe" -- "$cur") )
        else
          case "$api_cmd" in
            ls) COMPREPLY=( $(compgen -W "--json --raw --plain --search --limit" -- "$cur") ) ;;
            add) COMPREPLY=( $(compgen -W "--episode-json --uuid --podcast --title --published --url --raw" -- "$cur") ) ;;
            rm) COMPREPLY=( $(compgen -W "--dry-run --force --no-input --raw" -- "$cur") ) ;;
            play) COMPREPLY=( $(compgen -W "--search --dry-run --browser --browser-app --url-contains --web-base" -- "$cur") ) ;;
            pick) COMPREPLY=( $(compgen -W "--search --limit --recent --unplayed --in-progress --no-play --browser --browser-app --url-contains --web-base" -- "$cur") ) ;;
            bump) COMPREPLY=( $(compgen -W "--dry-run --json --raw" -- "$cur") ) ;;
            move) COMPREPLY=( $(compgen -W "--dry-run --json --raw" -- "$cur") ) ;;
            dedupe) COMPREPLY=( $(compgen -W "--dry-run --json --raw" -- "$cur") ) ;;
          esac
        fi
      fi
      return 0
      ;;
    local)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "pick play pause resume stop status" -- "$cur") )
      else
        case "$sub" in
          pick) COMPREPLY=( $(compgen -W "--search --limit --recent --unplayed --in-progress --from-start" -- "$cur") ) ;;
          play) COMPREPLY=( $(compgen -W "--from-start --dry-run" -- "$cur") ) ;;
          status) COMPREPLY=( $(compgen -W "--json --plain" -- "$cur") ) ;;
          *) COMPREPLY=() ;;
        esac
      fi
      return 0
      ;;
    har)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "summarize graphql redact" -- "$cur") )
      else
        case "$sub" in
          summarize|graphql) COMPREPLY=( $(compgen -W "--host --json" -- "$cur") ) ;;
          redact) COMPREPLY=() ;;
        esac
      fi
      return 0
      ;;
  esac

  COMPREPLY=()
}
complete -F _pocketcastsctl_completions pocketcastsctl
`,
		"zsh": `#compdef pocketcastsctl
_pocketcastsctl_completions() {
  local curcontext="$curcontext" state line
  local cmd sub
  cmd="${words[2]}"
  sub="${words[3]}"

  if (( CURRENT == 2 )); then
    _values "commands" \
      "help" "version" "completion" "now" "doctor" "setup" "start" "config" "auth" "web" "queue" "local" "har"
    return
  fi

  case "$cmd" in
    completion)
      _values "shell" "bash" "zsh" "fish"
      ;;
    now)
      _values "flags" "--json" "--plain" "--watch" "--interactive" "--verify-auth" "--interval" "--max-updates"
      ;;
    setup)
      if (( CURRENT == 3 )); then
        _values "subcommands/flags" "run" "check" "auth" "verify" "--json" "--plain" "--no-input"
      else
        _values "flags" "--json" "--plain" "--no-input"
      fi
      ;;
    start)
      _values "flags" "--json" "--plain" "--no-input"
      ;;
    doctor)
      if (( CURRENT == 3 )); then
        _values "subcommands/flags" "explain" "--json" "--plain" "--quick" "--full" "--fix" "--apply"
      else
        _values "flags/codes" "--json" "--plain" "--quick" "--full" "--fix" "--apply" "doctor.auth.invalid" "doctor.auth.unverified" "doctor.auth.session_missing" "doctor.auth.legacy_config"
      fi
      ;;
    auth)
      if (( CURRENT == 3 )); then
        _values "auth subcommands" "login" "import-browser" "refresh" "status" "verify" "logout" "sync" "tabs" "clear"
      else
        case "$sub" in
          login) _values "flags" "--email" "--password-stdin" "--force" "--no-input" "--json" "--plain" ;;
          import-browser) _values "flags" "--browser" "--profile" "--force" "--no-input" "--json" "--plain" ;;
          refresh|status|verify|logout) _values "flags" "--json" "--plain" ;;
          sync) _values "flags" "--browser" "--profile" "--force" "--no-input" "--json" "--plain" ;;
          tabs) _values "flags" "--browser" "--browser-app" "--json" "--plain" ;;
        esac
      fi
      ;;
    config)
      if (( CURRENT == 3 )); then
        _values "config subcommands" "init" "path" "show"
      else
        [[ "$sub" == "show" ]] && _values "flags" "--json" "--reveal-secrets"
      fi
      ;;
    web)
      if (( CURRENT == 3 )); then
        _values "web subcommands" "login" "tabs" "play" "pause" "toggle" "next" "prev" "status"
      else
        _values "flags" "--browser" "--browser-app" "--url-contains" "--json" "--plain"
      fi
      ;;
    queue)
      if (( CURRENT == 3 )); then
        _values "queue subcommands" "ls" "api"
      elif [[ "$sub" == "ls" ]]; then
        _values "flags" "--json" "--plain" "--search" "--limit" "--browser" "--browser-app" "--url-contains"
      elif [[ "$sub" == "api" ]]; then
        local api_cmd="${words[4]}"
        if (( CURRENT == 4 )); then
          _values "queue api subcommands" "ls" "add" "rm" "play" "pick" "bump" "move" "dedupe"
        else
          case "$api_cmd" in
            ls) _values "flags" "--json" "--raw" "--plain" "--search" "--limit" ;;
            add) _values "flags" "--episode-json" "--uuid" "--podcast" "--title" "--published" "--url" "--raw" ;;
            rm) _values "flags" "--dry-run" "--force" "--no-input" "--raw" ;;
            play) _values "flags" "--search" "--dry-run" "--browser" "--browser-app" "--url-contains" "--web-base" ;;
            pick) _values "flags" "--search" "--limit" "--recent" "--unplayed" "--in-progress" "--no-play" "--browser" "--browser-app" "--url-contains" "--web-base" ;;
            bump|move|dedupe) _values "flags" "--dry-run" "--json" "--raw" ;;
          esac
        fi
      fi
      ;;
    local)
      if (( CURRENT == 3 )); then
        _values "local subcommands" "pick" "play" "pause" "resume" "stop" "status"
      else
        case "$sub" in
          pick) _values "flags" "--search" "--limit" "--recent" "--unplayed" "--in-progress" "--from-start" ;;
          play) _values "flags" "--from-start" "--dry-run" ;;
          status) _values "flags" "--json" "--plain" ;;
        esac
      fi
      ;;
    har)
      if (( CURRENT == 3 )); then
        _values "har subcommands" "summarize" "graphql" "redact"
      else
        case "$sub" in
          summarize|graphql) _values "flags" "--host" "--json" ;;
        esac
      fi
      ;;
  esac
}
_pocketcastsctl_completions "$@"
`,
		"fish": `complete -c pocketcastsctl -f -n '__fish_use_subcommand' -a 'help version completion now doctor setup start config auth web queue local har'
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'

complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from now' -l json -l plain -l watch -l interactive -l verify-auth -l interval -l max-updates
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from setup' -a 'run check auth verify'
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from setup' -l json -l plain -l no-input
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from start' -l json -l plain -l no-input
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from doctor' -a 'explain' -l json -l plain -l quick -l full -l fix -l apply
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from config' -a 'init path show'
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from auth' -a 'login import-browser refresh status verify logout sync tabs clear'
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from web' -a 'login tabs play pause toggle next prev status'
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from auth; and __fish_seen_subcommand_from login' -l email -l password-stdin -l force -l no-input -l json -l plain
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from auth; and __fish_seen_subcommand_from import-browser' -l browser -l profile -l force -l no-input -l json -l plain
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from auth; and __fish_seen_subcommand_from sync' -l browser -l profile -l force -l no-input -l json -l plain
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from queue' -a 'ls api'
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from queue; and __fish_seen_subcommand_from api' -a 'ls add rm play pick bump move dedupe'
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from local' -a 'pick play pause resume stop status'
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from har' -a 'summarize graphql redact'

complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from queue; and __fish_seen_subcommand_from api; and __fish_seen_subcommand_from play' -l dry-run -l search -l browser -l browser-app -l url-contains -l web-base
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from queue; and __fish_seen_subcommand_from api; and __fish_seen_subcommand_from bump' -l dry-run -l json -l raw
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from queue; and __fish_seen_subcommand_from api; and __fish_seen_subcommand_from move' -l dry-run -l json -l raw
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from queue; and __fish_seen_subcommand_from api; and __fish_seen_subcommand_from dedupe' -l dry-run -l json -l raw
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from local; and __fish_seen_subcommand_from play' -l dry-run -l from-start
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from queue; and __fish_seen_subcommand_from api; and __fish_seen_subcommand_from rm' -l dry-run -l force -l no-input
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from queue; and __fish_seen_subcommand_from ls' -l json -l plain -l search -l limit -l browser -l browser-app -l url-contains
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from queue; and __fish_seen_subcommand_from api; and __fish_seen_subcommand_from ls' -l json -l plain -l raw -l search -l limit
`,
	}
}
