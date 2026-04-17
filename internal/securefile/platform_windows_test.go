//go:build windows

package securefile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestValidateAndReadSingleTokenRejectWindowsInstallerUserWhenRuntimeUserDiffers(t *testing.T) {
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

	systemSID, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		t.Fatalf("CreateWellKnownSid(WinLocalSystemSid) error = %v", err)
	}

	restoreCurrentWindowsUserSID := currentWindowsUserSID
	currentWindowsUserSID = func() (*windows.SID, error) {
		return systemSID, nil
	}
	t.Cleanup(func() {
		currentWindowsUserSID = restoreCurrentWindowsUserSID
	})

	if err := Validate(tokenPath); err == nil {
		t.Fatal("Validate() error = nil, want runtime-user mismatch rejection")
	}
	if _, err := ReadSingleToken(tokenPath); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want runtime-user mismatch rejection")
	}
}

func TestValidateWindowsACLReturnsDescriptorReadError(t *testing.T) {
	originalWindowsSecurityDescriptorForPath := windowsSecurityDescriptorForPath
	t.Cleanup(func() {
		windowsSecurityDescriptorForPath = originalWindowsSecurityDescriptorForPath
	})

	injectedErr := windows.ERROR_ACCESS_DENIED
	windowsSecurityDescriptorForPath = func(string) (*windows.SECURITY_DESCRIPTOR, error) {
		return nil, injectedErr
	}

	err := validateWindowsACL("ignored", fileRiskyAccessMask(), errors.New("validation error"))
	if err == nil {
		t.Fatal("validateWindowsACL() error = nil, want descriptor read error")
	}
	if !errors.Is(err, injectedErr) {
		t.Fatalf("validateWindowsACL() error = %v, want wrapped %v", err, injectedErr)
	}
	if !strings.Contains(err.Error(), "read Windows ACL") {
		t.Fatalf("validateWindowsACL() error = %v, want read Windows ACL prefix", err)
	}
}

func TestValidateWindowsACLRejectsNilDescriptor(t *testing.T) {
	originalWindowsSecurityDescriptorForPath := windowsSecurityDescriptorForPath
	t.Cleanup(func() {
		windowsSecurityDescriptorForPath = originalWindowsSecurityDescriptorForPath
	})

	windowsSecurityDescriptorForPath = func(string) (*windows.SECURITY_DESCRIPTOR, error) {
		return nil, nil
	}

	validationErr := errors.New("validation error")
	if err := validateWindowsACL("ignored", fileRiskyAccessMask(), validationErr); !errors.Is(err, validationErr) {
		t.Fatalf("validateWindowsACL() error = %v, want %v", err, validationErr)
	}
}

func TestValidateWindowsACLPropagatesPrincipalResolutionError(t *testing.T) {
	originalWindowsSecurityDescriptorForPath := windowsSecurityDescriptorForPath
	originalCurrentWindowsUserSID := currentWindowsUserSID
	t.Cleanup(func() {
		windowsSecurityDescriptorForPath = originalWindowsSecurityDescriptorForPath
		currentWindowsUserSID = originalCurrentWindowsUserSID
	})

	windowsSecurityDescriptorForPath = func(string) (*windows.SECURITY_DESCRIPTOR, error) {
		return mustWindowsSecurityDescriptorFromString(t, "D:P(A;;GA;;;SY)"), nil
	}

	injectedErr := errors.New("lookup failed")
	currentWindowsUserSID = func() (*windows.SID, error) {
		return nil, injectedErr
	}

	err := validateWindowsACL("ignored", fileRiskyAccessMask(), errors.New("validation error"))
	if err == nil {
		t.Fatal("validateWindowsACL() error = nil, want principal resolution error")
	}
	if !errors.Is(err, injectedErr) {
		t.Fatalf("validateWindowsACL() error = %v, want wrapped %v", err, injectedErr)
	}
	if !strings.Contains(err.Error(), "resolve allowed Windows principals") {
		t.Fatalf("validateWindowsACL() error = %v, want principal resolution prefix", err)
	}
}

func TestValidateWindowsACLRejectsMissingDACL(t *testing.T) {
	originalWindowsSecurityDescriptorForPath := windowsSecurityDescriptorForPath
	t.Cleanup(func() {
		windowsSecurityDescriptorForPath = originalWindowsSecurityDescriptorForPath
	})

	sd, err := windows.NewSecurityDescriptor()
	if err != nil {
		t.Fatalf("NewSecurityDescriptor() error = %v", err)
	}
	windowsSecurityDescriptorForPath = func(string) (*windows.SECURITY_DESCRIPTOR, error) {
		return sd, nil
	}

	validationErr := errors.New("validation error")
	if err := validateWindowsACL("ignored", fileRiskyAccessMask(), validationErr); !errors.Is(err, validationErr) {
		t.Fatalf("validateWindowsACL() error = %v, want %v", err, validationErr)
	}
}

