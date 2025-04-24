// filepath.Walk

// ============================================================================

// Walk walks the file tree rooted at root, calling fn for each file or
// directory in the tree, including root.
//
// All errors that arise visiting files and directories are filtered by fn:
// see the [WalkFunc] documentation for details.
//
// The files are walked in lexical order, which makes the output deterministic
// but requires Walk to read an entire directory into memory before proceeding
// to walk that directory.
//
// Walk does not follow symbolic links.
//
// Walk is less efficient than [WalkDir], introduced in Go 1.16,
// which avoids calling os.Lstat on every visited file or directory.
func Walk(root string, fn WalkFunc) error {
	info, err := os.Lstat(root)
	if err != nil {
		err = fn(root, nil, err)
	} else {
		err = walk(root, info, fn)
	}
	if err == SkipDir || err == SkipAll {
		return nil
	}
	return err
}

// ============================================================================

// Lstat returns a [FileInfo] describing the named file.
// If the file is a symbolic link, the returned FileInfo
// describes the symbolic link. Lstat makes no attempt to follow the link.
// If there is an error, it will be of type [*PathError].
//
// On Windows, if the file is a reparse point that is a surrogate for another
// named entity (such as a symbolic link or mounted folder), the returned
// FileInfo describes the reparse point, and makes no attempt to resolve it.
func Lstat(name string) (FileInfo, error) {
	testlog.Stat(name)
	return lstatNolog(name)
}

// ============================================================================

// lstatNolog implements Lstat for Windows.
func lstatNolog(name string) (FileInfo, error) {
	followSurrogates := false
	if name != "" && IsPathSeparator(name[len(name)-1]) {
		// We try to implement POSIX semantics for Lstat path resolution
		// (per https://pubs.opengroup.org/onlinepubs/9699919799.2013edition/basedefs/V1_chap04.html#tag_04_12):
		// symlinks before the last separator in the path must be resolved. Since
		// the last separator in this case follows the last path element, we should
		// follow symlinks in the last path element.
		followSurrogates = true
	}
	return stat("Lstat", name, followSurrogates)
}

// ============================================================================
// stat_windows.go

// stat implements both Stat and Lstat of a file.
func stat(funcname, name string, followSurrogates bool) (FileInfo, error) {
	if len(name) == 0 {
		return nil, &PathError{Op: funcname, Path: name, Err: syscall.Errno(syscall.ERROR_PATH_NOT_FOUND)}
	}
	namep, err := syscall.UTF16PtrFromString(fixLongPath(name))
	if err != nil {
		return nil, &PathError{Op: funcname, Path: name, Err: err}
	}

	// Try GetFileAttributesEx first, because it is faster than CreateFile.
	// See https://golang.org/issues/19922#issuecomment-300031421 for details.
	var fa syscall.Win32FileAttributeData
	err = syscall.GetFileAttributesEx(namep, syscall.GetFileExInfoStandard, (*byte)(unsafe.Pointer(&fa)))
	if err == nil && fa.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
		// Not a surrogate for another named entity, because it isn't any kind of reparse point.
		// The information we got from GetFileAttributesEx is good enough for now.
		fs := newFileStatFromWin32FileAttributeData(&fa)
		if err := fs.saveInfoFromPath(name); err != nil {
			return nil, err
		}
		return fs, nil
	}

	// GetFileAttributesEx fails with ERROR_SHARING_VIOLATION error for
	// files like c:\pagefile.sys. Use FindFirstFile for such files.
	if err == windows.ERROR_SHARING_VIOLATION {
		var fd syscall.Win32finddata
		sh, err := syscall.FindFirstFile(namep, &fd)
		if err != nil {
			return nil, &PathError{Op: "FindFirstFile", Path: name, Err: err}
		}
		syscall.FindClose(sh)
		if fd.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
			// Not a surrogate for another named entity. FindFirstFile is good enough.
			fs := newFileStatFromWin32finddata(&fd)
			if err := fs.saveInfoFromPath(name); err != nil {
				return nil, err
			}
			return fs, nil
		}
	}

	// Use CreateFile to determine whether the file is a name surrogate and, if so,
	// save information about the link target.
	// Set FILE_FLAG_BACKUP_SEMANTICS so that CreateFile will create the handle
	// even if name refers to a directory.
	var flags uint32 = syscall.FILE_FLAG_BACKUP_SEMANTICS | syscall.FILE_FLAG_OPEN_REPARSE_POINT
	h, err := syscall.CreateFile(namep, 0, 0, nil, syscall.OPEN_EXISTING, flags, 0)

	if err == windows.ERROR_INVALID_PARAMETER {
		// Console handles, like "\\.\con", require generic read access. See
		// https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew#consoles.
		// We haven't set it previously because it is normally not required
		// to read attributes and some files may not allow it.
		h, err = syscall.CreateFile(namep, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, flags, 0)
	}
	if err != nil {
		// Since CreateFile failed, we can't determine whether name refers to a
		// name surrogate, or some other kind of reparse point. Since we can't return a
		// FileInfo with a known-accurate Mode, we must return an error.
		return nil, &PathError{Op: "CreateFile", Path: name, Err: err}
	}

	fi, err := statHandle(name, h)
	syscall.CloseHandle(h)
	if err == nil && followSurrogates && fi.(*fileStat).isReparseTagNameSurrogate() {
		// To obtain information about the link target, we reopen the file without
		// FILE_FLAG_OPEN_REPARSE_POINT and examine the resulting handle.
		// (See https://devblogs.microsoft.com/oldnewthing/20100212-00/?p=14963.)
		h, err = syscall.CreateFile(namep, 0, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if err != nil {
			// name refers to a symlink, but we couldn't resolve the symlink target.
			return nil, &PathError{Op: "CreateFile", Path: name, Err: err}
		}
		defer syscall.CloseHandle(h)
		return statHandle(name, h)
	}
	return fi, err
}

