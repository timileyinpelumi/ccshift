package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/telemetry"
)

// describePlatform is run by the background sender, never by a command the user is waiting on.
func describePlatform(ctx context.Context) telemetry.Platform {
	p := telemetry.Platform{OSVersion: osVersion(ctx)}
	if out, err := runQuick(ctx, "claude", "--version"); err == nil {
		p.ClaudeVersion = claudeVersion(out)
	}
	if goos == "linux" {
		b, _ := os.ReadFile("/proc/version")
		p.WSL = os.Getenv("WSL_DISTRO_NAME") != "" || strings.Contains(strings.ToLower(string(b)), "microsoft")
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		p.Shell = strings.TrimSuffix(filepath.Base(sh), ".exe")
	} else if goos == "windows" {
		p.Shell = "powershell"
	}
	return p
}

func osVersion(ctx context.Context) string {
	switch goos {
	case "linux":
		b, _ := os.ReadFile("/etc/os-release")
		return distro(string(b))
	case "darwin":
		if out, err := runQuick(ctx, "sw_vers", "-productVersion"); err == nil && strings.TrimSpace(out) != "" {
			return "macOS " + strings.TrimSpace(out)
		}
	case "windows":
		return windowsVersion()
	}
	return ""
}

func runQuick(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// distro turns /etc/os-release into "Ubuntu 24.04".
func distro(osRelease string) string {
	fields := map[string]string{}
	for _, line := range strings.Split(osRelease, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			fields[k] = strings.Trim(v, `"'`)
		}
	}
	name := fields["NAME"]
	if name == "" {
		return ""
	}
	if v := fields["VERSION_ID"]; v != "" {
		return name + " " + v
	}
	return name
}

var semver = regexp.MustCompile(`\d+\.\d+\.\d+`)

func claudeVersion(out string) string { return semver.FindString(out) }

// windowsName reports Windows 11 by its build, since the registry still calls it Windows 10.
func windowsName(product, display, build string) string {
	name := "Windows"
	if n, err := strconv.Atoi(build); err == nil {
		if n >= 22000 {
			name = "Windows 11"
		} else {
			name = "Windows 10"
		}
	} else if strings.HasPrefix(product, "Windows ") {
		name = strings.Join(strings.Fields(product)[:2], " ")
	}
	if display != "" {
		name += " " + display
	}
	if build != "" {
		name += " (" + build + ")"
	}
	return name
}
