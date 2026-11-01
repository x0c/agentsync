//go:build windows

package agentsync

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const (
	fsctlSetReparsePoint   = 0x900A4
	ioReparseTagMountPoint = 0xA0000003
	maxReparseBufferSize   = 16 * 1024
)

// createJunction creates an NTFS directory junction at link pointing to
// source, using only the Go standard library (syscall + os). Junctions apply
// only on Windows and only to directories; they require no symlink privilege
// but do require an NTFS volume.
func createJunction(source, link string) error {
	absSource, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	info, err := os.Stat(absSource)
	if err != nil {
		return fmt.Errorf("junction source: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("junction source is not a directory: %s", absSource)
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	// The link itself must be an empty directory before the reparse point is set.
	if err := os.Mkdir(link, 0o755); err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(link)
		}
	}()

	substitute := `\??\` + absSource
	printName := absSource
	subUTF16 := syscall.StringToUTF16(substitute)
	printUTF16 := syscall.StringToUTF16(printName)
	subLen := (len(subUTF16) - 1) * 2
	printLen := (len(printUTF16) - 1) * 2
	subOff := 0
	printOff := len(subUTF16) * 2
	pathBytes := (len(subUTF16) + len(printUTF16)) * 2
	reparseDataLen := 8 + pathBytes
	if reparseDataLen+8 > maxReparseBufferSize {
		return fmt.Errorf("junction target path too long: %s", absSource)
	}
	buf := make([]byte, 8+reparseDataLen)
	binary.LittleEndian.PutUint32(buf[0:4], ioReparseTagMountPoint)
	binary.LittleEndian.PutUint16(buf[4:6], uint16(reparseDataLen))
	binary.LittleEndian.PutUint16(buf[6:8], 0)
	binary.LittleEndian.PutUint16(buf[8:10], uint16(subOff))
	binary.LittleEndian.PutUint16(buf[10:12], uint16(subLen))
	binary.LittleEndian.PutUint16(buf[12:14], uint16(printOff))
	binary.LittleEndian.PutUint16(buf[14:16], uint16(printLen))
	off := 16
	for _, v := range subUTF16 {
		binary.LittleEndian.PutUint16(buf[off:off+2], v)
		off += 2
	}
	for _, v := range printUTF16 {
		binary.LittleEndian.PutUint16(buf[off:off+2], v)
		off += 2
	}

	linkPtr, err := syscall.UTF16PtrFromString(link)
	if err != nil {
		return err
	}
	handle, err := syscall.CreateFile(linkPtr, syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OPEN_REPARSE_POINT|syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(handle)
	var bytesReturned uint32
	if err := syscall.DeviceIoControl(handle, fsctlSetReparsePoint, &buf[0], uint32(len(buf)), nil, 0, &bytesReturned, nil); err != nil {
		return err
	}
	cleanup = false
	return nil
}
