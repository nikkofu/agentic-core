package wecom

import "testing"

func TestCodecVerifyURLRoundTrip(t *testing.T) {
	codec, err := NewCodec(Config{
		CorpID:         "ww-test-corp",
		Token:          "gateway-token",
		EncodingAESKey: "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
	})
	if err != nil {
		t.Fatalf("NewCodec failed: %v", err)
	}

	encrypted, err := codec.Encrypt([]byte("echo-ok"))
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	timestamp := "1773811200"
	nonce := "nonce-1"
	signature := codec.Signature(timestamp, nonce, encrypted)

	got, err := codec.VerifyURL(signature, timestamp, nonce, encrypted)
	if err != nil {
		t.Fatalf("VerifyURL failed: %v", err)
	}
	if got != "echo-ok" {
		t.Fatalf("expected echo-ok, got %s", got)
	}
}
