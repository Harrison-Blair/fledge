package lifecycle

import (
	"context"
	"errors"
	"testing"
)

// TestSessionRecordLockRemainsHeldAcrossRecordRewrite proves the startup lock
// survives an atomic session.json rewrite. The first holder locks, rewriteRecord
// renames a fresh inode over session.json, and a second independent acquisition
// must remain blocked until its context deadline. When the lock tracks the record
// inode instead of a dedicated file, the rename orphans the flock and the second
// acquisition wrongly succeeds against the new inode.
func TestSessionRecordLockRemainsHeldAcrossRecordRewrite(t *testing.T) {
	root := t.TempDir()
	writeTestRecord(t, root)

	unlockFirst, err := lockSessionRecord(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unlockFirst() }()

	if err := rewriteRecord(root, record{
		Version:            recordVersion,
		SessionName:        testSessionName,
		MessagingSessionID: "rewritten-session-id",
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*lockRetryInterval)
	defer cancel()
	unlockSecond, err := lockSessionRecord(ctx, root)
	if err == nil {
		_ = unlockSecond()
		t.Fatal("second independent startup lock acquired after session record rename")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second lock error = %v, want the context deadline", err)
	}
}
