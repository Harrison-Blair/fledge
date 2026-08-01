package agentprofile

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/pelletier/go-toml"
)

const (
	profileDirName = "profiles"
	profileSuffix  = ".toml"
	maxProfileSize = 1 << 20
)

// Store manages profiles below <project>/.fledge/profiles. Its open project
// root descriptor prevents later path replacement from changing that boundary.
type Store struct {
	projectRoot string
	root        *os.Root
}

// New constructs a store without creating any project metadata directories.
func New(projectRoot string) (*Store, error) {
	if projectRoot == "" {
		return nil, invalid("open", "", projectRoot, fieldError("project_root", "is required"))
	}
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, invalid("open", "", projectRoot, fmt.Errorf("resolve project root: %w", err))
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, invalid("open", "", abs, fmt.Errorf("resolve project root: %w", err))
	}
	before, err := os.Lstat(real)
	if err != nil {
		return nil, invalid("open", "", real, fmt.Errorf("inspect project root: %w", err))
	}
	if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return nil, invalid("open", "", real, errors.New("project root is not a real directory"))
	}
	root, err := os.OpenRoot(real)
	if err != nil {
		return nil, invalid("open", "", real, fmt.Errorf("open project root: %w", err))
	}
	opened, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, invalid("open", "", real, fmt.Errorf("verify project root: %w", err))
	}
	after, statErr := opened.Stat()
	closeErr := opened.Close()
	if statErr != nil || closeErr != nil {
		root.Close()
		return nil, invalid("open", "", real, fmt.Errorf("verify project root: %w", errors.Join(statErr, closeErr)))
	}
	if !after.IsDir() || !os.SameFile(before, after) {
		root.Close()
		return nil, invalid("open", "", real, errors.New("project root changed while opening"))
	}
	return &Store{projectRoot: real, root: root}, nil
}

// NewStore is an explicit alias for New.
func NewStore(projectRoot string) (*Store, error) { return New(projectRoot) }

// ProjectRoot returns the canonical, immutable project root used by the store.
func (s *Store) ProjectRoot() string { return s.projectRoot }

// Close releases the project directory descriptor held by the store.
func (s *Store) Close() error { return s.root.Close() }

// Create atomically persists p and never replaces an existing profile.
func (s *Store) Create(p Profile) (Profile, error) {
	p, err := prepareForWrite(p)
	if err != nil {
		return Profile{}, invalid("create", p.Name, "", err)
	}
	data, err := encode(p)
	if err != nil {
		return Profile{}, invalid("create", p.Name, s.profilePath(p.Name), err)
	}
	profiles, exists, err := s.openProfiles(true)
	if err != nil {
		return Profile{}, err
	}
	if !exists {
		return Profile{}, invalid("create", p.Name, s.profilesPath(), errors.New("profile directory was not created"))
	}
	defer profiles.Close()
	name := p.Name + profileSuffix
	err = s.withLock(profiles, func() error {
		_, statErr := profiles.Lstat(name)
		switch {
		case statErr == nil:
			existing, err := openSafeProfile(profiles, name, "create", p.Name, s.profilePath(p.Name))
			if err != nil {
				return err
			}
			_ = existing.Close()
			return alreadyExists("create", p.Name, s.profilePath(p.Name), nil)
		case !errors.Is(statErr, fs.ErrNotExist):
			return invalid("create", p.Name, s.profilePath(p.Name), fmt.Errorf("inspect profile: %w", statErr))
		}
		if err := writeReplaceAtomic(profiles, name, data, 0o600); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return alreadyExists("create", p.Name, s.profilePath(p.Name), err)
			}
			return invalid("create", p.Name, s.profilePath(p.Name), fmt.Errorf("persist profile: %w", err))
		}
		return nil
	})
	if err != nil {
		return Profile{}, err
	}
	return p, nil
}

