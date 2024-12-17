## install on debian/ubuntu
`curl -L https://raw.githubusercontent.com/rsbear/nixtea/main/install.sh | sudo bash`


### cli to make
    ssh nixtea ctx add "github.com/rsbear/nixtea" (adds a context)
    ssh nixtea ctx ls (lists contexts)
    ssh nixtea ctx rm <ctx name> (removes a context)
    ssh nixtea ps (lists packages for a repo)
    ssh nixtea run <pkg key> (starts the child process)
    ssh nixtea stop <pkg key> (stops the child process)
    ssh nixtea status <pkg key> (prints table of metrics and last 10 log lines
    ssh nixtea logs <pkg key> (open an unterminated log viewing session, esc to quit)


