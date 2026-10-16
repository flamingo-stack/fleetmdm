//go:build !windows

package bitlocker

import "errors"

// ErrNotSupported is returned by COMWorker methods on non-Windows platforms
// to indicate that BitLocker operations are not supported on this platform.
var ErrNotSupported = errors.New("bitlocker: not supported on this platform")

// COMWorker is a no-op on non-Windows platforms.
type COMWorker struct{}

// NewCOMWorker returns a no-op COMWorker on non-Windows platforms.
func NewCOMWorker() (*COMWorker, error) { return &COMWorker{}, nil }

// Close is a no-op on non-Windows platforms.
func (w *COMWorker) Close() {}

// GetEncryptionStatus is a no-op on non-Windows platforms.
func (w *COMWorker) GetEncryptionStatus() ([]VolumeStatus, error) { return nil, ErrNotSupported }

// EncryptVolume is a no-op on non-Windows platforms.
func (w *COMWorker) EncryptVolume(string) (string, error) { return "", ErrNotSupported }

// RotateRecoveryKey is a no-op on non-Windows platforms.
func (w *COMWorker) RotateRecoveryKey(string) (string, error) { return "", ErrNotSupported }
