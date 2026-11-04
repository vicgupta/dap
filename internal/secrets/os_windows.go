//go:build windows

package secrets

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// credMan used to call Credential Manager. DAP no longer writes there.
type credMan struct{}

func newPlatformKeeper() (Keeper, error) {
	return nil, errors.New("secrets stay in the data directory")
}

func (credMan) Backend() string { return "credential-manager" }

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
)

type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

var (
	advapi32       = windows.NewLazySystemDLL("advapi32.dll")
	procCredWrite  = advapi32.NewProc("CredWriteW")
	procCredRead   = advapi32.NewProc("CredReadW")
	procCredDelete = advapi32.NewProc("CredDeleteW")
	procCredFree   = advapi32.NewProc("CredFree")
)

func targetOf(service, account string) string {
	return "DAP:" + service + ":" + account
}

func (credMan) Set(service, account, secret string) error {
	target, err := windows.UTF16PtrFromString(targetOf(service, account))
	if err != nil {
		return err
	}
	user, err := windows.UTF16PtrFromString(account)
	if err != nil {
		return err
	}
	comment, err := windows.UTF16PtrFromString("DAP secret")
	if err != nil {
		return err
	}
	blob := []byte(secret)
	cred := credential{
		Type:       credTypeGeneric,
		TargetName: target,
		Comment:    comment,
		UserName:   user,
		Persist:    credPersistLocalMachine,
	}
	if len(blob) > 0 {
		cred.CredentialBlobSize = uint32(len(blob))
		cred.CredentialBlob = &blob[0]
	}
	r, _, callErr := procCredWrite.Call(uintptr(unsafe.Pointer(&cred)), 0)
	if r == 0 {
		return fmt.Errorf("CredWrite: %w", callErr)
	}
	return nil
}

func (credMan) Get(service, account string) (string, error) {
	target, err := windows.UTF16PtrFromString(targetOf(service, account))
	if err != nil {
		return "", err
	}
	var out uintptr
	r, _, callErr := procCredRead.Call(uintptr(unsafe.Pointer(target)), uintptr(credTypeGeneric), 0, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return "", ErrNotFound
		}
		return "", ErrNotFound
	}
	defer procCredFree.Call(out)
	cred := (*credential)(unsafe.Pointer(out))
	if cred.CredentialBlobSize == 0 || cred.CredentialBlob == nil {
		return "", nil
	}
	buf := unsafe.Slice(cred.CredentialBlob, cred.CredentialBlobSize)
	return string(append([]byte(nil), buf...)), nil
}

func (credMan) Delete(service, account string) error {
	target, err := windows.UTF16PtrFromString(targetOf(service, account))
	if err != nil {
		return err
	}
	r, _, callErr := procCredDelete.Call(uintptr(unsafe.Pointer(target)), uintptr(credTypeGeneric), 0)
	if r == 0 {
		return fmt.Errorf("CredDelete: %w", callErr)
	}
	return nil
}
