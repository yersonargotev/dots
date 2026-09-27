// Package batcache activates dots' reviewed bat theme through bat's native
// custom-asset cache without adopting the generated cache as managed state.
package batcache

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	configRelative = ".config/bat/config"
	cacheRelative  = ".cache/bat"
	themeRelative  = ".config/bat/themes/Carbonfox.tmTheme"
)

var generatedFiles = []string{"metadata.yaml", "syntaxes.bin", "themes.bin"}

// Input contains the selected-home boundary and the exact Managed Entry bytes
// captured before apply. BaseEnv is sanitized before every native invocation.
type Input struct {
	Home    string
	BaseEnv []string
	Config  []byte
	Theme   []byte
	Runner  CommandRunner
	testOps *operationHooks
}

type operationHooks struct {
	flock              func(int, int) error
	mkdirTemp          func(string, string) (string, error)
	beforeStageCleanup func(string) error
	cleanupStage       func(*os.File) error
	publish            publishOperations
}

func defaultOperationHooks() *operationHooks {
	return &operationHooks{
		flock: unix.Flock, mkdirTemp: os.MkdirTemp, beforeStageCleanup: func(string) error { return nil },
		cleanupStage: removeDirectoryContents, publish: realPublishOperations{},
	}
}

type syncWriteCloser interface {
	io.Writer
	Sync() error
	Close() error
}

type syncCloser interface {
	Sync() error
	Close() error
}

type publishOperations interface {
	create(*os.File, string) (syncWriteCloser, error)
	remove(*os.File, string) error
	rename(*os.File, string, string) error
	openDir(*os.File) (syncCloser, error)
}

type realPublishOperations struct{}

