package fscache

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"slices"
	"testing"
	"time"
)

func TestConstructionOnValidPath(t *testing.T) {
	fpath, err := os.MkdirTemp("", "test.*")
	if err != nil {
		t.Fatalf("Could not create tmpdir to test fscache: %v", err)
	}
	defer os.Remove(fpath)
	fc, err := New(fpath)
	if err != nil {
		log.Fatalf("New() unexpectedly returned error upon valid construction into %v", fpath)
	}
	if fc == nil {
		log.Fatalf("New() unexpectedly returned no error but nil object upon valid construction into %v", fpath)
	}
	allfiles, err2 := os.ReadDir(fpath)
	if err2 != nil {
		log.Fatalf("failed to read fscache directory: permissions changed? err: %v", err2)
	}
	if len(allfiles) != 0 {
		log.Fatalf("fscache.New() unexpectedly left files behind: %v", allfiles)
	}
}

func TestConstructionOnInvalidPath(t *testing.T) {
	fpath := "/non-existing/path"
	fc, err := New(fpath)
	if err == nil || fc != nil {
		log.Fatalf("New() unexpectedly returned success operating on inexisting path")
	}
}

func TestGetInstanceOnValidPath(t *testing.T) {
	fpath, derr := os.MkdirTemp("", "test.*")
	if derr != nil {
		t.Fatalf("Could not create tmpdir to test fscache: %v", derr)
	}
	defer os.Remove(fpath)
	i, err := GetInstance(fpath)
	if err != nil {
		log.Fatalf("GetInstance() unexpectedly returned error upon valid construction into %v", fpath)
	}
	defer i.Clear()
	i2, err2 := GetInstance(fpath)
	if err2 != nil {
		log.Fatalf("Repeated GetInstance() call unexpectedly returned error upon valid construction into %v", fpath)
	}
	if i2 != i {
		log.Fatalf("GetInstance() returns different cache instance upon repeated call on same path")
	}
}

func TestGetInstanceOnInvalidPath(t *testing.T) {
	fpath := "/non-existing/path"
	fc, err := GetInstance(fpath)
	if err == nil || fc != nil {
		log.Fatalf("GetInstance() unexpectedly returned success operating on inexisting path")
	}
	fc2, err2 := GetInstance(fpath)
	if err2 == nil || fc2 != nil {
		log.Fatalf("Repeated call to GetInstance() unexpectedly returned success operating on inexisting path")
	}
}

func TestListValidEmpty(t *testing.T) {
	fpath, derr := os.MkdirTemp("", "test.*")
	if derr != nil {
		t.Fatalf("Could not create tmpdir to test fscache: %v", derr)
	}
	defer os.Remove(fpath)
	fc, err := GetInstance(fpath)
	if err != nil {
		t.Fatalf("GetInstance() failed to open path at %v: %v", fpath, err)
	}
	entries, _ := fc.List()
	if len(entries) != 0 {
		t.Errorf("List() returns non-0 entries on empty cache: %v", len(entries))
	}
}

func TestListValidSome(t *testing.T) {
	fpath, derr := os.MkdirTemp("", "test.*")
	if derr != nil {
		t.Fatalf("Could not create tmpdir to test fscache: %v", derr)
	}
	defer os.Remove(fpath)
	fc, err := GetInstance(fpath)
	if err != nil {
		t.Fatalf("GetInstance() failed to open path at %v: %v", fpath, err)
	}
	// add some items
	wanted_items := []string{"a", "a1", "a2", "b", "b_url"}
	for _, iname := range wanted_items {
		fc.Set(iname, []byte(iname+"_body"))
	}
	// list result
	entries, err := fc.List()
	if err != nil {
		t.Fatalf("List() fails with %v", err)
	}
	if len(entries) != len(wanted_items) {
		t.Errorf("List() returns %v entries on cache with %v", len(entries), len(wanted_items))
	}
	for _, iname := range wanted_items {
		if slices.Index(entries, iname) == -1 {
			t.Errorf("List() returns %v which misses expected item '%v'", entries, iname)
		}
	}
}

func TestSetValid(t *testing.T) {
	fpath, derr := os.MkdirTemp("", "test.*")
	if derr != nil {
		t.Fatalf("Could not create tmpdir to test fscache: %v", derr)
	}
	defer os.Remove(fpath)
	fc, _ := GetInstance(fpath)
	defer fc.Clear()
	err := fc.Set("foobar", nil)
	if err != nil {
		log.Fatalf("Set() unexpectedly returned error when storing key='foobar' val=nil : %v", err)
	}
	err = fc.Set("foobar", []byte("x"))
	if err != nil {
		log.Fatalf("Set() for valid key='foobar' val='x' unexpectedly returns error %v", err)
	}
}