// Update atomically replaces an existing regular profile.
func (s *Store) Update(p Profile) (Profile, error) {
	p, err := prepareForWrite(p)
	if err != nil {
		return Profile{}, invalid("update", p.Name, "", err)
	}
	data, err := encode(p)
	if err != nil {
		return Profile{}, invalid("update", p.Name, s.profilePath(p.Name), err)
	}
	profiles, exists, err := s.openProfiles(false)
	if err != nil {
		return Profile{}, err
	}
	if !exists {
		return Profile{}, notFound("update", p.Name, s.profilePath(p.Name), nil)
	}
	defer profiles.Close()
	name := p.Name + profileSuffix
	err = s.withLock(profiles, func() error {
		existing, err := openSafeProfile(profiles, name, "update", p.Name, s.profilePath(p.Name))
		if err != nil {
			return err
		}
		defer existing.Close()
		if err := writeReplaceAtomic(profiles, name, data, 0o600); err != nil {
			return invalid("update", p.Name, s.profilePath(p.Name), fmt.Errorf("persist profile: %w", err))
		}
		return nil
	})
	if err != nil {
		return Profile{}, err
	}
	return p, nil
}

// Load reads and strictly validates name.
func (s *Store) Load(name string) (Profile, error) {
	if err := ValidateName(name); err != nil {
		return Profile{}, invalid("load", name, "", err)
	}
	profiles, exists, err := s.openProfiles(false)
	if err != nil {
		return Profile{}, err
	}
	if !exists {
		return Profile{}, notFound("load", name, s.profilePath(name), nil)
	}
	defer profiles.Close()
	return loadProfile(profiles, name+profileSuffix, name, "load", s.profilePath(name))
}

// Show is an alias for Load for command-oriented consumers.
func (s *Store) Show(name string) (Profile, error) { return s.Load(name) }

// List discovers all TOML profiles and returns them sorted by logical name.
// Invalid TOML entries are reported rather than skipped.
func (s *Store) List() ([]Profile, error) {
	profilesRoot, exists, err := s.openProfiles(false)
	if err != nil {
		return nil, err
	}
	if !exists {
		return []Profile{}, nil
	}
	defer profilesRoot.Close()
	dir, err := profilesRoot.Open(".")
	if err != nil {
		return nil, invalid("list", "", s.profilesPath(), fmt.Errorf("open profile directory: %w", err))
	}
	entries, readErr := dir.ReadDir(-1)
	closeErr := dir.Close()
	if readErr != nil || closeErr != nil {
		return nil, invalid("list", "", s.profilesPath(), fmt.Errorf("read profile directory: %w", errors.Join(readErr, closeErr)))
	}
	profiles := make([]Profile, 0, len(entries))
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), profileSuffix) {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), profileSuffix)
		path := s.profilePath(name)
		if err := ValidateName(name); err != nil {
			return nil, invalid("list", name, path, err)
		}
		profile, err := loadProfile(profilesRoot, entry.Name(), name, "list", path)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, nil
}

// Delete atomically removes an existing regular profile.
func (s *Store) Delete(name string) error {
	if err := ValidateName(name); err != nil {
		return invalid("delete", name, "", err)
	}
	profiles, exists, err := s.openProfiles(false)
	if err != nil {
		return err
	}
	if !exists {
		return notFound("delete", name, s.profilePath(name), nil)
	}
	defer profiles.Close()
	filename := name + profileSuffix
	return s.withLock(profiles, func() error {
		existing, err := openSafeProfile(profiles, filename, "delete", name, s.profilePath(name))
		if err != nil {
			return err
		}
		defer existing.Close()
		if err := profiles.Remove(filename); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return notFound("delete", name, s.profilePath(name), err)
			}
			return invalid("delete", name, s.profilePath(name), fmt.Errorf("remove profile: %w", err))
		}
		syncRoot(profiles)
		return nil
	})
}

func (s *Store) profilesPath() string {
	return filepath.Join(s.projectRoot, ".fledge", profileDirName)
}

func (s *Store) profilePath(name string) string {
	return filepath.Join(s.profilesPath(), name+profileSuffix)
}

