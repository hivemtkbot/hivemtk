package fingerprint

import (
	"testing"
)

func TestGenerateFingerprint(t *testing.T) {
	fp, err := GenerateFingerprint()

	if err != nil {
		t.Fatalf("GenerateFingerprint failed: %v", err)
	}

	if fp == "" {
		t.Error("Fingerprint should not be empty")
	}
}

func TestFingerprintFormat(t *testing.T) {
	fp, err := GenerateFingerprint()

	if err != nil {
		t.Fatalf("GenerateFingerprint failed: %v", err)
	}

	if len(fp) != 32 {
		t.Errorf("Expected fingerprint length 32, got %d", len(fp))
	}

	for _, c := range fp {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("Invalid character in fingerprint: %c", c)
		}
	}
}

func TestFingerprintConsistency(t *testing.T) {
	fp1, err := GenerateFingerprint()
	if err != nil {
		t.Fatalf("GenerateFingerprint failed: %v", err)
	}

	fp2, err := GenerateFingerprint()
	if err != nil {
		t.Fatalf("GenerateFingerprint failed: %v", err)
	}

	if fp1 != fp2 {
		t.Error("Fingerprints from same machine should be identical")
	}
}

func TestFingerprintUniqueness(t *testing.T) {
	fingerprints := make(map[string]bool)

	for i := 0; i < 10; i++ {
		fp, err := GenerateFingerprint()
		if err != nil {
			t.Fatalf("GenerateFingerprint failed: %v", err)
		}
		fingerprints[fp] = true
	}

	if len(fingerprints) != 1 {
		t.Error("All fingerprints from same machine should be identical")
	}
}

