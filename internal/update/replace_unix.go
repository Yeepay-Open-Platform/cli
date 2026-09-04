//go:build !windows

package update

func (u *Updater) PrepareSelfReplace() (func(), error) { return func() {}, nil }
func (u *Updater) CleanupStaleFiles()                  {}
func (u *Updater) CanRestorePreviousVersion() bool     { return u.backupCreated }
