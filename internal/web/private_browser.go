package web

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

func privateBrowserCommand(url string) ([]string, error) {
	preferred := os.Getenv("M365_PRIVATE_BROWSER")
	if runtime.GOOS == "darwin" {
		browsers := []struct {
			name string
			app  string
			args []string
		}{
			{"chrome", "Google Chrome", []string{"-na", "Google Chrome", "--args", "--incognito", url}},
			{"edge", "Microsoft Edge", []string{"-na", "Microsoft Edge", "--args", "--inprivate", url}},
			{"brave", "Brave Browser", []string{"-na", "Brave Browser", "--args", "--incognito", url}},
			{"firefox", "Firefox", []string{"-na", "Firefox", "--args", "-private-window", url}},
		}
		if _, err := exec.LookPath("open"); err != nil {
			return nil, fmt.Errorf("macOS open command is unavailable")
		}
		for _, browser := range browsers {
			if (preferred == "" || preferred == "auto" || preferred == browser.name) && macOSAppAvailable(browser.app) {
				return append([]string{"open"}, browser.args...), nil
			}
		}
		return nil, fmt.Errorf("unsupported private browser %q", preferred)
	}

	browsers := []struct {
		name string
		args []string
	}{
		{"edge", []string{"msedge", "--inprivate", url}},
		{"chrome", []string{"chrome", "--incognito", url}},
		{"brave", []string{"brave", "--incognito", url}},
		{"chromium", []string{"chromium", "--incognito", url}},
		{"firefox", []string{"firefox", "-private-window", url}},
	}
	if runtime.GOOS == "linux" {
		browsers[0].args[0] = "microsoft-edge"
		browsers[1].args[0] = "google-chrome"
	}
	for _, browser := range browsers {
		if (preferred != "" && preferred != "auto" && preferred != browser.name) || !browserAvailable(browser.args[0]) {
			continue
		}
		return browser.args, nil
	}
	if preferred != "" && preferred != "auto" {
		return nil, fmt.Errorf("private browser %q was not found on PATH", preferred)
	}
	return nil, fmt.Errorf("no supported private browser was found on PATH")
}

func macOSAppAvailable(name string) bool {
	return exec.Command("open", "-Ra", name).Run() == nil
}

func browserAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func launchPrivateBrowser(url string) error {
	command, err := privateBrowserCommand(url)
	if err != nil {
		return err
	}
	if err := exec.Command(command[0], command[1:]...).Start(); err != nil {
		return fmt.Errorf("start private browser: %w", err)
	}
	return nil
}