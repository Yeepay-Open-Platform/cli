//go:build windows

package update

import "os"

func (u *Updater) PrepareSelfReplace() (func(), error) {
	executable, err := u.resolveExecutable()
	if err != nil {
		return func() {}, nil
	}
	oldPath := executable + ".old"
	_ = os.Remove(oldPath)
	if err := os.Rename(executable, oldPath); err != nil {
		return func() {}, err
	}
	u.backupCreated = true
	return func() {
		if _, err := os.Stat(oldPath); err != nil {
			u.backupCreated = false
			return
		}
		_ = os.Remove(executable)
		if os.Rename(oldPath, executable) != nil {
			u.backupCreated = false
		}
	}, nil
}

func (u *Updater) CleanupStaleFiles() {
	executable, err := u.resolveExecutable()
	if err != nil {
		return
	}
	oldPath := executable + ".old"
	if _, err := os.Stat(oldPath); err != nil {
		return
	}
	if _, err := os.Stat(executable); err != nil {
		_ = os.Rename(oldPath, executable)
		return
	}
	_ = os.Remove(oldPath)
}

func (u *Updater) CanRestorePreviousVersion() bool { return u.backupCreated }
