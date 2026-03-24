package email

import (
	"context"
	"fmt"
	"time"
)

// Service is a lightweight email dispatcher.
// It queues sends to avoid blocking request/worker paths.
type Service struct {
	Sender Sender
	From   Address

	queue chan Message
}

type Address struct {
	Email string
	Name  string
}

type Message struct {
	To      Address
	Subject string
	Text    string
	HTML    string
	Tags    map[string]string
}

type Sender interface {
	Send(ctx context.Context, from Address, msg Message) error
	Name() string
}

func NewService(sender Sender, from Address) *Service {
	s := &Service{
		Sender: sender,
		From:   from,
		queue:  make(chan Message, 200),
	}
	go s.worker()
	return s
}

func (s *Service) Enabled() bool {
	return s != nil && s.Sender != nil && s.From.Email != ""
}

// Enqueue attempts to queue an email for async sending.
// Returns false if the queue is full or service disabled.
func (s *Service) Enqueue(msg Message) bool {
	if !s.Enabled() {
		return false
	}
	select {
	case s.queue <- msg:
		return true
	default:
		return false
	}
}

func (s *Service) worker() {
	for msg := range s.queue {
		if !s.Enabled() {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = s.Sender.Send(ctx, s.From, msg)
		cancel()
	}
}

func (a Address) String() string {
	if a.Name == "" {
		return a.Email
	}
	return fmt.Sprintf("%s <%s>", a.Name, a.Email)
}
