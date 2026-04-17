//go:build windows

package securefile

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	directoryAddFileMask     = 0x00000002
	directoryAddSubdirMask   = 0x00000004
	directoryDeleteChildMask = 0x00000040
)

var (
	validatePlatformFilePermissions = func(path string) error {
		return validateWindowsACL(path, fileRiskyAccessMask(), errors.New("must not grant read or write access to other Windows users"))
	}
	validatePlatformParentDirectoryPermissions = func(path string) error {
		return validateWindowsACL(path, directoryRiskyAccessMask(), errors.New("parent directory must not grant write access to other Windows users"))
	}
	windowsSecurityDescriptorForPath = func(path string) (*windows.SECURITY_DESCRIPTOR, error) {
		return windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	}
	currentWindowsUserSID = func() (*windows.SID, error) {
		token, err := windows.OpenCurrentProcessToken()
		if err != nil {
			return nil, err
		}
		defer token.Close()

		user, err := token.GetTokenUser()
		if err != nil {
			return nil, err
		}
		return user.User.Sid, nil
	}
)

func validateWindowsACL(path string, riskyMask windows.ACCESS_MASK, validationErr error) error {
	sd, err := windowsSecurityDescriptorForPath(path)
	if err != nil {
		return fmt.Errorf("read Windows ACL: %w", err)
	}
	if sd == nil || !sd.IsValid() {
		return validationErr
	}

	allowedSIDs, err := windowsAllowedPrincipalSet(sd)
	if err != nil {
		return fmt.Errorf("resolve allowed Windows principals: %w", err)
	}

	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return validationErr
	}

	for aceIndex := uint16(0); aceIndex < dacl.AceCount; aceIndex++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(aceIndex), &ace); err != nil {
			return fmt.Errorf("read Windows ACL entry: %w", err)
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		if ace.Mask&riskyMask == 0 {
			continue
		}

		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if _, ok := allowedSIDs[aceSID.String()]; ok {
			continue
		}
		return validationErr
	}

	return nil
}

func windowsAllowedPrincipalSet(sd *windows.SECURITY_DESCRIPTOR) (map[string]struct{}, error) {
	currentUserSID, err := currentWindowsUserSID()
	if err != nil {
		return nil, err
	}

	systemSID, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, err
	}
	adminSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return nil, err
	}

	allowedSIDs := make(map[string]struct{}, 3)
	for _, sid := range []*windows.SID{currentUserSID, systemSID, adminSID} {
		allowedSIDs[sid.String()] = struct{}{}
	}
	return allowedSIDs, nil
}

func fileRiskyAccessMask() windows.ACCESS_MASK {
	return windows.ACCESS_MASK(
		windows.FILE_READ_DATA |
			windows.FILE_READ_EA |
			windows.FILE_WRITE_DATA |
			windows.FILE_APPEND_DATA |
			windows.FILE_WRITE_EA |
			windows.FILE_WRITE_ATTRIBUTES |
			windows.DELETE |
			windows.WRITE_DAC |
			windows.WRITE_OWNER |
			windows.GENERIC_READ |
			windows.GENERIC_WRITE |
			windows.GENERIC_ALL |
			windows.MAXIMUM_ALLOWED,
	)
}

func directoryRiskyAccessMask() windows.ACCESS_MASK {
	return windows.ACCESS_MASK(
		directoryAddFileMask |
			directoryAddSubdirMask |
			directoryDeleteChildMask |
			windows.FILE_WRITE_EA |
			windows.FILE_WRITE_ATTRIBUTES |
			windows.DELETE |
			windows.WRITE_DAC |
			windows.WRITE_OWNER |
			windows.GENERIC_WRITE |
			windows.GENERIC_ALL |
			windows.MAXIMUM_ALLOWED,
	)
}
