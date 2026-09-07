package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// jsonList is a list of records kept in one JSON file under the data directory
// ({"<field>": [...]}) — the shape shared by the artifact index and the
// notification outbox. It carries the load/save/update mechanics once, so the
// locking and atomic-write recipe cannot drift between the two.
type jsonList[T any] struct {
	file  string // file name under the data directory
	field string // the document's single top-level key
	// dropWhenEmpty removes the file instead of writing an empty list, so a
	// reader's "is there anything?" check stays a cheap ENOENT.
	dropWhenEmpty bool
}

// load reads the list. A missing file yields an empty list. The caller must
// hold s.mu.
func (l jsonList[T]) load(s *Store) ([]T, error) {
	data, err := os.ReadFile(s.path(l.file))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", l.file, err)
	}
	var doc map[string][]T
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", l.file, err)
	}
	return doc[l.field], nil
}

// save atomically writes the list. The caller must hold s.mu.
func (l jsonList[T]) save(s *Store, list []T) error {
	if len(list) == 0 && l.dropWhenEmpty {
		if err := os.Remove(s.path(l.file)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", l.file, err)
		}
		return nil
	}
	if list == nil {
		list = []T{}
	}
	data, err := json.MarshalIndent(map[string][]T{l.field: list}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", l.file, err)
	}
	return writeAtomic(s.path(l.file), data)
}

// update atomically applies fn to the list, serialized with other updates (and
// cross-process via the write lock) so the daemon and a CLI process never
// clobber each other's changes.
func (l jsonList[T]) update(s *Store, fn func(*[]T) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.withWriteLock(func() error {
		s.mu.Lock()
		list, err := l.load(s)
		s.mu.Unlock()
		if err != nil {
			return err
		}
		if err := fn(&list); err != nil {
			return err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		return l.save(s, list)
	})
}
