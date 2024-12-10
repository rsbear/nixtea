## install on debian/ubuntu
`curl -L https://raw.githubusercontent.com/rsbear/nixtea/main/install.sh | sudo bash`


### cli to make
    ssh nixtea (lists saved repos, add repo input, set active repo)
    ssh nixtea ps (lists packages for a repo)
    ssh nixtea <pkg key> run (starts the child process)
    ssh nixtea <pkg key> stop (stops the child process)
    ssh nixtea <pkg key> status (prints table of metrics and last 10 log lines
    ssh nixtea <pkg key> logs (open an unterminated log viewing session, esc to quit)


