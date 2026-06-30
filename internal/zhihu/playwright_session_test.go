package zhihu

import (
	"strings"
	"testing"
)

func TestMarshalBrowserRequestBodyPreservesLargeIntegerIDs(t *testing.T) {
	body, err := marshalBrowserRequestBody(map[string]any{
		"article_id": int64(2055401990973400552),
	})
	if err != nil {
		t.Fatalf("marshalBrowserRequestBody returned error: %v", err)
	}
	if !strings.Contains(body, `"article_id":2055401990973400552`) {
		t.Fatalf("body = %s, want exact integer literal", body)
	}
}
