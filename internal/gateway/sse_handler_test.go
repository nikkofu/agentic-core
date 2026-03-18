package gateway

import (
	"bytes"
	"testing"
)

func TestWriteSSEFrame(t *testing.T) {
	buf := &bytes.Buffer{}
	err := WriteSSEFrame(buf, "delta", []byte(`{"content": "hi"}`))
	if err != nil {
		t.Fatalf("failed to write frame: %v", err)
	}

	expected := "event: delta\ndata: {\"content\": \"hi\"}\n\n"
	if buf.String() != expected {
		t.Errorf("expected frame:\n%q\ngot:\n%q", expected, buf.String())
	}
}

func TestWriteDoneFrame(t *testing.T) {
	buf := &bytes.Buffer{}
	WriteDoneFrame(buf)
	
	expected := "event: done\ndata: [DONE]\n\n"
	if buf.String() != expected {
		t.Errorf("expected frame:\n%q\ngot:\n%q", expected, buf.String())
	}
}
