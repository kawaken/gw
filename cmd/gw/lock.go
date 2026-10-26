package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A gw lock declares that a process (one CLI run started by an external tool)
// is working in a worktree. It is stored as one file per worktree so locks on
// different worktrees never race on a shared file.

type lockRecord struct {
	Path     string `json:"path"`
	PID      int    `json:"pid"`
	LockedAt string `json:"locked_at"`
}

// LockState is the gw lock of a worktree as reported in JSON output. Alive is
// false when the locking process no longer exists (a stale lock), which does
// not protect the worktree.
type LockState struct {
	PID      int    `json:"pid"`
	LockedAt string `json:"locked_at"`
	Alive    bool   `json:"alive"`
}

// processAlive reports whether a process with pid exists. It is a var so tests
// can replace it.
var processAlive = pidAlive

func runLock(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lock", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pid := fs.Int("pid", 0, "process ID working in the worktree")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) != 1 || *pid <= 0 {
		fmt.Fprintln(stderr, "usage: gw lock <worktree> --pid <pid>")
		return 2
	}
	record, err := resolveTargetRecord(positional[0])
	if err != nil {
		return printCommandError(stderr, err)
	}
	if !processAlive(*pid) {
		return printCommandError(stderr, fmt.Errorf("process %d is not running", *pid))
	}
	existing, err := readLock(record.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return printCommandError(stderr, err)
	}
	if err == nil && existing.PID != *pid && processAlive(existing.PID) {
		return printCommandError(stderr, fmt.Errorf("%s is already locked by running process %d", record.Path, existing.PID))
	}
	lock := lockRecord{Path: record.Path, PID: *pid, LockedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := writeLock(lock); err != nil {
		return printCommandError(stderr, err)
	}
	return 0
}

func runUnlock(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("unlock", flag.ContinueOnError)
	fs.SetOutput(stderr)
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) != 1 {
		fmt.Fprintln(stderr, "usage: gw unlock <worktree>")
		return 2
	}
	record, err := resolveTargetRecord(positional[0])
	if err != nil {
		return printCommandError(stderr, err)
	}
	if err := removeLock(record.Path); err != nil {
		return printCommandError(stderr, err)
	}
	return 0
}

// resolveTargetRecord finds a worktree of the current repository by path,
// branch name, or directory name.
func resolveTargetRecord(query string) (worktreeRecord, error) {
	repoPath, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return worktreeRecord{}, fmt.Errorf("not inside a Git repository: %w", err)
	}
	records, err := gitWorktrees(mustAbs(strings.TrimSpace(repoPath)))
	if err != nil {
		return worktreeRecord{}, err
	}
	return matchTarget(records, query)
}

// matchTarget matches query against worktree paths first, then branch names,
// then directory names. A directory name shared by several worktrees is
// rejected as ambiguous.
func matchTarget(records []worktreeRecord, query string) (worktreeRecord, error) {
	if query == "" {
		return worktreeRecord{}, errors.New("worktree must not be empty")
	}
	absQuery := mustAbs(query)
	resolvedQuery := resolvePath(query)
	for _, record := range records {
		if record.Path == absQuery || record.Path == resolvedQuery {
			return record, nil
		}
	}
	for _, record := range records {
		if record.Branch != "" && record.Branch == query {
			return record, nil
		}
	}
	var matches []worktreeRecord
	for _, record := range records {
		if filepath.Base(record.Path) == query {
			matches = append(matches, record)
		}
	}
	switch len(matches) {
	case 0:
		return worktreeRecord{}, fmt.Errorf("worktree %q not found", query)
	case 1:
		return matches[0], nil
	default:
		return worktreeRecord{}, fmt.Errorf("worktree %q is ambiguous; specify its path", query)
	}
}

// lockStateFor returns the gw lock state of a worktree, or nil when it has no
// lock.
func lockStateFor(path string) *LockState {
	lock, err := readLock(path)
	if err != nil {
		return nil
	}
	return &LockState{PID: lock.PID, LockedAt: lock.LockedAt, Alive: processAlive(lock.PID)}
}

func lockFilePath(worktreePath string) (string, error) {
	digest := sha256.Sum256([]byte(mustAbs(worktreePath)))
	return statePath(filepath.Join("locks", fmt.Sprintf("%x.json", digest)))
}

func readLock(worktreePath string) (lockRecord, error) {
	path, err := lockFilePath(worktreePath)
	if err != nil {
		return lockRecord{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return lockRecord{}, os.ErrNotExist
		}
		return lockRecord{}, fmt.Errorf("read lock: %w", err)
	}
	var lock lockRecord
	if err := json.Unmarshal(data, &lock); err != nil {
		return lockRecord{}, fmt.Errorf("parse lock: %w", err)
	}
	if lock.Path != mustAbs(worktreePath) {
		return lockRecord{}, os.ErrNotExist
	}
	return lock, nil
}

func writeLock(lock lockRecord) error {
	path, err := lockFilePath(lock.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create lock directory: %w", err)
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("encode lock: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".lock-*.tmp")
	if err != nil {
		return fmt.Errorf("create lock temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write lock: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close lock: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace lock: %w", err)
	}
	return nil
}

func removeLock(worktreePath string) error {
	path, err := lockFilePath(worktreePath)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove lock: %w", err)
	}
	return nil
}