func (s *Store) openProfiles(create bool) (*os.Root, bool, error) {
	fledge, exists, err := openVerifiedDir(s.root, ".fledge", create, true)
	if err != nil {
		return nil, false, invalid("access", "", filepath.Join(s.projectRoot, ".fledge"), err)
	}
	if !exists {
		return nil, false, nil
	}
	defer fledge.Close()
	profiles, exists, err := openVerifiedDir(fledge, profileDirName, create, true)
	if err != nil {
		return nil, false, invalid("access", "", s.profilesPath(), err)
	}
	return profiles, exists, nil
}

// openVerifiedDir binds operations to the exact non-symlink directory inode
// observed by Lstat. A concurrent rename or symlink substitution either fails
// verification or leaves the returned descriptor attached to the original.
func openVerifiedDir(parent *os.Root, name string, create, secure bool) (*os.Root, bool, error) {
	before, err := parent.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) && create {
		if mkdirErr := parent.Mkdir(name, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, fs.ErrExist) {
			return nil, false, fmt.Errorf("create directory: %w", mkdirErr)
		}
		before, err = parent.Lstat(name)
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
		return nil, false, errors.New("path is not a real directory")
	}
	root, err := parent.OpenRoot(name)
	if err != nil {
		return nil, false, fmt.Errorf("open directory: %w", err)
	}
	dir, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, false, fmt.Errorf("verify directory: %w", err)
	}
	after, err := dir.Stat()
	if err != nil {
		dir.Close()
		root.Close()
		return nil, false, fmt.Errorf("verify directory: %w", err)
	}
	if !after.IsDir() || !os.SameFile(before, after) {
		dir.Close()
		root.Close()
		return nil, false, errors.New("directory changed while opening")
	}
	if secure {
		if err := requireCurrentOwner(after); err != nil {
			dir.Close()
			root.Close()
			return nil, false, err
		}
		if after.Mode().Perm() != 0o700 {
			if err := dir.Chmod(0o700); err != nil {
				dir.Close()
				root.Close()
				return nil, false, fmt.Errorf("secure directory permissions: %w", err)
			}
			secured, err := dir.Stat()
			if err != nil || secured.Mode().Perm() != 0o700 {
				dir.Close()
				root.Close()
				if err == nil {
					err = fmt.Errorf("mode is %o after chmod", secured.Mode().Perm())
				}
				return nil, false, fmt.Errorf("verify directory permissions: %w", err)
			}
		}
	}
	if err := dir.Close(); err != nil {
		root.Close()
		return nil, false, fmt.Errorf("close directory verifier: %w", err)
	}
	return root, true, nil
}

func openSafeProfile(root *os.Root, filename, op, name, path string) (*os.File, error) {
	before, err := root.Lstat(filename)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, notFound(op, name, path, err)
	}
	if err != nil {
		return nil, invalid(op, name, path, fmt.Errorf("inspect profile: %w", err))
	}
	if err := validateProfileInode(before); err != nil {
		return nil, invalid(op, name, path, err)
	}
	file, err := root.OpenFile(filename, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, notFound(op, name, path, err)
	}
	if err != nil {
		return nil, invalid(op, name, path, fmt.Errorf("open profile: %w", err))
	}
	after, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, invalid(op, name, path, fmt.Errorf("inspect open profile: %w", err))
	}
	if err := validateProfileInode(after); err != nil {
		file.Close()
		return nil, invalid(op, name, path, err)
	}
	if !os.SameFile(before, after) {
		file.Close()
		return nil, invalid(op, name, path, errors.New("profile changed while opening"))
	}
	return file, nil
}

