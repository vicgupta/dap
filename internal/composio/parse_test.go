package composio

import (
	"encoding/json"
	"testing"
)

func TestMessagesGmailAndOutlook(t *testing.T) {
	gmail := json.RawMessage(`{"messages":[{"messageId":"m1","threadId":"t1","subject":"Q4","sender":"Dana Lee <dana@northwind.co>","to":"vic@example.com","messageTimestamp":"2026-10-01T12:00:00Z","preview":"hello","labelIds":["INBOX"]}]}`)
	got := Messages(gmail, "vic@example.com", false)
	if len(got) != 1 || got[0].ThreadID != "t1" || got[0].FromEmail != "dana@northwind.co" || got[0].FromMe {
		t.Fatalf("%+v", got)
	}
	outlook := json.RawMessage(`{"value":[{"id":"AAMk","conversationId":"conv","subject":"Re: Q4","from":{"emailAddress":{"name":"Vic","address":"vic@example.com"}},"toRecipients":[{"emailAddress":{"address":"dana@northwind.co"}}],"receivedDateTime":"2026-10-02T12:00:00Z","bodyPreview":"circling"}]}`)
	sent := Messages(outlook, "vic@example.com", true)
	if len(sent) != 1 || !sent[0].FromMe || sent[0].ToEmail != "dana@northwind.co" {
		t.Fatalf("%+v", sent)
	}
	latest := Latest(append(got, sent...))
	if len(latest) != 2 {
		t.Fatalf("threads %d", len(latest))
	}
}

func TestEmailAndDraftID(t *testing.T) {
	if Email(json.RawMessage(`{"emailAddress":"vic@example.com"}`)) != "vic@example.com" {
		t.Fatal("gmail profile")
	}
	if Email(json.RawMessage(`{"mail":"vic@outlook.com"}`)) != "vic@outlook.com" {
		t.Fatal("outlook profile")
	}
	if Identifier(json.RawMessage(`{"id":"r-1"}`)) != "r-1" {
		t.Fatal("draft id")
	}
	if Identifier(json.RawMessage(`{"data":{"messageId":"m9"}}`)) != "m9" {
		t.Fatal("nested id")
	}
}