func TestValidateWindowsACLSkipsInheritOnlyAndDenyEntries(t *testing.T) {
	originalWindowsSecurityDescriptorForPath := windowsSecurityDescriptorForPath
	originalCurrentWindowsUserSID := currentWindowsUserSID
	t.Cleanup(func() {
		windowsSecurityDescriptorForPath = originalWindowsSecurityDescriptorForPath
		currentWindowsUserSID = originalCurrentWindowsUserSID
	})

	systemSID := mustWellKnownSID(t, windows.WinLocalSystemSid)
	windowsSecurityDescriptorForPath = func(string) (*windows.SECURITY_DESCRIPTOR, error) {
		return mustWindowsSecurityDescriptorFromString(t, "D:P(A;IO;GA;;;BU)(D;;GA;;;BU)(A;;GA;;;SY)"), nil
	}
	currentWindowsUserSID = func() (*windows.SID, error) {
		return systemSID, nil
	}

	if err := validateWindowsACL("ignored", fileRiskyAccessMask(), errors.New("validation error")); err != nil {
		t.Fatalf("validateWindowsACL() error = %v", err)
	}
}

func TestValidateWindowsACLReturnsEntryReadError(t *testing.T) {
	originalWindowsSecurityDescriptorForPath := windowsSecurityDescriptorForPath
	originalCurrentWindowsUserSID := currentWindowsUserSID
	originalWindowsGetACE := windowsGetACE
	t.Cleanup(func() {
		windowsSecurityDescriptorForPath = originalWindowsSecurityDescriptorForPath
		currentWindowsUserSID = originalCurrentWindowsUserSID
		windowsGetACE = originalWindowsGetACE
	})

	systemSID := mustWellKnownSID(t, windows.WinLocalSystemSid)
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{
		explicitAccessEntryForSID(systemSID, windows.READ_CONTROL),
	}, nil)
	if err != nil {
		t.Fatalf("ACLFromEntries() error = %v", err)
	}
	acl.AceCount = 2

	sd, err := windows.NewSecurityDescriptor()
	if err != nil {
		t.Fatalf("NewSecurityDescriptor() error = %v", err)
	}
	if err := sd.SetDACL(acl, true, false); err != nil {
		t.Fatalf("SetDACL() error = %v", err)
	}

	windowsSecurityDescriptorForPath = func(string) (*windows.SECURITY_DESCRIPTOR, error) {
		return sd, nil
	}
	currentWindowsUserSID = func() (*windows.SID, error) {
		return systemSID, nil
	}
	windowsGetACE = func(acl *windows.ACL, aceIndex uint32, ace **windows.ACCESS_ALLOWED_ACE) error {
		if aceIndex == 0 {
			return originalWindowsGetACE(acl, aceIndex, ace)
		}
		return windows.ERROR_INVALID_ACL
	}

	err = validateWindowsACL("ignored", fileRiskyAccessMask(), errors.New("validation error"))
	if err == nil {
		t.Fatal("validateWindowsACL() error = nil, want ACL entry read error")
	}
	if !strings.Contains(err.Error(), "read Windows ACL entry") {
		t.Fatalf("validateWindowsACL() error = %v, want ACL entry read prefix", err)
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

func mustWindowsSecurityDescriptorFromString(t *testing.T, sddl string) *windows.SECURITY_DESCRIPTOR {
	t.Helper()

	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatalf("SecurityDescriptorFromString(%q) error = %v", sddl, err)
	}
	return sd
}

func mustWellKnownSID(t *testing.T, sidType windows.WELL_KNOWN_SID_TYPE) *windows.SID {
	t.Helper()

	sid, err := windows.CreateWellKnownSid(sidType)
	if err != nil {
		t.Fatalf("CreateWellKnownSid(%v) error = %v", sidType, err)
	}
	return sid
}

func currentWindowsUserSIDForTest(t *testing.T) *windows.SID {
	t.Helper()

	sid, err := currentWindowsUserSID()
	if err != nil {
		t.Fatalf("currentWindowsUserSID() error = %v", err)
	}
	return sid
}
