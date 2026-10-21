package web

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPrivateBrowserWindowsInstallDirectories(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		variable string
		relative string
		browser  string
		flag     string
	}{
		{"edge32", "ProgramFiles(x86)", "Microsoft/Edge/Application/msedge.exe", "auto", "--inprivate"},
		{"chrome64", "ProgramFiles", "Google/Chrome/Application/chrome.exe", "chrome", "--incognito"},
		{"chromeUser", "LOCALAPPDATA", "Google/Chrome/Application/chrome.exe", "chrome", "--incognito"},
		{"firefox64", "ProgramW6432", "Mozilla Firefox/firefox.exe", "firefox", "-private-window"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("PATH", t.TempDir())
			for _, variable := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "LOCALAPPDATA"} {
				t.Setenv(variable, "")
			}
			root := t.TempDir()
			t.Setenv(testCase.variable, root)
			executable := filepath.Join(root, filepath.FromSlash(testCase.relative))
			if err := os.MkdirAll(filepath.Dir(executable), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(executable, nil, 0600); err != nil {
				t.Fatal(err)
			}
			const authURL = "https://example.test/authorize?state=a&code_challenge=b"
			command, err := privateBrowserCommandForOS("windows", authURL, testCase.browser)
			if err != nil {
				t.Fatal(err)
			}
			if want := []string{executable, testCase.flag, authURL}; !reflect.DeepEqual(command, want) {
				t.Fatalf("command = %q, want %q", command, want)
			}
		})
	}
}

func TestPrivateBrowserWindowsMissingDoesNotLaunchOrdinaryBrowser(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, variable := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "LOCALAPPDATA"} {
		t.Setenv(variable, "")
	}
	for _, preferred := range []string{"auto", "chrome", "invalid"} {
		if command, err := privateBrowserCommandForOS("windows", "https://example.test", preferred); err == nil || command != nil {
			t.Fatalf("preference %q: command = %q, error = %v", preferred, command, err)
		}
	}
}