func (realPublishOperations) create(cacheDir *os.File, name string) (syncWriteCloser, error) {
	fd, err := unix.Openat(int(cacheDir.Fd()), name, unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}
func (realPublishOperations) remove(cacheDir *os.File, name string) error {
	return unix.Unlinkat(int(cacheDir.Fd()), name, 0)
}
func (realPublishOperations) rename(cacheDir *os.File, oldName, newName string) error {
	return unix.Renameat(int(cacheDir.Fd()), oldName, int(cacheDir.Fd()), newName)
}
func (realPublishOperations) openDir(cacheDir *os.File) (syncCloser, error) {
	fd, err := unix.Dup(int(cacheDir.Fd()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), cacheDir.Name()), nil
}

// Receipt is returned only after the generated cache passes native loading and
// rendering from the actual selected-home paths.
type Receipt struct {
	Executable string
	CacheDir   string
	Hashes     map[string]string
}

// CommandRunner is the deterministic process boundary used by tests.
type CommandRunner interface {
	Run(context.Context, string, []string, []string, []byte, string) ([]byte, []byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, executable string, args, env []string, stdin []byte, dir string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = env
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

type sourceRecord struct {
	path string
	info fs.FileInfo
	data []byte
}

type sourceDirectoryRecord struct {
	path    string
	info    fs.FileInfo
	entries []string
}

type stageGuard struct {
	path string
	info fs.FileInfo
}

func (s stageGuard) verify() error {
	info, err := os.Lstat(s.path)
	if err != nil {
		return fmt.Errorf("inspect private bat cache stage: %w", err)
	}
	if !info.IsDir() || !os.SameFile(s.info, info) {
		return fmt.Errorf("private bat cache stage changed identity")
	}
	return nil
}

// Activate builds in private staging, publishes only bat's three generated
// cache slots through an os.Root, and proves the real cache loads Carbonfox.
func Activate(ctx context.Context, input Input) (receipt Receipt, resultErr error) {
	defer func() {
		if resultErr != nil {
			receipt = Receipt{}
		}
	}()
	ops := input.testOps
	if ops == nil {
		ops = defaultOperationHooks()
	}
	if strings.TrimSpace(input.Home) == "" {
		return Receipt{}, fmt.Errorf("bat activation home is required")
	}
	home, err := filepath.Abs(input.Home)
	if err != nil || !filepath.IsAbs(home) {
		return Receipt{}, fmt.Errorf("resolve bat activation home: %w", err)
	}
	home = filepath.Clean(home)
	root, err := os.OpenRoot(home)
	if err != nil {
		return Receipt{}, fmt.Errorf("open bat activation home: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, root.Close()) }()
	homeDir, err := os.Open(home)
	if err != nil {
		return Receipt{}, fmt.Errorf("open bat activation home descriptor: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, homeDir.Close()) }()
	rootInfo, rootErr := root.Stat(".")
	homeInfo, homeErr := homeDir.Stat()
	if err := errors.Join(rootErr, homeErr); err != nil {
		return Receipt{}, fmt.Errorf("inspect bat activation home: %w", err)
	}
	if !os.SameFile(rootInfo, homeInfo) {
		return Receipt{}, fmt.Errorf("bat activation home changed while opening")
	}

	if err := rejectWriteSymlinkAncestors(root, ".cache/dots"); err != nil {
		return Receipt{}, err
	}
	if err := mkdirAllRoot(root, ".cache/dots", 0o700); err != nil {
		return Receipt{}, fmt.Errorf("create bat activation state directory: %w", err)
	}
	dotsDir, err := openPinnedDirectory(root, ".cache/dots")
	if err != nil {
		return Receipt{}, fmt.Errorf("open confined bat activation state directory: %w", err)
	}
	lockFD, err := unix.Openat(int(dotsDir.Fd()), "bat-cache.lock", unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	dotsCloseErr := dotsDir.Close()
	if err != nil {
		return Receipt{}, errors.Join(fmt.Errorf("open bat cache operation lock: %w", err), dotsCloseErr)
	}
	lock := os.NewFile(uintptr(lockFD), "bat-cache.lock")
	lockInfo, statErr := lock.Stat()
	if err := errors.Join(statErr, dotsCloseErr); err != nil {
		return Receipt{}, errors.Join(fmt.Errorf("validate bat cache operation lock: %w", err), lock.Close())
	}
	if !lockInfo.Mode().IsRegular() {
		return Receipt{}, errors.Join(fmt.Errorf("bat cache operation lock is not a regular file"), lock.Close())
	}
	locked := false
	defer func() {
		if locked {
			if err := ops.flock(int(lock.Fd()), unix.LOCK_UN); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("release bat cache operation lock: %w", err))
			}
		}
		if err := lock.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close bat cache operation lock: %w", err))
		}
	}()
	if err := ops.flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return Receipt{}, fmt.Errorf("acquire bat cache operation lock: %w", err)
	}
	locked = true
	stage, err := ops.mkdirTemp("", "dots-bat-cache-")
	if err != nil {
		return Receipt{}, fmt.Errorf("create private bat cache stage: %w", err)
	}
	stage, err = filepath.Abs(stage)
	if err != nil {
		return Receipt{}, fmt.Errorf("resolve private bat cache stage; empty stage retained at %s: %w", stage, err)
	}
	stage = filepath.Clean(stage)
	stageParent, err := os.Open(filepath.Dir(stage))
	if err != nil {
		return Receipt{}, fmt.Errorf("open private bat cache stage parent; empty stage retained at %s: %w", stage, err)
	}
	stageBase := filepath.Base(stage)
	stageFD, err := unix.Openat(int(stageParent.Fd()), stageBase, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return Receipt{}, errors.Join(fmt.Errorf("open private bat cache stage; stage retained at %s: %w", stage, err), stageParent.Close())
	}
	stageDir := os.NewFile(uintptr(stageFD), stage)
	if err := stageParent.Close(); err != nil {
		cleanupErr := ops.cleanupStage(stageDir)
		emptyErr := verifyDirectoryEmpty(stageDir)
		return Receipt{}, errors.Join(fmt.Errorf("close private bat cache stage parent; outer stage retained at %s: %w", stage, err), cleanupErr, emptyErr, stageDir.Close())
	}
	stageInfo, err := stageDir.Stat()
	if err != nil {
		cleanupErr := ops.cleanupStage(stageDir)
		emptyErr := verifyDirectoryEmpty(stageDir)
		return Receipt{}, errors.Join(fmt.Errorf("capture private bat cache stage identity; outer stage retained at %s: %w", stage, err), cleanupErr, emptyErr, stageDir.Close())
	}
	if stageInfo.Mode().Perm() != 0o700 {
		cleanupErr := ops.cleanupStage(stageDir)
		emptyErr := verifyDirectoryEmpty(stageDir)
		return Receipt{}, errors.Join(fmt.Errorf("private bat cache stage mode is %o, want 700; outer stage retained at %s", stageInfo.Mode().Perm(), stage), cleanupErr, emptyErr, stageDir.Close())
	}
	guard := stageGuard{path: stage, info: stageInfo}
	defer func() {
		hookErr := ops.beforeStageCleanup(stage)
		cleanupErr := ops.cleanupStage(stageDir)
		emptyErr := verifyDirectoryEmpty(stageDir)
		closeErr := stageDir.Close()
		resultErr = errors.Join(resultErr, hookErr, cleanupErr, emptyErr, closeErr)
	}()

	env := sanitizedEnvironment(input.BaseEnv, home)
	executable, err := resolveExecutable("bat", env)
	if err != nil {
		return Receipt{}, err
	}
	runner := input.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	if err := validateNativePaths(ctx, runner, executable, env, home, guard); err != nil {
		return Receipt{}, err
	}
	if err := verifyManagedInputs(root, input); err != nil {
		return Receipt{}, err
	}

	sourceStage := filepath.Join(stage, "source")
	outputStage := filepath.Join(stage, "output")
	if err := os.MkdirAll(sourceStage, 0o700); err != nil {
		return Receipt{}, fmt.Errorf("create bat source stage: %w", err)
	}
	if err := os.MkdirAll(outputStage, 0o700); err != nil {
		return Receipt{}, fmt.Errorf("create bat output stage: %w", err)
	}

	records, directories, err := snapshotCustomAssets(home, sourceStage)
	if err != nil {
		return Receipt{}, err
	}
	if err := writeStageFile(filepath.Join(sourceStage, "config"), input.Config); err != nil {
		return Receipt{}, err
	}
	if err := writeStageFile(filepath.Join(sourceStage, "themes", "Carbonfox.tmTheme"), input.Theme); err != nil {
		return Receipt{}, err
	}

	buildEnv := withEnv(env,
		"BAT_CONFIG_DIR="+sourceStage,
		"BAT_CONFIG_PATH="+filepath.Join(sourceStage, "config"),
		"BAT_CACHE_PATH="+outputStage,
	)
	if _, stderr, err := runNative(ctx, runner, guard, executable, []string{"cache", "--build"}, buildEnv, nil); err != nil {
		return Receipt{}, fmt.Errorf("build bat custom cache: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	if err := revalidateSources(records); err != nil {
		return Receipt{}, err
	}
	if err := revalidateDirectories(directories); err != nil {
		return Receipt{}, err
	}
	if err := verifyManagedInputs(root, input); err != nil {
		return Receipt{}, err
	}
	generated, hashes, err := inventoryGenerated(outputStage, true)
	if err != nil {
		return Receipt{}, err
	}
	if err := verifyNativeTheme(ctx, runner, executable, buildEnv, guard); err != nil {
		return Receipt{}, fmt.Errorf("validate staged bat Carbonfox cache: %w", err)
	}

	if err := publishGenerated(root, generated, ops.publish); err != nil {
		return Receipt{}, err
	}
	actual, err := inventoryPublished(root)
	if err != nil {
		return Receipt{}, fmt.Errorf("inventory published bat cache: %w", err)
	}
	for _, name := range generatedFiles {
		if actual[name] != hashes[name] {
			return Receipt{}, fmt.Errorf("published bat cache %s does not match generated output", name)
		}
	}
	if err := verifyNativeTheme(ctx, runner, executable, env, guard); err != nil {
		return Receipt{}, fmt.Errorf("validate selected-home bat Carbonfox cache: %w", err)
	}
	return Receipt{Executable: executable, CacheDir: filepath.Join(home, cacheRelative), Hashes: hashes}, nil
}

func sanitizedEnvironment(base []string, home string) []string {
	if base == nil {
		base = os.Environ()
	}
	result := make([]string, 0, len(base)+1)
	for _, item := range base {
		name, _, ok := strings.Cut(item, "=")
		if !ok || name == "HOME" || strings.HasPrefix(name, "BAT_") || strings.HasPrefix(name, "XDG_") {
			continue
		}
		result = append(result, item)
	}
	return append(result, "HOME="+home)
}

func withEnv(base []string, values ...string) []string {
	result := append([]string(nil), base...)
	for _, value := range values {
		name, _, _ := strings.Cut(value, "=")
		filtered := result[:0]
		for _, item := range result {
			if !strings.HasPrefix(item, name+"=") {
				filtered = append(filtered, item)
			}
		}
		result = append(filtered, value)
	}
	return result
}

func resolveExecutable(name string, env []string) (string, error) {
	path := ""
	for _, item := range env {
		if strings.HasPrefix(item, "PATH=") {
			path = strings.TrimPrefix(item, "PATH=")
			break
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("resolve bat executable in sanitized PATH")
}

func validateNativePaths(ctx context.Context, runner CommandRunner, executable string, env []string, home string, guard stageGuard) error {
	wants := []struct {
		args []string
		path string
	}{
		{[]string{"--config-dir"}, filepath.Join(home, ".config", "bat")},
		{[]string{"--config-file"}, filepath.Join(home, configRelative)},
		{[]string{"--cache-dir"}, filepath.Join(home, cacheRelative)},
	}
	for _, item := range wants {
		stdout, stderr, err := runNative(ctx, runner, guard, executable, item.args, env, nil)
		if err != nil {
			return fmt.Errorf("query bat %s: %w: %s", item.args[0], err, strings.TrimSpace(string(stderr)))
		}
		got := strings.TrimSpace(string(stdout))
		if strings.ContainsAny(got, "\r\n\x00") || filepath.Clean(got) != item.path {
			return fmt.Errorf("bat %s reported %q, want selected-home path %q", item.args[0], got, item.path)
		}
	}
	return nil
}

func snapshotCustomAssets(home, stage string) ([]sourceRecord, []sourceDirectoryRecord, error) {
	var records []sourceRecord
	var directories []sourceDirectoryRecord
	for _, name := range []string{"themes", "syntaxes"} {
		source := filepath.Join(home, ".config", "bat", name)
		if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, nil, fmt.Errorf("inspect bat custom %s: %w", name, err)
		}
		if err := snapshotTree(source, filepath.Join(stage, name), &records, &directories, map[string]bool{}); err != nil {
			return nil, nil, fmt.Errorf("snapshot bat custom %s: %w", name, err)
		}
	}
	return records, directories, nil
}

func snapshotTree(source, target string, records *[]sourceRecord, directories *[]sourceDirectoryRecord, ancestors map[string]bool) error {
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		resolved = filepath.Clean(resolved)
		if ancestors[resolved] {
			return fmt.Errorf("cyclic directory link at %s", source)
		}
		next := make(map[string]bool, len(ancestors)+1)
		for key, value := range ancestors {
			next[key] = value
		}
		next[resolved] = true
		if err := os.MkdirAll(target, 0o700); err != nil {
			return err
		}
		dir, err := os.Open(source)
		if err != nil {
			return err
		}
		opened, statErr := dir.Stat()
		entries, readErr := dir.ReadDir(-1)
		closeErr := dir.Close()
		if err := errors.Join(statErr, readErr, closeErr); err != nil {
			return err
		}
		if !os.SameFile(info, opened) {
			return fmt.Errorf("custom bat directory changed identity: %s", source)
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
			if err := snapshotTree(filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name()), records, directories, next); err != nil {
				return err
			}
		}
		sort.Strings(names)
		*directories = append(*directories, sourceDirectoryRecord{path: source, info: opened, entries: names})
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("custom bat input %s is not a regular file", source)
	}
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	opened, err := file.Stat()
	if err != nil {
		return errors.Join(err, file.Close())
	}
	if !os.SameFile(info, opened) {
		return errors.Join(fmt.Errorf("custom bat input changed identity: %s", source), file.Close())
	}
	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return err
	}
	if err := writeStageFile(target, data); err != nil {
		return err
	}
	*records = append(*records, sourceRecord{path: source, info: opened, data: data})
	return nil
}

func revalidateSources(records []sourceRecord) error {
	for _, record := range records {
		file, err := os.Open(record.path)
		if err != nil {
			return fmt.Errorf("reopen custom bat input %s: %w", record.path, err)
		}
		info, statErr := file.Stat()
		data, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if err := errors.Join(statErr, readErr, closeErr); err != nil {
			return fmt.Errorf("revalidate custom bat input %s: %w", record.path, err)
		}
		if !os.SameFile(record.info, info) || !bytes.Equal(record.data, data) {
			return fmt.Errorf("custom bat input changed during cache build: %s", record.path)
		}
	}
	return nil
}

func revalidateDirectories(records []sourceDirectoryRecord) error {
	for _, record := range records {
		dir, err := os.Open(record.path)
		if err != nil {
			return fmt.Errorf("reopen custom bat directory %s: %w", record.path, err)
		}
		info, statErr := dir.Stat()
		entries, readErr := dir.ReadDir(-1)
		closeErr := dir.Close()
		if err := errors.Join(statErr, readErr, closeErr); err != nil {
			return fmt.Errorf("revalidate custom bat directory %s: %w", record.path, err)
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		sort.Strings(names)
		if !os.SameFile(record.info, info) || !equalStrings(record.entries, names) {
			return fmt.Errorf("custom bat directory changed during cache build: %s", record.path)
		}
	}
	return nil
}

func writeStageFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write bat stage file %s: %w", path, err)
	}
	return nil
}

func inventoryGenerated(dir string, exact bool) (map[string][]byte, map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if exact && !equalStrings(names, generatedFiles) {
		return nil, nil, fmt.Errorf("bat cache output inventory = %v, want %v", names, generatedFiles)
	}
	generated := make(map[string][]byte, len(generatedFiles))
	hashes := make(map[string]string, len(generatedFiles))
	for _, name := range generatedFiles {
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return nil, nil, fmt.Errorf("bat cache output %s is not a regular file", name)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		if len(data) == 0 {
			return nil, nil, fmt.Errorf("bat cache output %s is empty", name)
		}
		generated[name] = data
		hashes[name] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	return generated, hashes, nil
}

func inventoryPublished(root *os.Root) (map[string]string, error) {
	hashes := make(map[string]string, len(generatedFiles))
	for _, name := range generatedFiles {
		path := filepath.Join(cacheRelative, name)
		info, err := root.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("published bat cache %s is not a regular file", name)
		}
		data, err := readRootRegular(root, path)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("published bat cache %s is empty", name)
		}
		hashes[name] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	return hashes, nil
}

func publishGenerated(root *os.Root, generated map[string][]byte, ops publishOperations) (resultErr error) {
	if err := rejectWriteSymlinkAncestors(root, cacheRelative); err != nil {
		return err
	}
	if err := mkdirAllRoot(root, cacheRelative, 0o700); err != nil {
		return fmt.Errorf("create bat cache directory: %w", err)
	}
	cacheDir, err := openPinnedDirectory(root, cacheRelative)
	if err != nil {
		return fmt.Errorf("open confined bat cache directory: %w", err)
	}
	defer func() {
		if err := cacheDir.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close confined bat cache directory: %w", err))
		}
	}()
	for _, name := range generatedFiles {
		data, ok := generated[name]
		if !ok || len(data) == 0 {
			return fmt.Errorf("missing captured bat cache output %s", name)
		}
		token, err := randomToken()
		if err != nil {
			return fmt.Errorf("create bat cache temporary name: %w", err)
		}
		temp := ".dots-" + token
		file, err := ops.create(cacheDir, temp)
		if err != nil {
			return fmt.Errorf("create bat cache temporary file: %w", err)
		}
		_, writeErr := io.Copy(file, bytes.NewReader(data))
		syncErr := file.Sync()
		closeErr := file.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			cleanupErr := ops.remove(cacheDir, temp)
			return fmt.Errorf("write bat cache %s: %w", name, errors.Join(err, cleanupErr))
		}
		if err := ops.rename(cacheDir, temp, name); err != nil {
			cleanupErr := ops.remove(cacheDir, temp)
			return fmt.Errorf("publish bat cache %s: %w", name, errors.Join(err, cleanupErr))
		}
		dir, err := ops.openDir(cacheDir)
		if err != nil {
			return err
		}
		syncDirErr := dir.Sync()
		closeDirErr := dir.Close()
		if err := errors.Join(syncDirErr, closeDirErr); err != nil {
			return fmt.Errorf("sync bat cache directory after %s: %w", name, err)
		}
	}
	return nil
}

