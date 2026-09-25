package main

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"
)

const (
	cdnHost     = "downloadcommon.limbuscompanycdn.org"
	defaultRepo = "TIMER-err/Limbus-Company-Mobile-Localization"
)

var (
	safeTag       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	checksumLine  = regexp.MustCompile(`(?im)^([0-9a-f]{64})\s+(localize_jp\.zip|manifest\.json)$`)
	fallbackHosts = []string{"104.18.16.32", "104.18.17.32"}
)

type release struct {
	Tag    string `json:"tag_name"`
	Body   string `json:"body"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

type dnsResponse struct {
	Answer []struct {
		Type int    `json:"type"`
		Data string `json:"data"`
	} `json:"Answer"`
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	if len(os.Args) < 2 {
		fatalf("用法：%s <init|serve|update|status>", filepath.Base(os.Args[0]))
	}

	switch os.Args[1] {
	case "init":
		fs := flag.NewFlagSet("init", flag.ExitOnError)
		dataDir := fs.String("data", "", "模块数据目录")
		certDir := fs.String("cert-dir", "", "system CA 覆盖目录")
		_ = fs.Parse(os.Args[2:])
		requireDir(*dataDir, "data")
		requireDir(*certDir, "cert-dir")
		if err := initialize(*dataDir, *certDir); err != nil {
			fatalf("初始化失败：%v", err)
		}
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		dataDir := fs.String("data", "", "模块数据目录")
		listen := fs.String("listen", "127.0.0.1:443", "监听地址")
		_ = fs.Parse(os.Args[2:])
		requireDir(*dataDir, "data")
		if err := serve(*dataDir, *listen); err != nil {
			fatalf("服务退出：%v", err)
		}
	case "update":
		fs := flag.NewFlagSet("update", flag.ExitOnError)
		dataDir := fs.String("data", "", "模块数据目录")
		repo := fs.String("repo", defaultRepo, "Release 仓库")
		_ = fs.Parse(os.Args[2:])
		requireDir(*dataDir, "data")
		tag, changed, err := update(*dataDir, *repo)
		if err != nil {
			fatalf("更新失败：%v", err)
		}
		if changed {
			fmt.Printf("已更新到 %s\n", tag)
		} else {
			fmt.Printf("已是最新版本：%s\n", tag)
		}
	case "status":
		fs := flag.NewFlagSet("status", flag.ExitOnError)
		dataDir := fs.String("data", "", "模块数据目录")
		_ = fs.Parse(os.Args[2:])
		requireDir(*dataDir, "data")
		if err := status(*dataDir); err != nil {
			fatalf("状态检查失败：%v", err)
		}
	default:
		fatalf("未知命令：%s", os.Args[1])
	}
}

func requireDir(value, name string) {
	if value == "" || !filepath.IsAbs(value) {
		fatalf("--%s 必须是绝对路径", name)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func initialize(dataDir, certDir string) error {
	sslDir := filepath.Join(dataDir, "ssl")
	for _, dir := range []string{sslDir, certDir, filepath.Join(dataDir, "releases"), filepath.Join(dataDir, "logs")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	if err := os.Chmod(dataDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(certDir, 0o755); err != nil {
		return err
	}

	caCertPath := filepath.Join(sslDir, "ca.crt")
	caKeyPath := filepath.Join(sslDir, "ca.key")
	caCreated := false
	if !regularFileExists(caCertPath) || !regularFileExists(caKeyPath) {
		if err := generateCA(caCertPath, caKeyPath); err != nil {
			return err
		}
		caCreated = true
	}

	caDER, err := readCertificate(caCertPath)
	if err != nil {
		return err
	}
	serverCertPath := filepath.Join(sslDir, "server.crt")
	serverKeyPath := filepath.Join(sslDir, "server.key")
	serverCreated := !serverCertificateReusable(caDER, serverCertPath, serverKeyPath)
	if serverCreated {
		if err := generateServerCertificate(caCertPath, caKeyPath, serverCertPath, serverKeyPath); err != nil {
			return err
		}
	}

	hash := subjectHashOld(caDER.RawSubject)
	destination := filepath.Join(certDir, hash+".0")
	contents, err := os.ReadFile(caCertPath)
	if err != nil {
		return err
	}
	if err := atomicWrite(destination, contents, 0o644); err != nil {
		return err
	}
	caStatus := "复用"
	if caCreated {
		caStatus = "新生成"
	}
	serverStatus := "复用"
	if serverCreated {
		serverStatus = "新签发"
	}
	fmt.Printf("CA：%s\nHTTPS 服务证书：%s\nCA 文件：%s\nCA SHA-256：%s\n", caStatus, serverStatus, destination, fingerprint(caDER.Raw))
	return nil
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func serverCertificateReusable(ca *x509.Certificate, certPath, keyPath string) bool {
	if !regularFileExists(certPath) || !regularFileExists(keyPath) {
		return false
	}
	cert, err := readCertificate(certPath)
	if err != nil || time.Now().Add(30*24*time.Hour).After(cert.NotAfter) || time.Now().Before(cert.NotBefore) {
		return false
	}
	if cert.VerifyHostname(cdnHost) != nil || cert.CheckSignatureFrom(ca) != nil {
		return false
	}
	_, err = tls.LoadX509KeyPair(certPath, keyPath)
	return err == nil
}

func generateCA(certPath, keyPath string) error {
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          randomSerial(),
		Subject:               pkix.Name{CommonName: "Limbus Localization Local CA"},
		NotBefore:             now.Add(-24 * time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}
	if err := writePEM(certPath, "CERTIFICATE", der, 0o644); err != nil {
		return err
	}
	return writePEM(keyPath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key), 0o600)
}

func generateServerCertificate(caCertPath, caKeyPath, certPath, keyPath string) error {
	caCert, err := readCertificate(caCertPath)
	if err != nil {
		return err
	}
	caKeyPEM, err := os.ReadFile(caKeyPath)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(caKeyPEM)
	if block == nil {
		return errors.New("无法解析 CA 私钥")
	}
	caKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: randomSerial(),
		Subject:      pkix.Name{CommonName: cdnHost},
		DNSNames:     []string{cdnHost},
		NotBefore:    now.Add(-24 * time.Hour),
		NotAfter:     now.AddDate(2, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return err
	}
	if err := writePEM(certPath, "CERTIFICATE", der, 0o644); err != nil {
		return err
	}
	return writePEM(keyPath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key), 0o600)
}

func randomSerial() *big.Int {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		panic(err)
	}
	return serial
}

func writePEM(path, kind string, der []byte, mode os.FileMode) error {
	return atomicWrite(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}), mode)
}

func atomicWrite(path string, contents []byte, mode os.FileMode) error {
	tmp := path + ".new"
	if err := os.WriteFile(tmp, contents, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readCertificate(path string) (*x509.Certificate, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(contents)
	if block == nil {
		return nil, fmt.Errorf("无法解析证书：%s", path)
	}
	return x509.ParseCertificate(block.Bytes)
}

func subjectHashOld(rawSubject []byte) string {
	sum := md5.Sum(rawSubject) // Android 证书文件名沿用 OpenSSL X509_NAME_hash_old。
	return fmt.Sprintf("%08x", binary.LittleEndian.Uint32(sum[:4]))
}

func fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	parts := make([]string, len(sum))
	for i, value := range sum {
		parts[i] = fmt.Sprintf("%02X", value)
	}
	return strings.Join(parts, ":")
}

func serve(dataDir, listen string) error {
	origins := loadOrigins(dataDir)
	var counter atomic.Uint64
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:               nil,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        16,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, ServerName: cdnHost},
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var lastErr error
			start := int(counter.Add(1)-1) % len(origins)
			for i := range origins {
				address := net.JoinHostPort(origins[(start+i)%len(origins)], "443")
				conn, err := dialer.DialContext(ctx, "tcp", address)
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
	}
	target := &url.URL{Scheme: "https", Host: cdnHost}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = transport
	proxy.ErrorLog = log.New(os.Stderr, "回源失败：", log.LstdFlags)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		log.Printf("回源失败：%v", err)
		http.Error(w, "official CDN unavailable", http.StatusBadGateway)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.ToLower(filepath.Base(r.URL.Path))
		switch name {
		case "localizepatchinfo.json":
			serveFile(w, r, filepath.Join(dataDir, "current", "manifest.json"), "application/json", "manifest")
		case "localize_jp.zip":
			serveFile(w, r, filepath.Join(dataDir, "current", "localize_jp.zip"), "application/zip", "archive")
		default:
			r.Host = cdnHost
			proxy.ServeHTTP(w, r)
		}
	})

	server := &http.Server{
		Addr:              listen,
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
	log.Printf("监听 %s，官方回源 %s", listen, strings.Join(origins, ","))
	return server.ListenAndServeTLS(filepath.Join(dataDir, "ssl", "server.crt"), filepath.Join(dataDir, "ssl", "server.key"))
}

func serveFile(w http.ResponseWriter, r *http.Request, path, contentType, kind string) {
	file, err := os.Open(path)
	if err != nil {
		http.Error(w, "localization resource unavailable", http.StatusServiceUnavailable)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.Error(w, "localization resource unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Limbus-Localization", kind)
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}

func loadOrigins(dataDir string) []string {
	path := filepath.Join(dataDir, "origins.json")
	contents, err := os.ReadFile(path)
	if err == nil {
		var origins []string
		if json.Unmarshal(contents, &origins) == nil {
			origins = validIPv4(origins)
			if len(origins) > 0 {
				return origins
			}
		}
	}
	return slices.Clone(fallbackHosts)
}

func refreshOrigins(ctx context.Context, dataDir string, client *http.Client) []string {
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://dns.google/resolve?name="+cdnHost+"&type=A", nil)
	request.Header.Set("User-Agent", "limbus-localization-ksu")
	response, err := client.Do(request)
	if err != nil {
		return loadOrigins(dataDir)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return loadOrigins(dataDir)
	}
	var payload dnsResponse
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload) != nil {
		return loadOrigins(dataDir)
	}
	var origins []string
	for _, answer := range payload.Answer {
		if answer.Type == 1 {
			origins = append(origins, answer.Data)
		}
	}
	origins = validIPv4(origins)
	if len(origins) == 0 {
		return loadOrigins(dataDir)
	}
	contents, _ := json.Marshal(origins)
	_ = atomicWrite(filepath.Join(dataDir, "origins.json"), append(contents, '\n'), 0o644)
	return origins
}

func validIPv4(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		ip := net.ParseIP(value)
		if ip != nil && ip.To4() != nil && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func update(dataDir, repo string) (string, bool, error) {
	client := &http.Client{Timeout: 90 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	refreshOrigins(ctx, dataDir, client)

	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "limbus-localization-ksu")
	response, err := client.Do(request)
	if err != nil {
		return "", false, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("GitHub API 返回 %s", response.Status)
	}
	var latest release
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&latest); err != nil {
		return "", false, err
	}
	if !safeTag.MatchString(latest.Tag) {
		return "", false, fmt.Errorf("不安全的 Release 标签：%q", latest.Tag)
	}
	if current, _ := os.ReadFile(filepath.Join(dataDir, "current-version")); strings.TrimSpace(string(current)) == latest.Tag {
		return latest.Tag, false, nil
	}

	assetURLs := map[string]string{}
	for _, asset := range latest.Assets {
		assetURLs[asset.Name] = asset.URL
	}
	for _, name := range []string{"localize_jp.zip", "manifest.json"} {
		if assetURLs[name] == "" {
			return "", false, fmt.Errorf("Release 缺少 %s", name)
		}
	}

	expected := parseChecksums(latest.Body)
	if checksumURL := assetURLs["checksums.sha256"]; checksumURL != "" {
		if contents, err := downloadBytes(ctx, client, checksumURL, 1<<20); err == nil {
			expected = parseChecksums(string(contents))
		}
	}
	if expected["localize_jp.zip"] == "" || expected["manifest.json"] == "" {
		return "", false, errors.New("Release 缺少完整 SHA-256")
	}

	releasesDir := filepath.Join(dataDir, "releases")
	if err := os.MkdirAll(releasesDir, 0o700); err != nil {
		return "", false, err
	}
	staging, err := os.MkdirTemp(releasesDir, ".staging-")
	if err != nil {
		return "", false, err
	}
	defer os.RemoveAll(staging)
	for _, name := range []string{"localize_jp.zip", "manifest.json"} {
		path := filepath.Join(staging, name)
		limit := int64(512 << 20)
		if name == "manifest.json" {
			limit = 16 << 20
		}
		if err := downloadFile(ctx, client, assetURLs[name], path, limit); err != nil {
			return "", false, err
		}
		actual, err := fileSHA256(path)
		if err != nil {
			return "", false, err
		}
		if !strings.EqualFold(actual, expected[name]) {
			return "", false, fmt.Errorf("%s SHA-256 不匹配", name)
		}
	}
	if err := validateZip(filepath.Join(staging, "localize_jp.zip")); err != nil {
		return "", false, fmt.Errorf("ZIP 校验失败：%w", err)
	}
	manifest, err := os.ReadFile(filepath.Join(staging, "manifest.json"))
	if err != nil || !json.Valid(manifest) {
		return "", false, errors.New("manifest.json 不是有效 JSON")
	}

	target := filepath.Join(releasesDir, latest.Tag)
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(staging, target); err != nil {
			return "", false, err
		}
	}
	currentLink := filepath.Join(dataDir, "current")
	oldTarget, _ := os.Readlink(currentLink)
	if oldTarget != "" && oldTarget != filepath.Join("releases", latest.Tag) {
		if err := replaceSymlink(filepath.Join(dataDir, "previous"), oldTarget); err != nil {
			return "", false, err
		}
	}
	if err := replaceSymlink(currentLink, filepath.Join("releases", latest.Tag)); err != nil {
		return "", false, err
	}
	if err := atomicWrite(filepath.Join(dataDir, "current-version"), []byte(latest.Tag+"\n"), 0o644); err != nil {
		return "", false, err
	}
	return latest.Tag, true, nil
}

func parseChecksums(contents string) map[string]string {
	result := map[string]string{}
	for _, match := range checksumLine.FindAllStringSubmatch(contents, -1) {
		result[match[2]] = strings.ToLower(match[1])
	}
	return result
}

func downloadBytes(ctx context.Context, client *http.Client, source string, limit int64) ([]byte, error) {
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	request.Header.Set("User-Agent", "limbus-localization-ksu")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载返回 %s", response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, limit))
}

func downloadFile(ctx context.Context, client *http.Client, source, destination string, limit int64) error {
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	request.Header.Set("User-Agent", "limbus-localization-ksu")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 %s 返回 %s", filepath.Base(destination), response.Status)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, io.LimitReader(response.Body, limit+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if written > limit {
		return fmt.Errorf("下载 %s 超过大小限制", filepath.Base(destination))
	}
	return closeErr
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func validateZip(path string) error {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer archive.Close()
	for _, entry := range archive.File {
		reader, err := entry.Open()
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(io.Discard, reader)
		closeErr := reader.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func replaceSymlink(path, target string) error {
	tmp := path + ".new"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func status(dataDir string) error {
	version, err := os.ReadFile(filepath.Join(dataDir, "current-version"))
	if err != nil {
		return err
	}
	ca, err := readCertificate(filepath.Join(dataDir, "ssl", "ca.crt"))
	if err != nil {
		return err
	}
	fmt.Printf("资源版本：%s\nCA SHA-256：%s\n", strings.TrimSpace(string(version)), fingerprint(ca.Raw))
	return nil
}
