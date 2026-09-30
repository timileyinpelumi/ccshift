package cli

import "testing"

func TestDistroFromOSRelease(t *testing.T) {
	cases := map[string]string{
		"NAME=\"Ubuntu\"\nVERSION_ID=\"24.04\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n": "Ubuntu 24.04",
		"NAME=\"Fedora Linux\"\nVERSION_ID=41\n":                                      "Fedora Linux 41",
		"NAME=\"Arch Linux\"\nBUILD_ID=rolling\n":                                     "Arch Linux",
		"": "",
	}
	for in, want := range cases {
		if got := distro(in); got != want {
			t.Errorf("distro(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClaudeVersion(t *testing.T) {
	for in, want := range map[string]string{"2.1.3 (Claude Code)\n": "2.1.3", "": "", "claude 1.0.9": "1.0.9"} {
		if got := claudeVersion(in); got != want {
			t.Errorf("claudeVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWindowsName(t *testing.T) {
	if got := windowsName("Windows 10 Pro", "24H2", "26100"); got != "Windows 11 24H2 (26100)" {
		t.Fatal(got)
	}
	if got := windowsName("Windows 10 Pro", "22H2", "19045"); got != "Windows 10 22H2 (19045)" {
		t.Fatal(got)
	}
}
