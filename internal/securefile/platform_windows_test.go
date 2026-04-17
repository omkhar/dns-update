//go:build windows

package securefile

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestValidateAndReadSingleTokenWindowsACLs(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenPath, []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := setProtectedACL(dir, secureWindowsEntries(t, windows.GENERIC_ALL)...); err != nil {
		t.Fatalf("setProtectedACL(dir) error = %v", err)
	}
	if err := setProtectedACL(tokenPath, secureWindowsEntries(t, windows.GENERIC_ALL)...); err != nil {
		t.Fatalf("setProtectedACL(tokenPath) error = %v", err)
	}

	if err := Validate(tokenPath); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	token, err := ReadSingleToken(tokenPath)
	if err != nil {
		t.Fatalf("ReadSingleToken() error = %v", err)
	}
	if got, want := token, "secret"; got != want {
		t.Fatalf("ReadSingleToken() = %q, want %q", got, want)
	}
}

func TestValidateAndReadSingleTokenRejectWindowsFileReaders(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenPath, []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := setProtectedACL(dir, secureWindowsEntries(t, windows.GENERIC_ALL)...); err != nil {
		t.Fatalf("setProtectedACL(dir) error = %v", err)
	}
	entries := append(secureWindowsEntries(t, windows.GENERIC_ALL), explicitAccessEntry(t, windows.WinBuiltinUsersSid, windows.GENERIC_READ))
	if err := setProtectedACL(tokenPath, entries...); err != nil {
		t.Fatalf("setProtectedACL(tokenPath) error = %v", err)
	}

	if err := Validate(tokenPath); err == nil {
		t.Fatal("Validate() error = nil, want Windows ACL rejection")
	}
	if _, err := ReadSingleToken(tokenPath); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want Windows ACL rejection")
	}
}

func TestValidateAndReadSingleTokenRejectWindowsWritableParent(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenPath, []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	parentEntries := append(secureWindowsEntries(t, windows.GENERIC_ALL), explicitAccessEntry(t, windows.WinBuiltinUsersSid, windows.GENERIC_WRITE))
	if err := setProtectedACL(dir, parentEntries...); err != nil {
		t.Fatalf("setProtectedACL(dir) error = %v", err)
	}
	if err := setProtectedACL(tokenPath, secureWindowsEntries(t, windows.GENERIC_ALL)...); err != nil {
		t.Fatalf("setProtectedACL(tokenPath) error = %v", err)
	}

	if err := Validate(tokenPath); err == nil {
		t.Fatal("Validate() error = nil, want Windows parent-directory ACL rejection")
	}
	if _, err := ReadSingleToken(tokenPath); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want Windows parent-directory ACL rejection")
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

	sid, err := currentWindowsUserSID()
	if err != nil {
		t.Fatalf("currentWindowsUserSID() error = %v", err)
	}
	return sid
}
