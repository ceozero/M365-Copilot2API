package web

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func privateBrowserCommand(url string) ([]string, error) {
	return privateBrowserCommandForOS(runtime.GOOS, url, os.Getenv("M365_PRIVATE_BROWSER"))
}

func privateBrowserCommandForOS(goos, url, preferred string) ([]string, error) {
	preferred = strings.ToLower(strings.TrimSpace(preferred))
	browsers := []struct {
		name        string
		app         string
		binary      string
		windowsPath string
		flag        string
	}{
		{"edge", "Microsoft Edge", "msedge", "Microsoft/Edge/Application/msedge.exe", "--inprivate"},
		{"chrome", "Google Chrome", "chrome", "Google/Chrome/Application/chrome.exe", "--incognito"},
		{"brave", "Brave Browser", "brave", "BraveSoftware/Brave-Browser/Application/brave.exe", "--incognito"},
		{"chromium", "Chromium", "chromium", "Chromium/Application/chrome.exe", "--incognito"},
		{"firefox", "Firefox", "firefox", "Mozilla Firefox/firefox.exe", "-private-window"},
	}
	if goos == "darwin" {
		browsers[0], browsers[1] = browsers[1], browsers[0]
	} else if goos != "windows" {
		browsers[0].binary = "microsoft-edge"
		browsers[1].binary = "google-chrome"
		browsers[2].binary = "brave-browser"
	}
	for _, browser := range browsers {
		if preferred != "" && preferred != "auto" && preferred != browser.name {
			continue
		}
		if goos == "darwin" {
			if app := macOSAppPath(browser.app); app != "" {
				return []string{"/usr/bin/open", "-na", app, "--args", browser.flag, url}, nil
			}
			continue
		}
		if goos == "windows" {
			for _, variable := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "LOCALAPPDATA"} {
				if root := os.Getenv(variable); root != "" {
					candidate := filepath.Join(root, filepath.FromSlash(browser.windowsPath))
					if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
						return []string{candidate, browser.flag, url}, nil
					}
				}
			}
		}
		if executable, err := exec.LookPath(browser.binary); err == nil {
			return []string{executable, browser.flag, url}, nil
		}
		if goos != "windows" && browser.name == "chromium" {
			if executable, err := exec.LookPath("chromium-browser"); err == nil {
				return []string{executable, browser.flag, url}, nil
			}
		}
	}
	if preferred != "" && preferred != "auto" {
		return nil, fmt.Errorf("private browser %q was not found in installation directories or PATH", preferred)
	}
	return nil, fmt.Errorf("no supported private browser is available on the server; use the authorization link in your browser")
}

func macOSAppPath(name string) string {
	roots := []string{"/Applications"}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, "Applications"))
	}
	for _, root := range roots {
		candidate := filepath.Join(root, name+".app")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	if exec.Command("/usr/bin/open", "-Ra", name).Run() == nil {
		return name
	}
	return ""
}

func launchPrivateBrowser(url string) error {
	command, err := privateBrowserCommand(url)
	if err != nil {
		return err
	}
	process := exec.Command(command[0], command[1:]...)
	if runtime.GOOS == "darwin" {
		if err := process.Run(); err != nil {
			return fmt.Errorf("open private browser: %w", err)
		}
		return nil
	}
	if err := process.Start(); err != nil {
		return fmt.Errorf("start private browser: %w", err)
	}
	go func() { _ = process.Wait() }()
	return nil
}
