package rpc

import (
	"time"

	"github.com/p4tin/goi/internal/journal"
)

// JournalPut records an unsaved buffer so it can be restored after a crash.
// baseModTime is the file's RFC 3339 modification time when the buffer was
// loaded ("" if unknown).
func (s *AppService) JournalPut(path, content, baseModTime string) error {
	sess, err := s.app.current()
	if err != nil {
		return err
	}
	if sess.journal == nil {
		return nil
	}
	base, _ := time.Parse(time.RFC3339Nano, baseModTime)
	return sess.journal.Put(journal.Entry{Path: path, Content: content, BaseModTime: base})
}

// JournalDrop forgets a buffer's journaled content (after save or discard).
func (s *AppService) JournalDrop(path string) error {
	sess, err := s.app.current()
	if err != nil {
		return err
	}
	if sess.journal == nil {
		return nil
	}
	return sess.journal.Drop(path)
}
