// precommit checks various properties of the tree before commiting.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

func run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	if err := CheckUncommittedDocs(ctx); err != nil {
		return fmt.Errorf("main.CheckUncommittedDocs: %v", err)
	}
	return nil
}

func stdout(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.WaitDelay = time.Millisecond
	output, err := cmd.Output()
	if exiterr, ok := err.(*exec.ExitError); ok {
		output = append(output, exiterr.Stderr...)
		return "", fmt.Errorf("main.Execute %q: %v, stderr: %s", args, exiterr, bytes.TrimSpace(output))
	} else if err != nil {
		return "", fmt.Errorf("main.Execute %q: %v", args, err)
	}
	return string(output), nil
}

// all files from docs must be in the index.
func CheckUncommittedDocs(ctx context.Context) error {
	gitfilesOutput, err := stdout(ctx, "git", "ls-files", "docs/")
	if err != nil {
		return err
	}
	gitfiles := make(map[string]bool, 512)
	for _, f := range strings.Split(gitfilesOutput, "\n") {
		gitfiles[f] = true
	}

	fsfiles, err := filepath.Glob("docs/*")
	if err != nil {
		return fmt.Errorf("glob: %v", err)
	}
	var missing []string
	for _, f := range fsfiles {
		if !gitfiles[f] {
			missing = append(missing, f)
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("main.FoundUntrackedFiles files=%v", missing)
	}
	return nil
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Printf("precommit failed: %v\n", err)
		os.Exit(1)
	}
}
