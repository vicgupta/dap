package composio

import (
	"encoding/json"
	"strings"
	"time"
)

// Message is one provider message, normalized enough for the mail adapter.
type Message struct {
	ID        string
	ThreadID  string
	Subject   string
	FromName  string
	FromEmail string
	ToEmail   string
	Snippet   string
	At        time.Time
	FromMe    bool
}

// Messages reads a Composio tool payload. fromMeHint marks a sent-folder page.
func Messages(raw json.RawMessage, me string, fromMeHint bool) []Message {
	var msgs []Message
	for _, row := range objects(raw) {
		id := str(row, "messageId", "message_id", "id")
		thread := str(row, "threadId", "thread_id", "conversationId", "conversation_id")
		if thread == "" {
			thread = id
		}
		fromName, fromEmail := person(firstVal(row, "sender", "from", "from_email"))
		_, toEmail := person(firstVal(row, "to", "recipient", "toRecipients", "recipient_email"))
		snippet := snippetOf(row)
		at := when(row)
		fromMe := fromMeHint
		if !fromMe && me != "" && fromEmail != "" && strings.EqualFold(fromEmail, me) {
			fromMe = true
		}
		for _, label := range labels(row) {
			if strings.EqualFold(label, "SENT") || strings.EqualFold(label, "SENTITEMS") {
				fromMe = true
			}
		}
		if id == "" && thread == "" && str(row, "subject") == "" {
			continue
		}
		msgs = append(msgs, Message{
			ID: id, ThreadID: thread, Subject: str(row, "subject"),
			FromName: fromName, FromEmail: fromEmail, ToEmail: toEmail,
			Snippet: snippet, At: at, FromMe: fromMe,
		})
	}
	return msgs
}

// Latest keeps the newest message per thread.
func Latest(msgs []Message) []Message {
	by := map[string]Message{}
	var order []string
	for _, m := range msgs {
		key := m.ThreadID
		if key == "" {
			key = m.ID
		}
		prev, ok := by[key]
		if !ok {
			order = append(order, key)
			by[key] = m
			continue
		}
		if m.At.After(prev.At) {
			by[key] = m
		}
	}
	out := make([]Message, 0, len(order))
	for _, key := range order {
		out = append(out, by[key])
	}
	return out
}

// Email finds an address in a profile payload.
func Email(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	return walkEmail(v)
}

// Identifier finds a draft or message id.
func Identifier(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	return walkID(v, []string{"draft_id", "draftId", "messageId", "message_id", "id"})
}

func objects(raw json.RawMessage) []map[string]any {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	if s, ok := v.(string); ok {
		_ = json.Unmarshal([]byte(s), &v)
	}
	if arr := asObjects(v); len(arr) > 0 {
		return arr
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	if data, ok := m["data"]; ok {
		if s, ok := data.(string); ok {
			_ = json.Unmarshal([]byte(s), &data)
		}
		if arr := asObjects(data); len(arr) > 0 {
			return arr
		}
		if dm, ok := data.(map[string]any); ok {
			m = dm
		}
	}
	keys := []string{"messages", "emails", "value", "items", "results", "threads"}
	for _, key := range keys {
		if arr := asObjects(m[key]); len(arr) > 0 {
			return arr
		}
	}
	for _, val := range m {
		dm, ok := val.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range keys {
			if arr := asObjects(dm[key]); len(arr) > 0 {
				return arr
			}
		}
	}
	return nil
}

func asObjects(v any) []map[string]any {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	var out []map[string]any
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil
		}
		out = append(out, m)
	}
	return out
}

func str(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func firstVal(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if v, ok := m[key]; ok && v != nil {
			return v
		}
	}
	return nil
}

func person(v any) (name, email string) {
	switch t := v.(type) {
	case string:
		return parseAddr(t)
	case []any:
		if len(t) == 0 {
			return "", ""
		}
		return person(t[0])
	case map[string]any:
		if nested, ok := t["emailAddress"].(map[string]any); ok {
			return person(nested)
		}
		name = str(t, "name", "displayName")
		email = str(t, "address", "email", "emailAddress")
		if email == "" {
			if s := str(t, "sender", "from"); s != "" {
				return parseAddr(s)
			}
		}
		return name, email
	default:
		return "", ""
	}
}

func parseAddr(s string) (name, email string) {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "<"); i >= 0 && strings.HasSuffix(s, ">") {
		name = strings.Trim(strings.TrimSpace(s[:i]), `"`)
		email = strings.Trim(s[i+1:len(s)-1], " <>")
		return name, email
	}
	if strings.Contains(s, "@") {
		return "", s
	}
	return s, ""
}

func snippetOf(m map[string]any) string {
	if s := str(m, "snippet", "bodyPreview", "preview", "messageText"); s != "" {
		return s
	}
	if p, ok := m["preview"].(map[string]any); ok {
		return str(p, "body", "text", "snippet")
	}
	return ""
}

func labels(m map[string]any) []string {
	v, ok := m["labelIds"]
	if !ok {
		v = m["labels"]
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func when(m map[string]any) time.Time {
	for _, key := range []string{"messageTimestamp", "receivedDateTime", "sentDateTime", "internalDate", "date", "time"} {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case string:
			if ts, err := time.Parse(time.RFC3339, t); err == nil {
				return ts
			}
			if ts, err := time.Parse(time.RFC3339Nano, t); err == nil {
				return ts
			}
		case float64:
			if t > 1e12 {
				return time.UnixMilli(int64(t))
			}
			if t > 1e9 {
				return time.Unix(int64(t), 0)
			}
		}
	}
	return time.Time{}
}

func walkEmail(v any) string {
	switch t := v.(type) {
	case map[string]any:
		for _, key := range []string{"emailAddress", "email", "mail", "userPrincipalName"} {
			if s, ok := t[key].(string); ok && strings.Contains(s, "@") {
				return s
			}
		}
		for _, val := range t {
			if s := walkEmail(val); s != "" {
				return s
			}
		}
	case []any:
		for _, item := range t {
			if s := walkEmail(item); s != "" {
				return s
			}
		}
	}
	return ""
}

func walkID(v any, keys []string) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range keys {
		if s, ok := m[key].(string); ok && s != "" && !strings.Contains(s, "@") {
			return s
		}
	}
	for _, key := range []string{"data", "response_data", "draft"} {
		if nested, ok := m[key]; ok {
			if s := walkID(nested, keys); s != "" {
				return s
			}
		}
	}
	return ""
}

func findString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	if err, ok := m["error"].(map[string]any); ok {
		if s := str(err, "message"); s != "" {
			return s
		}
	}
	return ""
}
