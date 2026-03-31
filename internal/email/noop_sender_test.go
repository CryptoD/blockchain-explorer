package email

import (
	"context"
	"testing"
)

func TestNoopEmailSender_Send(t *testing.T) {
	var s NoopEmailSender
	ctx := context.Background()
	err := s.Send(ctx, Address{Email: "from@x"}, Message{To: Address{Email: "to@x"}, Subject: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Name() != "noop" {
		t.Fatalf("name = %q", s.Name())
	}
}

func TestNewService_NoopSender_EnabledRequiresFrom(t *testing.T) {
	s := NewService(NoopEmailSender{}, Address{Email: "app@example.com"})
	if !s.Enabled() {
		t.Fatal("expected enabled with from address")
	}
	if !s.Enqueue(Message{To: Address{Email: "u@x"}, Subject: "s", Text: "t"}) {
		t.Fatal("enqueue failed")
	}
}
