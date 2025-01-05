## install on debian/ubuntu
`curl -L https://raw.githubusercontent.com/rsbear/nixtea/main/install.sh | sudo bash`


### cli to make
    # repo management
    ssh nt repos add "github.com/rsbear/nixtea" (adds a repo)
    ssh nt repos rm <repo name> (removes a repo)
    ssh nt repos select (tui - dropdown list of repos, select one)

    # after repo is selected, you can run pkg commands
    ssh nt pkgs (list all packages)
    ssh nt pkgs dash (see explanation below)
    ssh nt pkgs start <pkg key> (starts the child process)
    ssh nt pkgs stop <pkg key> (stops the child process)
    ssh nt pkgs status <pkg key> (prints table of metrics and last 10 log lines
    ssh nt pkgs logs <pkg key> (open an unterminated log viewing session, esc to quit)

### ssh nt pkgs dash
- fullscreen tui
- list all packages in left 1/3 of screen
- on hover, show details and logs in right 2/3 of screen
