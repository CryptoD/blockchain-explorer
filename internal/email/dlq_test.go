package email

import (
	"context"
	"sync"
	"testing"
)

func TestMaskEmailAddr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"a@b.co", "a***@b.co"},
		{"alice@example.com", "a***@example.com"},
		{"  bob@x.y  ", "b***@x.y"},
	}
	for _, tc := range cases {
		got := maskEmailAddr(tc.in)
		if got != tc.want {
			t.Fatalf("maskEmailAddr(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

// blockSendSender blocks in Send until the test closes unblock (so the queue can fill deterministically).
type blockSendSender struct {
	unblock   chan struct{}
	entered   chan struct{} // closed when Send is entered (first message only)
	enterOnce sync.Once
}

func (*blockSendSender) Name() string { return "block_send" }

func (b *blockSendSender) Send(ctx context.Context, from Address, msg Message) error {
	b.enterOnce.Do(func() {
		if b.entered != nil {
			close(b.entered)
		}
	})
	if b.unblock != nil {
		<-b.unblock
	}
	return nil
}

var _ EmailSender = (*blockSendSender)(nil)

func TestService_DeadLetterOnFullQueue(t *testing.T) {
	t.Parallel()
	unblock := make(chan struct{})
	entered := make(chan struct{})
	s := newService(&blockSendSender{unblock: unblock, entered: entered}, Address{Email: "app@example.com"}, 3, 5)

	// Worker takes one message and blocks in Send; buffer can hold 3 more.
	if !s.Enqueue(Message{To: Address{Email: "u0@x.co"}, Subject: "in_flight"}) {
		t.Fatal("first enqueue")
	}
	<-entered

	for i := 0; i < 3; i++ {
		if !s.Enqueue(Message{To: Address{Email: "u@x.co"}, Subject: "buf"}) {
			t.Fatalf("buffer enqueue %d", i)
		}
	}
	if s.Enqueue(Message{To: Address{Email: "u@x.co"}, Subject: "dropped", Tags: map[string]string{"type": "alert_triggered"}}) {
		t.Fatal("expected enqueue to fail (queue full)")
	}

	snap := s.QueueAdminSnapshot()
	if len(snap.DeadLetter) != 1 {
		t.Fatalf("dead letter len: %d %#v", len(snap.DeadLetter), snap.DeadLetter)
	}
	d := snap.DeadLetter[0]
	if d.Reason != "queue_full" || d.Subject != "dropped" || d.TagType != "alert_triggered" {
		t.Fatalf("unexpected dlq row: %+v", d)
	}
	if snap.Capacity != 3 || snap.DeadLetterCapacity != 5 {
		t.Fatalf("caps: %+v", snap)
	}

	close(unblock)
}
