//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package install

// chownTreeLike 在不支持 Unix owner 语义的平台上保持 no-op。
func chownTreeLike(path string, ownerPath string) error {
	return nil
}
