package domain

import (
	"net/url"
	"strings"
	"unicode"
)

type Remote struct {
	Name, URL string
}

// RemoteAddress accepts web URLs, SSH URLs and Git's scp-style SSH addresses.
func RemoteAddress(raw string) (*url.URL, bool) {
	if raw == "" || strings.ContainsRune(raw, '\\') || strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
		return nil, false
	}
	if !strings.Contains(raw, "://") {
		host, path, ok := strings.Cut(raw, ":")
		if !ok || strings.ContainsAny(host, "/?#") || len(host) < 2 || path == "" {
			return nil, false
		}
		raw = "ssh://" + host + "/" + strings.TrimLeft(path, "/")
	}
	addr, err := url.Parse(raw)
	if err != nil || addr.Hostname() == "" || strings.Trim(addr.Path, "/") == "" || addr.RawQuery != "" || addr.ForceQuery || addr.Fragment != "" {
		return nil, false
	}
	switch addr.Scheme {
	case "http", "https", "git":
		if addr.User != nil {
			return nil, false
		}
	case "ssh":
		if addr.User != nil {
			if _, password := addr.User.Password(); password {
				return nil, false
			}
		}
	default:
		return nil, false
	}
	return addr, true
}

// RemoteWebURL identifies the public hosts supported by the hosting menus.
func RemoteWebURL(raw string) (provider, webURL string) {
	addr, ok := RemoteAddress(raw)
	if !ok {
		return "", ""
	}
	host := strings.ToLower(addr.Hostname())
	switch host {
	case "github.com":
		provider = "GitHub"
	case "gitee.com":
		provider = "Gitee"
	case "gitlab.com":
		provider = "GitLab"
	default:
		return "", ""
	}
	path := strings.TrimSuffix(strings.Trim(addr.Path, "/"), ".git")
	segments := strings.Split(path, "/")
	if len(segments) < 2 || (provider != "GitLab" && len(segments) != 2) {
		return "", ""
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || strings.HasPrefix(segment, "-") {
			return "", ""
		}
	}
	return provider, (&url.URL{Scheme: "https", Host: host, Path: "/" + path}).String()
}