func TestGet(t *testing.T) {
	fpath, derr := os.MkdirTemp("", "test.*")
	if derr != nil {
		t.Fatalf("Could not create tmpdir to test fscache: %v", derr)
	}
	defer os.Remove(fpath)
	fc, _ := GetInstance(fpath)
	defer fc.Clear()
	// non-set value
	data := fc.Get("foobar")
	if data != nil {
		log.Fatalf("Get() of previously-unset value returns non-nil = '%v'", data)
	}
	// value set to nil
	fc.Set("foobar", nil)
	data = fc.Get("foobar")
	if data != nil {
		log.Fatalf("Get() of previously-set 'nil' value returns non-nil = '%v'", data)
	}
	// value set to empty
	fc.Set("foobar", []byte(""))
	data = fc.Get("foobar")
	if data == nil || !bytes.Equal(data, []byte("")) {
		log.Fatalf("Get() of previously-set '' value returns '%v' != ''", data)
	}
	// value set to non-empty
	fc.Set("foobar", []byte("ciao"))
	data = fc.Get("foobar")
	if data == nil || !bytes.Equal(data, []byte("ciao")) {
		log.Fatalf("Get() of previously-set 'ciao' value returns '%v' != 'ciao'", data)
	}
}

func TestGetExpiry(t *testing.T) {
	fpath, derr := os.MkdirTemp("", "test.*")
	if derr != nil {
		t.Fatalf("Could not create tmpdir to test fscache: %v", derr)
	}
	defer os.Remove(fpath)
	fc, _ := GetInstance(fpath)
	defer fc.Clear()
	fc.Set("asdf", []byte("val"))
	if fc.GetExpiry("asdf", time.Duration(10)*time.Millisecond) == nil {
		log.Fatalf("Get() fails to return set value")
	}
	time.Sleep(time.Duration(10) * time.Millisecond)
	if fc.GetExpiry("asdf", time.Duration(10)*time.Millisecond) != nil {
		log.Fatalf("GetExpiry() returns value cached before the maxAge requested")
	}
}

func TestUnset(t *testing.T) {
	fpath, derr := os.MkdirTemp("", "test.*")
	if derr != nil {
		t.Fatalf("Could not create tmpdir to test fscache: %v", derr)
	}
	defer os.Remove(fpath)
	fc, _ := GetInstance(fpath)
	defer fc.Clear()
	// clear previously-unset value
	cleared := fc.Unset("foobar")
	if cleared {
		log.Fatalf("Unset() on previously-unset value claims value was cleared.")
	}
	// clear previously-set value
	fc.Set("foobar", []byte("asd"))
	cleared = fc.Unset("foobar")
	if !cleared {
		log.Fatalf("Unset() on previously-set value claims value was not cleared.")
	}
}

func TestClearExpiry(t *testing.T) {
	fpath, derr := os.MkdirTemp("", "test.*")
	if derr != nil {
		t.Fatalf("Could not create tmpdir to test fscache: %v", derr)
	}
	defer os.Remove(fpath)
	fc, _ := GetInstance(fpath)
	defer fc.Clear()
	// cache 1st batch of values at t0
	var nitems uint = 20
	for i := 0; i < int(nitems); i++ {
		fc.Set(fmt.Sprintf("%d", i), []byte("1st"))
	}
	l := fc.Len()
	if l != nitems {
		t.Fatalf("Len() returns %v after creating %v items", l, nitems)
	}
	// clearing items before expiry makes no change
	fc.ClearExpired(time.Duration(2) * time.Second)
	l = fc.Len()
	if l != nitems {
		t.Fatalf("ClearExpired(2s) cleared %v items before they were expired", nitems-l)
	}
	// create 2nd batch of values at t1 = t0 + 1s
	time.Sleep(time.Duration(1) * time.Second)
	for i := nitems; i < 2*nitems; i++ {
		fc.Set(fmt.Sprintf("%d", i), []byte("2nd"))
	}
	l = fc.Len()
	if l != 2*nitems {
		t.Fatalf("Len() returns %v after creating %v items", l, 2*nitems)
	}
	// clearing first batch of items only leaves 2nd batch
	fc.ClearExpired(time.Duration(1) * time.Second)
	l = fc.Len()
	if l != nitems {
		t.Fatalf("ClearExpired(1s) cleared %v items != expected %v expired", nitems-l, nitems)
	}
}

func TestClear(t *testing.T) {
	fpath, derr := os.MkdirTemp("", "test.*")
	if derr != nil {
		t.Fatalf("Could not create tmpdir to test fscache: %v", derr)
	}
	defer os.Remove(fpath)
	fc, _ := GetInstance(fpath)
	defer fc.Clear()
	// cache 1st batch of values at t0
	var nitems uint = 20
	for i := 0; i < int(nitems); i++ {
		fc.Set(fmt.Sprintf("%d", i), []byte("1st"))
	}
	if fc.Len() != nitems {
		t.Fatalf("Set() failed to create expected number of items")
	}
	fc.Clear()
	nItemsLeft := fc.Len()
	if nItemsLeft != 0 {
		t.Fatalf("Clear() failed to remove all items, left %v behind", nItemsLeft)
	}
}
