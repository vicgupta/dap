//go:build windows

package secrets

import "golang.org/x/sys/windows"

// tightenSecret gives the current user read and write on the file and drops
// inherited access. It does not ask for administrator rights. The owner can
// still change the DACL later.
func tightenSecret(path string) error {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	sid, err := user.User.Sid.Copy()
	if err != nil {
		return err
	}
	entries := []windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.DELETE,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}}
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
