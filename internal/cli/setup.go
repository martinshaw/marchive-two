package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martinshaw/marchive-two/internal/client"
	"github.com/martinshaw/marchive-two/internal/service"
)

func cliSetup() *cobra.Command {
	return &cobra.Command{
		Use:   "cli-setup",
		Short: "Set the fixed UI port and login service from a terminal",
		Long: `Asks for a fixed localhost port, then whether Marchive should start at
login and restart after a crash. Both of those answers default to yes.
The port is used the next time Marchive starts.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			current := 0
			if state, err := currentState(); err == nil {
				current = state.ListenPort
			}
			port, open, restart, err := readSetup(cmd.InOrStdin(), cmd.OutOrStdout(), current)
			if err != nil {
				return err
			}
			if err := applySetup(port, open, restart); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Fixed UI port %d. Marchive uses it the next time it starts.\nOpen on login is %s.\nRestart on crash is %s.\n", port, onOff(open), onOff(restart))
			return nil
		},
	}
}

func applySetup(port int, open, restart bool) error {
	in := service.SettingsInput{
		ListenPort:     &port,
		OpenOnLogin:    &open,
		RestartOnCrash: &restart,
	}
	return withEither(func(c *client.Client) error {
		return c.UpdateSettings(in)
	}, func(svc *service.Service) error {
		return svc.UpdateSettings(in)
	})
}

func readSetup(in io.Reader, out io.Writer, currentPort int) (int, bool, bool, error) {
	lines := bufio.NewReader(in)
	port, err := askPort(lines, out, currentPort)
	if err != nil {
		return 0, false, false, err
	}
	open, err := askYes(lines, out, "Open on login?", true)
	if err != nil {
		return 0, false, false, err
	}
	restart, err := askYes(lines, out, "Restart on crash?", true)
	if err != nil {
		return 0, false, false, err
	}
	return port, open, restart, nil
}

func askPort(in *bufio.Reader, out io.Writer, currentPort int) (int, error) {
	prompt := "Fixed UI port (1-65535): "
	if currentPort > 0 {
		prompt = fmt.Sprintf("Fixed UI port (1-65535) [%d]: ", currentPort)
	}
	for {
		line, err := askLine(in, out, prompt)
		if err != nil {
			return 0, err
		}
		if line == "" && currentPort > 0 {
			return currentPort, nil
		}
		n, conv := strconv.Atoi(line)
		if conv != nil || n < 1 || n > 65535 {
			fmt.Fprintln(out, "Choose a port from 1 to 65535.")
			continue
		}
		return n, nil
	}
}

func askYes(in *bufio.Reader, out io.Writer, prompt string, fallback bool) (bool, error) {
	hint := "[y/N]"
	if fallback {
		hint = "[Y/n]"
	}
	for {
		line, err := askLine(in, out, prompt+" "+hint+" ")
		if err != nil {
			return false, err
		}
		switch strings.ToLower(line) {
		case "":
			return fallback, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(out, "Answer yes or no.")
		}
	}
}

func askLine(in *bufio.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	line, err := in.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && strings.TrimSpace(line) != "") {
		if errors.Is(err, io.EOF) {
			return "", errors.New("setup cancelled")
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}
