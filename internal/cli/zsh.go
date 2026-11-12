package cli

const zsh = `# Pogo: add this once near the end of ~/.zshrc via eval "$(pogo init zsh)".
function _pogo_record() {
    command pogo add -- "$PWD" >/dev/null 2>&1
    return 0
}

function j() {
    local destination result
    if (( $# == 0 )); then
        builtin cd -- "$HOME"
    elif (( $# == 1 )) && [[ "$1" == "-" ]]; then
        builtin cd -
    else
        # Exact names beat learned rankings and same-named local folders.
        # Explicit paths retain normal cd behavior.
        if (( $# == 1 )) && [[ "$1" != */* && "$1" != "." && "$1" != ".." ]]; then
            if destination=$(command pogo query --pin -- "$1"); then
                builtin cd -- "$destination"
                return $?
            else
                result=$?
                (( result == 2 )) || return $result
            fi
        fi
        if (( $# == 1 )) && [[ -d "$1" ]]; then
            builtin cd -- "$1"
        else
            destination=$(command pogo query -- "$@") || return $?
            builtin cd -- "$destination"
        fi
    fi
}

autoload -Uz add-zsh-hook
add-zsh-hook -d chpwd _pogo_record
add-zsh-hook chpwd _pogo_record
_pogo_record
`
