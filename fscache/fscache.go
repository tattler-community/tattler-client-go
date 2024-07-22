package fscache

import (
	"fmt"
	"os"
	"path"
	"sync"
	"time"
)

type FSCache struct {
	path string
}

type InstanceMap struct {
	instance map[string]*FSCache
	mux      sync.Mutex
}

var instanceMap InstanceMap

func init() {
	instanceMap = InstanceMap{
		instance: make(map[string]*FSCache, 1),
	}
}

func GetInstance(path string) (*FSCache, error) {
	instanceMap.mux.Lock()
	defer instanceMap.mux.Unlock()
	inst, ok := instanceMap.instance[path]
	if ok {
		return inst, nil
	}

	// no pre-existing instance for path; instantiate one
	inst, err := New(path)
	if err != nil {
		return nil, err
	}

	instanceMap.instance[path] = inst
	return inst, nil
}

func New(path string) (*FSCache, error) {
	// validate that directory
	tmpf, err := os.CreateTemp(path, "dirvalidation.*")
	if err != nil {
		return nil, fmt.Errorf("failed to validate write perms into '%v': creating file failed with %v", path, err)
	}
	os.Remove(tmpf.Name())
	tmpf.Close()

	c := &FSCache{
		path: path,
	}
	return c, nil
}

// List item names in cache.
// Return the list of their names upon success, or a non-nil error upon failure.
func (fc *FSCache) List() ([]string, error) {
	entries, err := os.ReadDir(fc.path)
	if err != nil {
		return nil, fmt.Errorf("failed to scan path '%v': %v", fc.path, err)
	}
	cacheEntries := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			cacheEntries = append(cacheEntries, entry.Name())
		}
	}
	return cacheEntries, nil
}

func (fc *FSCache) Set(key string, value []byte) error {
	if fc == nil {
		return fmt.Errorf("uninitialized filesystem cache given")
	}
	if value == nil {
		return nil
	}
	f, err := os.CreateTemp(fc.path, key+".*")
	if err != nil {
		return fmt.Errorf("failed to create tempfile to cache '%v': %v", key, err)
	}
	defer f.Close()
	_, werr := f.Write(value)
	if werr != nil {
		os.Remove(f.Name())
		return werr
	}
	f.Truncate(int64(len(value)))
	newpath := path.Join(fc.path, key)
	os.Rename(f.Name(), newpath)
	return nil
}

// return a cached element only if it's younger than a given duration
func (fc *FSCache) GetExpiry(key string, maxAge time.Duration) []byte {
	p := path.Join(fc.path, key)
	fstat, err := os.Stat(p)
	if err != nil {
		return nil
	}
	if maxAge.Nanoseconds() > 0 && time.Since(fstat.ModTime()) > maxAge {
		// found, but too old
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	return data
}

func (fc *FSCache) Get(key string) []byte {
	return fc.GetExpiry(key, time.Duration(0))
}

func (fc *FSCache) Clear() error {
	direntries, err := os.ReadDir(fc.path)
	if err != nil {
		return fmt.Errorf("failed to Clear() cacheDir '%v': %v", fc.path, err)
	}
	var nerr error = nil
	for _, dirent := range direntries {
		nerr = os.RemoveAll(path.Join(fc.path, dirent.Name()))
	}
	return nerr
}

func (fc *FSCache) Unset(key string) bool {
	p := path.Join(fc.path, key)
	_, err := os.Stat(p)
	if err != nil {
		return false
	}
	os.Remove(p)
	return true
}

// clear all items older than a given age
func (fc *FSCache) ClearExpired(age time.Duration) error {
	direntries, err := os.ReadDir(fc.path)
	if err != nil {
		return fmt.Errorf("failed to ClearExpiry(%v) cacheDir '%v': %v", age, fc.path, err)
	}
	for _, dirent := range direntries {
		if !dirent.IsDir() {
			statInfo, statErr := dirent.Info()
			if statErr == nil && time.Since(statInfo.ModTime()) > age {
				expFn := path.Join(fc.path, dirent.Name())
				remErr := os.Remove(expFn)
				if remErr != nil {
					return fmt.Errorf("failed to clear expired '%v': %v", expFn, remErr)
				}
			}
		}
	}
	return nil
}

func (fc *FSCache) Len() uint {
	direntries, err := os.ReadDir(fc.path)
	if err != nil {
		return 0
	}
	var n uint = 0
	for _, dirent := range direntries {
		if !dirent.IsDir() {
			n++
		}
	}
	return n
}