// ============================================================================
// types_windows.go

func (fs *fileStat) Mode() FileMode {
	m := fs.mode()
	if winsymlink.Value() == "0" {
		old := fs.modePreGo1_23()
		if old != m {
			winsymlink.IncNonDefault()
			m = old
		}
	}
	return m
}

func (fs *fileStat) mode() (m FileMode) {
	if fs.FileAttributes&syscall.FILE_ATTRIBUTE_READONLY != 0 {
		m |= 0444
	} else {
		m |= 0666
	}

	// Windows reports the FILE_ATTRIBUTE_DIRECTORY bit for reparse points
	// that refer to directories, such as symlinks and mount points.
	// However, we follow symlink POSIX semantics and do not set the mode bits.
	// This allows users to walk directories without following links
	// by just calling "fi, err := os.Lstat(name); err == nil && fi.IsDir()".
	// Note that POSIX only defines the semantics for symlinks, not for
	// mount points or other surrogate reparse points, but we treat them
	// the same way for consistency. Also, mount points can contain infinite
	// loops, so it is not safe to walk them without special handling.
	if !fs.isReparseTagNameSurrogate() {
		if fs.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0 {
			m |= ModeDir | 0111
		}

		switch fs.filetype {
		case syscall.FILE_TYPE_PIPE:
			m |= ModeNamedPipe
		case syscall.FILE_TYPE_CHAR:
			m |= ModeDevice | ModeCharDevice
		}
	}

	if fs.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		switch fs.ReparseTag {
		case syscall.IO_REPARSE_TAG_SYMLINK:
			m |= ModeSymlink
		case windows.IO_REPARSE_TAG_AF_UNIX:
			m |= ModeSocket
		case windows.IO_REPARSE_TAG_DEDUP:
			// If the Data Deduplication service is enabled on Windows Server, its
			// Optimization job may convert regular files to IO_REPARSE_TAG_DEDUP
			// whenever that job runs.
			//
			// However, DEDUP reparse points remain similar in most respects to
			// regular files: they continue to support random-access reads and writes
			// of persistent data, and they shouldn't add unexpected latency or
			// unavailability in the way that a network filesystem might.
			//
			// Go programs may use ModeIrregular to filter out unusual files (such as
			// raw device files on Linux, POSIX FIFO special files, and so on), so
			// to avoid files changing unpredictably from regular to irregular we will
			// consider DEDUP files to be close enough to regular to treat as such.
		default:
			m |= ModeIrregular
		}
	}
	return
}

// os.Chmod

// Chmod changes the mode of the named file to mode.
// If the file is a symbolic link, it changes the mode of the link's target.
// If there is an error, it will be of type *PathError.
//
// A different subset of the mode bits are used, depending on the
// operating system.
//
// On Unix, the mode's permission bits, ModeSetuid, ModeSetgid, and
// ModeSticky are used.
//
// On Windows, only the 0o200 bit (owner writable) of mode is used; it
// controls whether the file's read-only attribute is set or cleared.
// The other bits are currently unused. For compatibility with Go 1.12
// and earlier, use a non-zero mode. Use mode 0o400 for a read-only
// file and 0o600 for a readable+writable file.
//
// On Plan 9, the mode's permission bits, ModeAppend, ModeExclusive,
// and ModeTemporary are used.
func Chmod(name string, mode FileMode) error { return chmod(name, mode) }

// ============================================================================
// file_posix.go

// See docs in file.go:Chmod.
func chmod(name string, mode FileMode) error {
	longName := fixLongPath(name)
	e := ignoringEINTR(func() error {
		return syscall.Chmod(longName, syscallMode(mode))
	})
	if e != nil {
		return &PathError{Op: "chmod", Path: name, Err: e}
	}
	return nil
}

// ============================================================================
// syscall_windows.go

func Chmod(path string, mode uint32) (err error) {
	p, e := UTF16PtrFromString(path)
	if e != nil {
		return e
	}
	attrs, e := GetFileAttributes(p)
	if e != nil {
		return e
	}
	if mode&S_IWRITE != 0 {
		attrs &^= FILE_ATTRIBUTE_READONLY
	} else {
		attrs |= FILE_ATTRIBUTE_READONLY
	}
	return SetFileAttributes(p, attrs)
}