package email

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/CryptoD/blockchain-explorer/internal/logging"
	"github.com/CryptoD/blockchain-explorer/internal/metrics"
	log "github.com/sirupsen/logrus"
)

const (
	defaultQueueCap = 200
	defaultDLQCap   = 100
)

// Service is a lightweight email dispatcher.
// It queues sends to avoid blocking request/worker paths.
// When the buffer is full, messages are recorded in an in-process dead-letter list
// (ring buffer) for admin visibility and metrics; they are not sent automatically later.
type Service struct {
	EmailSender EmailSender
	From        Address

	queue    chan Message
	queueCap int
	dlqCap   int

	deadMu sync.Mutex
	dead   []deadLetterInternal
}

type deadLetterInternal struct {
	At      time.Time
	Reason  string
	Subject string
	ToMask  string
	TagType string
}

// DeadLetterEntry is a redacted row shown in admin status when enqueue failed.
type DeadLetterEntry struct {
	At       int64  `json:"at"`
	Reason   string `json:"reason"`
	Subject  string `json:"subject"`
	ToMasked string `json:"to_masked"`
	TagType  string `json:"tag_type,omitempty"`
}

// QueueAdminSnapshot is returned for GET /api/v1/admin/status (email_queue).
type QueueAdminSnapshot struct {
	Enabled            bool              `json:"enabled"`
	Depth              int               `json:"depth"`
	Capacity           int               `json:"capacity"`
	DeadLetter         []DeadLetterEntry `json:"dead_letter"`
	DeadLetterCapacity int               `json:"dead_letter_capacity"`
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

// NewService builds a dispatcher with the default queue (200) and dead-letter buffer (100).
func NewService(sender EmailSender, from Address) *Service {
	return newService(sender, from, defaultQueueCap, defaultDLQCap)
}

func newService(sender EmailSender, from Address, queueCap, dlqCap int) *Service {
	if queueCap < 1 {
		queueCap = 1
	}
	if dlqCap < 1 {
		dlqCap = 1
	}
	s := &Service{
		EmailSender: sender,
		From:        from,
		queue:       make(chan Message, queueCap),
		queueCap:    queueCap,
		dlqCap:      dlqCap,
	}
	go s.worker()
	metrics.SetEmailQueueDepth(0)
	metrics.SetEmailDeadLetterGauge(0)
	return s
}

func (s *Service) Enabled() bool {
	return s != nil && s.EmailSender != nil && s.From.Email != ""
}

// QueueAdminSnapshot returns queue depth, capacity, and recent dead-letter rows for admin APIs.
func (s *Service) QueueAdminSnapshot() QueueAdminSnapshot {
	if s == nil {
		return QueueAdminSnapshot{}
	}
	out := QueueAdminSnapshot{
		Enabled:            s.Enabled(),
		Capacity:           s.queueCap,
		DeadLetterCapacity: s.dlqCap,
	}
	out.Depth = len(s.queue)

	s.deadMu.Lock()
	out.DeadLetter = make([]DeadLetterEntry, len(s.dead))
	for i := range s.dead {
		d := s.dead[i]
		out.DeadLetter[i] = DeadLetterEntry{
			At:       d.At.Unix(),
			Reason:   d.Reason,
			Subject:  d.Subject,
			ToMasked: d.ToMask,
			TagType:  d.TagType,
		}
	}
	s.deadMu.Unlock()
	return out
}

// Enqueue attempts to queue an email for async sending.
// Returns false if the queue is full or service disabled.
func (s *Service) Enqueue(msg Message) bool {
	if !s.Enabled() {
		return false
	}
	select {
	case s.queue <- msg:
		metrics.SetEmailQueueDepth(len(s.queue))
		return true
	default:
		s.recordDeadLetter(msg, "queue_full")
		metrics.RecordEmailEnqueueDrop("queue_full")
		metrics.SetEmailQueueDepth(len(s.queue))
		return false
	}
}

func (s *Service) recordDeadLetter(msg Message, reason string) {
	subj := msg.Subject
	if len(subj) > 200 {
		subj = subj[:200] + "…"
	}
	tagType := ""
	if msg.Tags != nil {
		tagType = msg.Tags["type"]
	}
	entry := deadLetterInternal{
		At:      time.Now().UTC(),
		Reason:  reason,
		Subject: subj,
		ToMask:  maskEmailAddr(msg.To.Email),
		TagType: tagType,
	}

	s.deadMu.Lock()
	s.dead = append(s.dead, entry)
	if len(s.dead) > s.dlqCap {
		s.dead = append([]deadLetterInternal(nil), s.dead[len(s.dead)-s.dlqCap:]...)
	}
	n := len(s.dead)
	s.deadMu.Unlock()

	metrics.SetEmailDeadLetterGauge(n)

	logging.WithComponent(logging.ComponentEmail).WithFields(log.Fields{
		logging.FieldEvent: "dead_letter",
		"reason":           reason,
		"subject":          subj,
		"to_masked":        entry.ToMask,
		"tag_type":         tagType,
	}).Warn("email enqueue failed; message recorded in dead letter buffer")
}

func maskEmailAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	at := strings.LastIndex(addr, "@")
	if at <= 0 || at == len(addr)-1 {
		r := []rune(addr)
		if len(r) > 8 {
			return string(r[:4]) + "…"
		}
		return "[redacted]"
	}
	local, domain := addr[:at], addr[at+1:]
	if local == "" {
		return "***@" + domain
	}
	r := []rune(local)
	first := string(r[0])
	if utf8.RuneCountInString(local) == 1 {
		return first + "***@" + domain
	}
	return first + "***@" + domain
}

func (s *Service) worker() {
	for msg := range s.queue {
		if !s.Enabled() {
			metrics.SetEmailQueueDepth(len(s.queue))
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = s.EmailSender.Send(ctx, s.From, msg)
		cancel()
		metrics.SetEmailQueueDepth(len(s.queue))
	}
}

func (a Address) String() string {
	if a.Name == "" {
		return a.Email
	}
	return fmt.Sprintf("%s <%s>", a.Name, a.Email)
}
