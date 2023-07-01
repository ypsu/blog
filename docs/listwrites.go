// listwrites lists all write operations happening under a directory.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"syscall"
	"unsafe"
)

var dirFlag = flag.String("dir", ".", "the directory under which to detect the writes.")

func run() error {
	flag.Parse()
	log.Printf("recursively initializing inotify rooted at %q.", *dirFlag)
	watches := map[int]string{}
	ifd, err := syscall.InotifyInit()
	if err != nil {
		return fmt.Errorf("inotify init: %w", err)
	}

	var watchpath func(string) error
	watchpath = func(dirpath string) error {
		var mask uint32
		mask |= syscall.IN_MODIFY
		mask |= syscall.IN_CREATE
		mask |= syscall.IN_DELETE
		mask |= syscall.IN_MOVED_FROM
		mask |= syscall.IN_MOVED_TO
		mask |= syscall.IN_DONT_FOLLOW
		mask |= syscall.IN_EXCL_UNLINK
		mask |= syscall.IN_ONLYDIR
		var wd int
		if wd, err = syscall.InotifyAddWatch(ifd, dirpath, mask); err != nil {
			return fmt.Errorf("inotify_add_watch %s: %w", dirpath, err)
		}
		watches[wd] = dirpath

		var walkErr error
		walkfunc := func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				log.Printf("[warning] can't walk %s: %v, skipping.", path, err)
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			if path == dirpath {
				return nil
			}
			if err := watchpath(path); err != nil {
				walkErr = err
			}
			return fs.SkipDir
		}
		filepath.WalkDir(dirpath, walkfunc)
		return walkErr
	}
	if err := watchpath(*dirFlag); err != nil {
		return fmt.Errorf("directory walk: %w", err)
	}

	log.Print("watching inotify events.")
	for {
		const bufsize = 16384
		eventbuf := [bufsize]byte{}
		n, err := syscall.Read(ifd, eventbuf[:])
		if n <= 0 || err != nil {
			return fmt.Errorf("inotify read: %w", err)
		}
		for offset := 0; offset < n; {
			if n-offset < syscall.SizeofInotifyEvent {
				return fmt.Errorf("invalid inotify read: n:%d offset:%d", n, offset)
			}
			event := (*syscall.InotifyEvent)(unsafe.Pointer(&eventbuf[offset]))
			wd := int(event.Wd)
			mask := int(event.Mask)
			namelen := int(event.Len)
			namebytes := (*[syscall.PathMax]byte)(unsafe.Pointer(&eventbuf[offset+syscall.SizeofInotifyEvent]))
			name := string(bytes.TrimRight(namebytes[0:namelen], "\000"))
			dir, ok := watches[wd]
			if !ok {
				return fmt.Errorf("unknown watch descriptor %d.", wd)
			}
			name = path.Join(dir, name)
			if mask&syscall.IN_IGNORED != 0 {
				delete(watches, wd)
			}
			if mask&syscall.IN_CREATE != 0 || mask&syscall.IN_MOVED_TO != 0 {
				fi, err := os.Stat(name)
				if err == nil && fi.IsDir() {
					watchpath(name)
				}
			}
			log.Printf("modified: %s", name)
			offset += syscall.SizeofInotifyEvent + namelen
		}
	}

}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
