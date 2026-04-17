//go:build windows

package config

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestLoadRejectsWindowsTokenFileReadableByBuiltinUsers(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenPath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := setProtectedACL(dir, secureWindowsEntries(t, windows.GENERIC_ALL)...); err != nil {
		t.Fatalf("setProtectedACL(dir) error = %v", err)
	}
	entries := append(secureWindowsEntries(t, windows.GENERIC_ALL), explicitAccessEntry(t, windows.WinBuiltinUsersSid, windows.GENERIC_READ))
	if err := setProtectedACL(tokenPath, entries...); err != nil {
		t.Fatalf("setProtectedACL(tokenPath) error = %v", err)
	}

	configPath := filepath.Join(dir, "config.json")
	configJSON := `{
  "record": {
    "name": "host.example.com.",
    "zone": "example.com.",
    "ttl_seconds": 300
  },
  "probe": {
    "timeout": "10s"
  },
  "provider": {
    "type": "cloudflare",
    "timeout": "10s",
    "cloudflare": {
      "zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
      "api_token_file": ` + jsonStringLiteral(t, tokenPath) + `
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	if _, err := Load(configPath); err == nil {
		t.Fatal("Load() error = nil, want Windows ACL rejection")
	}
}

func setProtectedACL(path string, entries ...windows.EXPLICIT_ACCESS) error {
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	)
}

func secureWindowsEntries(t *testing.T, permissions windows.ACCESS_MASK) []windows.EXPLICIT_ACCESS {
	t.Helper()

	return []windows.EXPLICIT_ACCESS{
		explicitAccessEntryForSID(currentWindowsUserSIDForTest(t), permissions),
		explicitAccessEntry(t, windows.WinLocalSystemSid, permissions),
		explicitAccessEntry(t, windows.WinBuiltinAdministratorsSid, permissions),
	}
}

func explicitAccessEntry(t *testing.T, sidType windows.WELL_KNOWN_SID_TYPE, permissions windows.ACCESS_MASK) windows.EXPLICIT_ACCESS {
	t.Helper()

	sid, err := windows.CreateWellKnownSid(sidType)
	if err != nil {
		t.Fatalf("CreateWellKnownSid(%v) error = %v", sidType, err)
	}
	return explicitAccessEntryForSID(sid, permissions)
}

func explicitAccessEntryForSID(sid *windows.SID, permissions windows.ACCESS_MASK) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: permissions,
		AccessMode:        windows.GRANT_ACCESS,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}

func currentWindowsUserSIDForTest(t *testing.T) *windows.SID {
	t.Helper()

	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		t.Fatalf("OpenCurrentProcessToken() error = %v", err)
	}
	defer token.Close()

	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatalf("GetTokenUser() error = %v", err)
	}
	return user.User.Sid
}
