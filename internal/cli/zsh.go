package cli

const zsh = `# Pogo: add this once near the end of ~/.zshrc via eval "$(pogo init zsh)".
function _pogo_record() {
    command pogo add -- "$PWD" >/dev/null 2>&1
    return 0
}

function j() {
    local destination
    if (( $# == 0 )); then
        builtin cd -- "$HOME"
    elif (( $# == 1 )) && [[ "$1" == "-" || -d "$1" ]]; then
        builtin cd -- "$1"
    else
        destination=$(command pogo query -- "$@") || return $?
        builtin cd -- "$destination"
    fi
}

autoload -Uz add-zsh-hook
add-zsh-hook -d chpwd _pogo_record
add-zsh-hook chpwd _pogo_record
_pogo_record
`
