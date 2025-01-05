## install on debian/ubuntu
`curl -L https://raw.githubusercontent.com/rsbear/nixtea/main/install.sh | sudo bash`


### cli to make
    # repo management
    ssh nt repos add "github.com/rsbear/nixtea" (adds a repo)
    ssh nt repos rm <repo name> (removes a repo)
    ssh nt repos update <repo name> (updates a repo)
    ssh nt repos select (tui - dropdown list of repos, select one)
    ssh nt repos (returns same handler as select)

    # after repo is selected, you can run pkg commands
    ssh nt pkgs (list all packages)
    ssh nt pkgs start <pkg key> (starts the child process)
    ssh nt pkgs stop <pkg key> (stops the child process)
    ssh nt pkgs status <pkg key> (prints table of metrics and last 10 log lines
    ssh nt pkgs logs <pkg key> (open an unterminated log viewing session, esc to quit)

### repo management
```bash
ssh nt repos add "github.com/rsbear/nixtea" (adds a repo)
```
```bash
ssh nt repos rm <repo name> (removes a repo)
```
```bash
ssh nt repos update <repo name> (updates a repo)
```
```bash
ssh nt repos select (tui - dropdown list of repos, select one)
```

### after repo is selected, you can run pkg commands
```bash
ssh nt pkgs (list all packages)
```
```bash
ssh nt pkgs start <pkg key> (starts the child process)
```
```bash
 ssh nt pkgs stop <pkg key> (stops the child process)
 ```
 ```bash
 ssh nt pkgs status <pkg key> (prints table of metrics and last 10 log lines
 ```
 ```bash
 ssh nt pkgs logs <pkg key> (open an unterminated log viewing session, esc to quit)
 ```

### ssh nt pkgs dash
this is a feature to add and is not yet implemented
- fullscreen tui
- list all packages in left 1/3 of screen
- on hover, show details and logs in right 2/3 of screen

### todo: how to update pkgs?
using git push hooks or should the cli pull updates?


### NEXT UP:
Task Overview:

1. Repository Management Updates:
   - Add missing `repos rm` command 
   - Rename existing repo commands for consistency:
     ```go
     repos add <url>
     repos rm <repo name>
     repos update <repo name> 
     repos select    // TUI selection
     repos          // Default to selection interface
     ```
   - Move repo selection TUI to `repos select` subcommand
   - Ensure `repos` without arguments shows selection interface

2. Package Management Updates:
   - Rename `pkgs run` to `pkgs start` for consistency
   - Structure package commands:
     ```go
     pkgs             // List all packages
     pkgs start <key> // Start package
     pkgs stop <key>  // Stop package
     pkgs status <key> // Show metrics
     pkgs logs <key>   // Stream logs
     ```
   - Consolidate package listing under bare `pkgs` command
   - Ensure consistent formatting across all package commands
   - Enhance status command with metrics/logs table
   - Implement better log streaming with clean terminal handling

3. Code Organization:
   - Create new package `internal/commands` to separate command implementations
   - Split command handlers into separate files:
     - `internal/commands/repos.go`
     - `internal/commands/pkgs.go`
   - Move shared utilities to `internal/commands/utils.go`
   - Create shared types and interfaces in `internal/commands/types.go`

4. SSH/Middleware Updates:
   - Update middleware in `cli.go` to handle new command structure
   - Ensure proper handling of TUI vs CLI modes
   - Add robust error handling for SSH sessions
   - Improve session context management

5. Documentation Updates:
   - Add detailed command documentation in code
   - Add usage examples in help text
   - Update command descriptions

6. Testing Requirements:
   - Add unit tests for commands package
   - Add integration tests for SSH command handling
   - Test error scenarios and edge cases
   - Add test utilities for command testing

Would you like me to provide more detailed implementation steps for any of these tasks? I can start with creating the new command structure or focus on a specific aspect of the refactoring.