func openPinnedDirectory(root *os.Root, relative string) (*os.File, error) {
	if err := rejectWriteSymlinkAncestors(root, relative); err != nil {
		return nil, err
	}
	dir, err := root.Open(relative)
	if err != nil {
		return nil, err
	}
	opened, statErr := dir.Stat()
	if statErr != nil {
		return nil, errors.Join(statErr, dir.Close())
	}
	if !opened.IsDir() {
		return nil, errors.Join(fmt.Errorf("%s is not a directory", relative), dir.Close())
	}
	if err := rejectWriteSymlinkAncestors(root, relative); err != nil {
		return nil, errors.Join(err, dir.Close())
	}
	observed, err := root.Lstat(relative)
	if err != nil {
		return nil, errors.Join(err, dir.Close())
	}
	if !observed.IsDir() || !os.SameFile(observed, opened) {
		return nil, errors.Join(fmt.Errorf("%s changed identity while opening", relative), dir.Close())
	}
	return dir, nil
}

func removeDirectoryContents(dir *os.File) error {
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return fmt.Errorf("read private bat cache stage directory: %w", err)
	}
	var errs []error
	for _, entry := range entries {
		name := entry.Name()
		childFD, openErr := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr == nil {
			child := os.NewFile(uintptr(childFD), name)
			childCleanupErr := removeDirectoryContents(child)
			childCloseErr := child.Close()
			removeErr := unix.Unlinkat(int(dir.Fd()), name, unix.AT_REMOVEDIR)
			if err := errors.Join(childCleanupErr, childCloseErr, removeErr); err != nil {
				errs = append(errs, fmt.Errorf("remove private bat cache stage directory %s: %w", name, err))
			}
			continue
		}
		if !errors.Is(openErr, unix.ENOTDIR) && !errors.Is(openErr, unix.ELOOP) {
			errs = append(errs, fmt.Errorf("inspect private bat cache stage entry %s: %w", name, openErr))
			continue
		}
		if err := unix.Unlinkat(int(dir.Fd()), name, 0); err != nil && !errors.Is(err, unix.ENOENT) {
			errs = append(errs, fmt.Errorf("remove private bat cache stage entry %s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

func verifyDirectoryEmpty(dir *os.File) error {
	fd, err := unix.Openat(int(dir.Fd()), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open private bat cache stage for empty verification: %w", err)
	}
	verification := os.NewFile(uintptr(fd), dir.Name())
	entries, readErr := verification.ReadDir(1)
	closeErr := verification.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return fmt.Errorf("verify private bat cache stage is empty: %w", errors.Join(readErr, closeErr))
	}
	if closeErr != nil {
		return fmt.Errorf("close private bat cache stage empty verification: %w", closeErr)
	}
	if len(entries) != 0 {
		return fmt.Errorf("private bat cache stage cleanup left %s", entries[0].Name())
	}
	return nil
}

func verifyManagedInputs(root *os.Root, input Input) error {
	wants := []struct {
		path string
		data []byte
	}{
		{configRelative, input.Config},
		{themeRelative, input.Theme},
	}
	for _, item := range wants {
		info, err := root.Lstat(item.path)
		if err != nil {
			return fmt.Errorf("inspect installed bat input %s: %w", item.path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("installed bat input %s is not a regular file", item.path)
		}
		data, err := readRootRegular(root, item.path)
		if err != nil {
			return fmt.Errorf("read installed bat input %s: %w", item.path, err)
		}
		if !bytes.Equal(data, item.data) {
			return fmt.Errorf("installed bat input %s differs from captured Source of Truth", item.path)
		}
	}
	return nil
}

func readRootRegular(root *os.Root, path string) ([]byte, error) {
	before, err := root.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	file, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	after, statErr := file.Stat()
	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if err := errors.Join(statErr, readErr, closeErr); err != nil {
		return nil, err
	}
	if !os.SameFile(before, after) {
		return nil, fmt.Errorf("%s changed identity while reading", path)
	}
	return data, nil
}

func rejectWriteSymlinkAncestors(root *os.Root, relative string) error {
	current := ""
	for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect bat write path %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("bat write path %s contains a symbolic link", current)
		}
	}
	return nil
}

func verifyNativeTheme(ctx context.Context, runner CommandRunner, executable string, env []string, guard stageGuard) error {
	stdout, stderr, err := runNative(ctx, runner, guard, executable, []string{"--list-themes"}, env, nil)
	if err != nil {
		return fmt.Errorf("list bat themes: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	found := false
	for _, line := range strings.Split(string(stdout), "\n") {
		if strings.TrimSpace(line) == "Carbonfox" {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("bat theme inventory does not contain Carbonfox")
	}
	stdout, stderr, err = runNative(ctx, runner, guard, executable, []string{"--color=always", "--paging=never", "--style=plain", "--language=Go", "-"}, env, []byte("package main\nfunc main() {}\n"))
	if err != nil {
		return fmt.Errorf("render bat Carbonfox sample: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	if len(stdout) == 0 || !bytes.Contains(stdout, []byte("\x1b[")) {
		return fmt.Errorf("bat Carbonfox render produced no ANSI-colored output")
	}
	return nil
}

func runNative(ctx context.Context, runner CommandRunner, guard stageGuard, executable string, args, env []string, stdin []byte) ([]byte, []byte, error) {
	if err := guard.verify(); err != nil {
		return nil, nil, err
	}
	stdout, stderr, runErr := runner.Run(ctx, executable, args, env, stdin, guard.path)
	return stdout, stderr, errors.Join(runErr, guard.verify())
}

func randomToken() (string, error) {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", value[:]), nil
}

func mkdirAllRoot(root *os.Root, relative string, mode fs.FileMode) error {
	current := ""
	for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("bat directory path %s is not a real directory", current)
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := root.Mkdir(current, mode); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
