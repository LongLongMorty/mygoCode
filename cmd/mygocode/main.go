package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"mygocode/internal/config"
	"mygocode/internal/hooks"
	"mygocode/internal/remote"
	"mygocode/internal/tui"
)

func main() {
	if args, ok := parseTeammateFlags(os.Args[1:]); ok {
		if err := runTeammate(args); err != nil {
			fmt.Fprintf(os.Stderr, "teammate: %s\n", err)
			os.Exit(1)
		}
		return
	}
	if handleProjectTrustFlag(os.Args[1:]) {
		return
	}

	// 解析 --remote 模式
	remoteAddr := ""
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "--remote" {
			remoteAddr = ":18888"
			if i+1 < len(os.Args) && os.Args[i+1][0] != '-' {
				remoteAddr = os.Args[i+1]
				i++
			}
		}
	}

	cfg, err := config.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	validHooks := cfg.Hooks
	if err := hooks.Validate(validHooks); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: hook configuration is invalid, starting with no hooks:\n%s\n", err)
		validHooks = nil
	}

	// --remote 模式：启动 HTTP + WebSocket 服务器，浏览器访问 Web UI
	if remoteAddr != "" {
		srv := remote.NewServer(cfg.Providers, cfg.MCPServers, validHooks, remoteAddr, cfg.EnableCoordinatorMode)
		if err := srv.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Remote server error: %s\n", err)
			os.Exit(1)
		}
		return
	}

	m := tui.New(cfg.Providers, cfg.MCPServers, validHooks)
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

// handleProjectTrustFlag provides a safe bootstrap path for projects that
// only have project-local configuration. Without this explicit decision there
// may be no global provider configuration available to start the TUI and run
// its /trust command.
func handleProjectTrustFlag(args []string) bool {
	if len(args) != 1 {
		return false
	}
	var state config.TrustState
	switch strings.ToLower(args[0]) {
	case "--trust-project":
		state = config.TrustTrusted
	case "--deny-project":
		state = config.TrustDenied
	case "--revoke-project-trust":
		wd, _ := os.Getwd()
		home, _ := os.UserHomeDir()
		if err := config.RevokeProjectTrust(home, wd); err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not revoke project trust: %s\n", err)
			os.Exit(1)
		}
		fmt.Println("Project trust revoked.")
		return true
	default:
		return false
	}
	wd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	if err := config.SetProjectTrust(home, wd, state); err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not save project trust decision: %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("Project trust set to %s. Start MygoCode again to apply it.\n", state)
	return true
}