func validateProfileInode(info fs.FileInfo) error {
	if !info.Mode().IsRegular() {
		return errors.New("profile path is not a regular file")
	}
	if err := requireCurrentOwner(info); err != nil {
		return fmt.Errorf("unsafe profile ownership: %w", err)
	}
	if err := requireSingleLink(info); err != nil {
		return fmt.Errorf("unsafe profile link count: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("profile permissions %o are too permissive", info.Mode().Perm())
	}
	return nil
}

func loadProfile(root *os.Root, filename, name, op, path string) (Profile, error) {
	file, err := openSafeProfile(root, filename, op, name, path)
	if err != nil {
		return Profile{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxProfileSize+1))
	if err != nil {
		return Profile{}, invalid(op, name, path, fmt.Errorf("read profile: %w", err))
	}
	if len(data) > maxProfileSize {
		return Profile{}, invalid(op, name, path, fmt.Errorf("profile exceeds %d bytes", maxProfileSize))
	}
	var p Profile
	if err := toml.NewDecoder(bytes.NewReader(data)).Strict(true).Decode(&p); err != nil {
		return Profile{}, invalid(op, name, path, fmt.Errorf("decode TOML: %w", err))
	}
	p.Name = name
	if p.NativeArgs == nil {
		p.NativeArgs = []string{}
	}
	if err := Validate(p); err != nil {
		return Profile{}, invalid(op, name, path, err)
	}
	return p, nil
}

func encode(p Profile) ([]byte, error) {
	data, err := encodeTOML(p)
	if err != nil {
		return nil, err
	}
	if len(data) > maxProfileSize {
		return nil, fmt.Errorf("encoded profile is %d bytes; maximum is %d", len(data), maxProfileSize)
	}
	return data, nil
}

func encodeTOML(p Profile) ([]byte, error) {
	var data bytes.Buffer
	if err := toml.NewEncoder(&data).Order(toml.OrderPreserve).Encode(p); err != nil {
		return nil, fmt.Errorf("encode TOML: %w", err)
	}
	return data.Bytes(), nil
}

func (s *Store) withLock(root *os.Root, fn func() error) error {
	const name = ".profiles.lock"
	path := filepath.Join(s.profilesPath(), name)
	lock, err := root.OpenFile(name, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return invalid("lock", "", path, err)
	}
	defer lock.Close()
	info, err := lock.Stat()
	if err != nil {
		return invalid("lock", "", path, err)
	}
	if !info.Mode().IsRegular() {
		return invalid("lock", "", path, errors.New("lock path is not a regular file"))
	}
	if err := requireCurrentOwner(info); err != nil {
		return invalid("lock", "", path, err)
	}
	if err := requireSingleLink(info); err != nil {
		return invalid("lock", "", path, err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return invalid("lock", "", path, fmt.Errorf("lock permissions %o are too permissive", info.Mode().Perm()))
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return invalid("lock", "", path, fmt.Errorf("acquire lock: %w", err))
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck
	return fn()
}

func writeReplaceAtomic(root *os.Root, path string, data []byte, perm fs.FileMode) error {
	tmp, tmpName, err := createTemp(root, path, perm)
	if err != nil {
		return err
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = root.Remove(tmpName)
		}
	}()
	if err := writeAndClose(tmp, data, perm); err != nil {
		return err
	}
	if err := root.Rename(tmpName, path); err != nil {
		return err
	}
	removeTemp = false
	syncRoot(root)
	return nil
}

func createTemp(root *os.Root, target string, perm fs.FileMode) (*os.File, string, error) {
	for range 100 {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", fmt.Errorf("generate temporary filename: %w", err)
		}
		name := "." + target + "." + hex.EncodeToString(random[:]) + ".tmp"
		file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR|syscall.O_NOFOLLOW, perm)
		if err == nil {
			return file, name, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, "", fmt.Errorf("create temporary file: %w", err)
		}
	}
	return nil, "", errors.New("create temporary file: name attempts exhausted")
}

func writeAndClose(file *os.File, data []byte, perm fs.FileMode) error {
	if err := file.Chmod(perm); err != nil {
		file.Close()
		return fmt.Errorf("set temporary file permissions: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	return nil
}

func syncRoot(root *os.Root) {
	dir, err := root.Open(".")
	if err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
}
