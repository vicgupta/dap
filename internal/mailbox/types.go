// Package mailbox holds the mail documents the broker and the providers share.
// It has no provider clients, so those clients can import it without a cycle.
package mailbox

import "time"

// Thread is one conversation normalized across Gmail, Graph, and the demo mailbox.
type Thread struct {
	ID         string    `json:"id"`
	MessageID  string    `json:"message_id,omitempty"`
	Provider   string    `json:"provider"`
	Subject    string    `json:"subject"`
	WithName   string    `json:"with_name"`
	WithEmail  string    `json:"with_email"`
	LastAt     time.Time `json:"last_at"`
	LastFromMe bool      `json:"last_from_me"`
	Snippet    string    `json:"snippet,omitempty"`
	Title      string    `json:"title,omitempty"`
	Why        string    `json:"why,omitempty"`
	Draft      string    `json:"draft,omitempty"`
	Promise    bool      `json:"promise,omitempty"`
	Short      bool      `json:"short,omitempty"`
	Evidence   string    `json:"evidence,omitempty"`
}

// SendRequest is one approved message.
type SendRequest struct {
	Provider  string `json:"provider"`
	ThreadID  string `json:"thread_id"`
	MessageID string `json:"message_id,omitempty"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	CardID    string `json:"card_id"`
}

// SendResult is the provider's id for the message that went out.
type SendResult struct {
	Provider  string    `json:"provider"`
	MessageID string    `json:"message_id"`
	ThreadID  string    `json:"thread_id"`
	To        string    `json:"to"`
	Subject   string    `json:"subject"`
	SentAt    time.Time `json:"sent_at"`
}

// Throttle is exposed to doctor and the Connections screen.
type Throttle struct {
	Active bool      `json:"active"`
	Until  time.Time `json:"until,omitempty"`
	Status int       `json:"status,omitempty"`
}

// Status is safe to show and to return to plugins. It never contains a token.
type Status struct {
	Provider        string   `json:"provider"`
	Label           string   `json:"label"`
	Connected       bool     `json:"connected"`
	Account         string   `json:"account,omitempty"`
	Scopes          []string `json:"scopes,omitempty"`
	SendGranted     bool     `json:"send_granted"`
	RefreshOK       bool     `json:"refresh_ok"`
	RefreshDetail   string   `json:"refresh_detail,omitempty"`
	Throttle        Throttle `json:"throttle"`
	ClientIDSet     bool     `json:"client_id_set"`
	ClientID        string   `json:"client_id,omitempty"`
	CalendarEnabled bool     `json:"calendar_enabled"`
	Broker          string   `json:"broker,omitempty"`
	Installed       bool     `json:"installed"`
}
