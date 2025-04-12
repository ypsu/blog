package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"unsafe"
)

func startwatch(dirs ...string) (ifd int, err error) {
	ifd, err = syscall.InotifyInit()
	if err != nil {
		return 0, fmt.Errorf("devserve.InotifyInit: %v", err)
	}

	var mask uint32
	mask |= syscall.IN_CLOSE_WRITE
	mask |= syscall.IN_CREATE
	mask |= syscall.IN_DELETE
	mask |= syscall.IN_MOVED_FROM
	mask |= syscall.IN_MOVED_TO
	for _, dir := range dirs {
		if _, err := syscall.InotifyAddWatch(ifd, dir, mask); err != nil {
			return 0, fmt.Errorf("devserve.InotifyAddWatch: %v", err)
		}
	}
	return ifd, nil
}

func streamevents(ifd int, notify chan<- string) error {
	for {
		const bufsize = 1 << 16
		eventbuf := [bufsize]byte{}
		n, err := syscall.Read(ifd, eventbuf[:])
		if n <= 0 || err != nil {
			return fmt.Errorf("devserve.InotifyRead: %v", err)
		}

		for offset := 0; offset < n; {
			if n-offset < syscall.SizeofInotifyEvent {
				return fmt.Errorf("devserve.BadInotifyOffset n=%d offset=%d", n, offset)
			}
			event := (*syscall.InotifyEvent)(unsafe.Pointer(&eventbuf[offset]))
			namelen := int(event.Len)
			namebytes := (*[syscall.PathMax]byte)(unsafe.Pointer(&eventbuf[offset+syscall.SizeofInotifyEvent]))
			name := string(bytes.TrimRight(namebytes[0:namelen], "\000"))
			notify <- name
			offset += syscall.SizeofInotifyEvent + namelen
		}
	}
}

func updatePubdatesCache(ctx context.Context) error {
	if _, err := os.Stat("pubdates.cache"); err != nil {
		return fmt.Errorf("devserve.PubdatesCacheNotFound: %v", err)
	}
	pubdatesfile, err := os.OpenFile("pubdates.cache.tmp", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("devserve.OpenPubdatesCache: %v", err)
	}
	defer pubdatesfile.Close()
	pdcmd := exec.CommandContext(ctx, "go", "run", "blog/pubdates", "-postpath=docs")
	pdcmd.Stdout = pubdatesfile
	if err := pdcmd.Run(); err != nil {
		return fmt.Errorf("devserve.UpdatePubdatesCache: %v", err)
	}
	if err := pubdatesfile.Close(); err != nil {
		return fmt.Errorf("devserve.ClosePubdatesCache: %v", err)
	}
	if err := os.Rename("pubdates.cache.tmp", "pubdates.cache"); err != nil {
		return fmt.Errorf("devserve.RenamePubdatesCache: %v", err)
	}
	return nil
}

func run(ctx context.Context) error {
	if _, err := os.Stat("../pubdates.cache"); err == nil {
		os.Chdir("..")
	}

	if err := updatePubdatesCache(ctx); err != nil {
		return fmt.Errorf("devserve.InitialUpdatePubdatesCache: %v", err)
	}

	for {
		w := &strings.Builder{}
		buildcmd := exec.CommandContext(ctx, "go", "build", "blog")
		buildcmd.Stdout = w
		buildcmd.Stderr = w
		err := buildcmd.Run()
		fmt.Print("\033[H\033[J")
		if err == nil {
			break
		}
		fmt.Printf("devserve.BuildBlog: %v\n%s\n", err, w)
		dirs, err := filepath.Glob("*/*.go")
		if err != nil {
			return fmt.Errorf("devserve.BuildGlob: %v", err)
		}
		for i, f := range dirs {
			dirs[i] = filepath.Dir(f)
		}
		dirs = append(dirs, ".")
		slices.Sort(dirs)
		ifd, err := startwatch(slices.Compact(dirs)...)
		if err != nil {
			return fmt.Errorf("devserve.StartBuildWatch: %v", err)
		}
		buildevent := make(chan string)
		go func() { streamevents(ifd, buildevent) }()
		select {
		case <-buildevent:
		case <-ctx.Done():
			return fmt.Errorf("devserve.SigintWhileBuild")
		}
		if err := syscall.Close(ifd); err != nil {
			return fmt.Errorf("devserve.CloseBuildIFD: %v", err)
		}
		fmt.Printf("devserve.BuildStarted\n")
	}

	events := make(chan string, 128)
	ifd, err := startwatch("docs")
	if err != nil {
		return fmt.Errorf("devserve.Startwatch: %v", err)
	}
	go func() {
		if err := streamevents(ifd, events); err != nil {
			fmt.Printf("devserve.StreamEvents (ignoring error): %v", err)
		}
	}()

	blogdone := make(chan error)
	args := append([]string{"-alogdb=./test.alogdb"}, os.Args[1:]...)
	blogcmd := exec.Command("./blog", args...)
	blogcmd.Stdout = os.Stdout
	blogcmd.Stderr = os.Stderr
	if err := blogcmd.Start(); err != nil {
		return fmt.Errorf("devserve.StartBlog: %v", err)
	}
	go func() {
		blogdone <- blogcmd.Wait()
	}()

	for {
		select {
		case <-events:
			// Drain all events.
		drainloop:
			for {
				select {
				case <-events:
				default:
					break drainloop
				}
			}
			if err := updatePubdatesCache(ctx); err != nil {
				return fmt.Errorf("devserve.RegularUpdatePubdatesCache: %v", err)
			}
			if err := blogcmd.Process.Signal(syscall.SIGINT); err != nil {
				return fmt.Errorf("devserve.SignalInt: %v", err)
			}

		case err := <-blogdone:
			return fmt.Errorf("devserve.BlogExited: %v", err)

		case <-ctx.Done():
			fmt.Printf("devserve.SigintReceived (gracefully shutting down)\n")
			if err := blogcmd.Process.Signal(syscall.SIGQUIT); err != nil {
				return fmt.Errorf("devserve.SignalQuit: %v", err)
			}
			if err := <-blogdone; err != nil {
				return fmt.Errorf("devserve.BlogWait: %v", err)
			}
			return nil
		}
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT)
	defer stop()

	go func() {
		sigquitch := make(chan os.Signal, 1)
		signal.Notify(sigquitch, syscall.SIGQUIT)
		<-sigquitch
		fmt.Printf("devserve.SigquitReceived (quitting without cleanup)\n")
		os.Exit(3)
	}()

	if err := run(ctx); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
