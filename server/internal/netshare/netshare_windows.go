package netshare

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Supported: Windows signs in to SMB shares for the process.
const Supported = true

var (
	mpr                   = windows.NewLazySystemDLL("mpr.dll")
	procAddConnection2    = mpr.NewProc("WNetAddConnection2W")
	procCancelConnection2 = mpr.NewProc("WNetCancelConnection2W")
	errCredentialConflict = windows.Errno(1219) // ERROR_SESSION_CREDENTIAL_CONFLICT
	errAlreadyAssigned    = windows.Errno(85)   // ERROR_ALREADY_ASSIGNED
	errLogonFailure       = windows.Errno(1326) // ERROR_LOGON_FAILURE
	errBadNetPath         = windows.Errno(53)   // ERROR_BAD_NETPATH
	errBadNetName         = windows.Errno(67)   // ERROR_BAD_NET_NAME
	errInvalidPassword    = windows.Errno(86)   // ERROR_INVALID_PASSWORD
	errAccountRestriction = windows.Errno(1327) // ERROR_ACCOUNT_RESTRICTION
	errNoNetwork          = windows.Errno(1222) // ERROR_NO_NETWORK
	resourceTypeDisk      = uint32(1)           // RESOURCETYPE_DISK
	connectTemporary      = uint32(4)           // CONNECT_TEMPORARY: not remembered for the next logon
)

// netResource is NETRESOURCEW.
type netResource struct {
	Scope, Type, DisplayType, Usage          uint32
	LocalName, RemoteName, Comment, Provider *uint16
}

// Connect signs in to the share path is on. A share already connected with
// other credentials (Windows allows one set per server) is disconnected
// first and tried again.
func Connect(path, user, password string) error {
	root, err := Root(path)
	if err != nil {
		return err
	}
	err = addConnection(root, user, password)
	if errors.Is(err, errCredentialConflict) {
		_ = Disconnect(root)
		err = addConnection(root, user, password)
	}
	if errors.Is(err, errAlreadyAssigned) {
		return nil
	}
	return describe(root, err)
}

func addConnection(root, user, password string) error {
	remote, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return err
	}
	nr := netResource{Type: resourceTypeDisk, RemoteName: remote}
	var u, p *uint16
	if user != "" {
		if u, err = windows.UTF16PtrFromString(user); err != nil {
			return err
		}
	}
	if p, err = windows.UTF16PtrFromString(password); err != nil {
		return err
	}
	r, _, _ := procAddConnection2.Call(uintptr(unsafe.Pointer(&nr)), uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(u)), uintptr(connectTemporary))
	if r != 0 {
		return windows.Errno(r)
	}
	return nil
}

// Disconnect drops the server's sign-in to a share.
func Disconnect(path string) error {
	root, err := Root(path)
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return err
	}
	r, _, _ := procCancelConnection2.Call(uintptr(unsafe.Pointer(name)), 0, 1)
	if r != 0 {
		return windows.Errno(r)
	}
	return nil
}

// describe turns Windows' error codes into what to do about them.
func describe(root string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errLogonFailure), errors.Is(err, errInvalidPassword):
		return fmt.Errorf("%s turned down the user name or password", root)
	case errors.Is(err, errAccountRestriction):
		return fmt.Errorf("%s won't accept that account (a blank password, or sign-in hours)", root)
	case errors.Is(err, errBadNetPath), errors.Is(err, errNoNetwork):
		return fmt.Errorf("can't reach %s: check the computer's name and that it's on", root)
	case errors.Is(err, errBadNetName):
		return fmt.Errorf("%s has no share by that name", root)
	}
	return fmt.Errorf("%s: %w", root, err)
}

// Protect seals a password with DPAPI for the account the server runs as,
// so the database (or a backup of it) is no use on another machine.
func Protect(password string) ([]byte, error) {
	if password == "" {
		return nil, nil
	}
	in := windows.DataBlob{Size: uint32(len(password)), Data: unsafe.StringData(password)}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

// Unprotect opens a password sealed by Protect. It fails when the server now
// runs as another account or on another machine: the password needs typing again.
func Unprotect(secret []byte) (string, error) {
	if len(secret) == 0 {
		return "", nil
	}
	in := windows.DataBlob{Size: uint32(len(secret)), Data: &secret[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return "", errors.New("the saved password can't be read (the service now runs as another account?): type it again")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return string(unsafe.Slice(out.Data, out.Size)), nil
}
