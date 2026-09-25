package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseChecksums(t *testing.T) {
	zipHash := strings.Repeat("a", 64)
	manifestHash := strings.Repeat("B", 64)
	got := parseChecksums("```\n" + zipHash + "  localize_jp.zip\n" + manifestHash + " manifest.json\n```")
	if got["localize_jp.zip"] != zipHash {
		t.Fatalf("ZIP hash = %q", got["localize_jp.zip"])
	}
	if got["manifest.json"] != strings.ToLower(manifestHash) {
		t.Fatalf("manifest hash = %q", got["manifest.json"])
	}
}

func TestSafeTag(t *testing.T) {
	for _, tag := range []string{"v1.115.0-2026092102", "release_1.0"} {
		if !safeTag.MatchString(tag) {
			t.Errorf("expected safe tag %q", tag)
		}
	}
	for _, tag := range []string{"", ".", "..", "../escape", "/absolute", strings.Repeat("a", 129)} {
		if safeTag.MatchString(tag) {
			t.Errorf("expected unsafe tag %q", tag)
		}
	}
}

func TestValidIPv4(t *testing.T) {
	got := validIPv4([]string{"104.18.16.32", "bad", "2001:db8::1", "104.18.16.32", "104.18.17.32"})
	if strings.Join(got, ",") != "104.18.16.32,104.18.17.32" {
		t.Fatalf("validIPv4 = %v", got)
	}
}

func TestInitializeCreatesReusableCA(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	certDir := filepath.Join(t.TempDir(), "cacerts")
	if err := initialize(dataDir, certDir); err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(dataDir, "ssl", "ca.crt")
	firstCA, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := readCertificate(caPath)
	if err != nil {
		t.Fatal(err)
	}
	if !ca.IsCA {
		t.Fatal("generated certificate is not a CA")
	}
	installed := filepath.Join(certDir, subjectHashOld(ca.RawSubject)+".0")
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("Android CA file missing: %v", err)
	}
	serverPath := filepath.Join(dataDir, "ssl", "server.crt")
	firstServer, err := os.ReadFile(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := initialize(dataDir, certDir); err != nil {
		t.Fatal(err)
	}
	secondCA, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstCA) != string(secondCA) {
		t.Fatal("existing CA was unexpectedly replaced")
	}
	secondServer, err := os.ReadFile(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstServer) != string(secondServer) {
		t.Fatal("valid server certificate was unexpectedly replaced")
	}
	server, err := readCertificate(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.VerifyHostname(cdnHost); err != nil {
		t.Fatalf("server certificate SAN: %v", err)
	}
}

func TestValidateZipChecksCRC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, err := archive.Create("payload.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := validateZip(path); err != nil {
		t.Fatalf("valid ZIP rejected: %v", err)
	}
}
