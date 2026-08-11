package posts

import (
	"errors"
	"testing"
)

func TestValidateChannel(t *testing.T) {
	valid := []string{"chat/general", "comments/article-123", "reviews/homepage", "a", "A-Z_0/9"}
	for _, c := range valid {
		if err := ValidateChannel(c); err != nil {
			t.Errorf("ValidateChannel(%q) = %v, want nil", c, err)
		}
	}

	invalid := []string{"", "chat general", "chat/general!", "chat/general?", "chat\\general", "chat:general", "chat/general#", "é"}
	for _, c := range invalid {
		if err := ValidateChannel(c); err == nil {
			t.Errorf("ValidateChannel(%q) = nil, want error", c)
		}
	}

	long := make([]byte, MaxChannelLength+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidateChannel(string(long)); err == nil {
		t.Error("expected error for over-long channel")
	}
}

func TestValidatePostLimits(t *testing.T) {
	limits := Limits{
		MaxTextBytes:     10,
		MaxMetadataBytes: 50,
		MaxMetadataKeys:  2,
		MaxKeyLength:     8,
		MaxURLsPerPost:   1,
	}

	ok := &NewPost{Channel: "chat/general", Text: "hello"}
	if err := limits.Validate(ok); err != nil {
		t.Fatalf("valid post rejected: %v", err)
	}

	if err := limits.Validate(&NewPost{Channel: "chat/general", Text: ""}); err == nil {
		t.Error("empty text accepted")
	}

	if err := limits.Validate(&NewPost{Channel: "chat/general", Text: "12345678901"}); err == nil {
		t.Error("over-long text accepted")
	}

	if err := limits.Validate(&NewPost{Channel: "chat/general", Text: "hi", Meta: map[string]any{"a": 1, "b": 2, "c": 3}}); err == nil {
		t.Error("too many metadata keys accepted")
	}

	if err := limits.Validate(&NewPost{Channel: "chat/general", Text: "hi", Meta: map[string]any{"toolongkey": 1}}); err == nil {
		t.Error("over-long metadata key accepted")
	}

	if err := limits.Validate(&NewPost{Channel: "chat/general", Text: "hi", Meta: map[string]any{"a": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}}); err == nil {
		t.Error("over-large metadata accepted")
	}

	if err := limits.Validate(&NewPost{Channel: "bad channel!", Text: "hi"}); !errors.Is(err, ErrChannel) {
		t.Errorf("invalid channel: got %v, want ErrChannel", err)
	}
}
