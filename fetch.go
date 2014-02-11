package main

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	ftp "github.com/jlaffaye/goftp"
)

// fetcher provides an interface for retrieving IMDB data files.
// This abstract over where the data files come from: local directory, HTTP,
// FTP, etc.
type fetcher interface {
	list(name string) io.ReadCloser
}

// newFetcher returns a fetcher based on the uri given. The uri may be a
// preset FTP site ("berlin", "digital", "funet" or "uiuc"), a full FTP or
// HTTP URL containing IMDB's list files, or a local directory containing
// IMDB's list files.
func newFetcher(uri string) fetcher {
	if v, ok := namedFtp[uri]; ok {
		uri = v
	}
	if !strings.HasPrefix(uri, "http") && !strings.HasPrefix(uri, "ftp") {
		return dirFetcher(uri)
	}

	loc, err := url.Parse(uri)
	if err != nil {
		pef("Could not parse URL '%s': %s", uri, err)
		return nil
	}
	switch loc.Scheme {
	case "http":
		return httpFetcher{loc}
	case "ftp":
		// It seems like FTP sites---even if they are public---require a
		// trivial login.
		if loc.User == nil {
			loc.User = url.UserPassword("anonymous", "anonymous")
		}
		// The FTP package I'm using requires a port number.
		// (Actually, I think it might be the net/textproto package.)
		if !strings.Contains(loc.Host, ":") {
			loc.Host += ":21"
		}
		return ftpFetcher{loc}
	}
	pef("Unsupported URL scheme '%s' in '%s'.", loc.Scheme, uri)
	return nil
}

// dirFetcher satisfies the fetcher interface by reading from a local
// directory.
type dirFetcher string

func (df dirFetcher) list(name string) io.ReadCloser {
	fpath := path.Join(string(df), sf("%s.list.gz", name))
	return openFile(fpath)
}

// httpFetcher satisfies the fetcher interface by reading from an HTTP URL.
type httpFetcher struct {
	*url.URL
}

func (hf httpFetcher) list(name string) io.ReadCloser {
	uri := sf("%s/%s.list.gz", hf.String(), name)
	resp, err := http.Get(uri)
	if err != nil {
		fatalf("Could not download '%s': %s", uri, err)
	}
	return resp.Body
}

// ftpFetcher satisfies the fetcher interface by reading from an FTP URL.
// Each fetcher opens a new FTP connection.
type ftpFetcher struct {
	*url.URL
}

// ftpRetrCloser syncs the closing of the file download with the closing of
// the connection.
type ftpRetrCloser struct {
	io.ReadCloser
	conn *ftp.ServerConn
}

// Close closes the file download and the FTP connection.
func (r *ftpRetrCloser) Close() error {
	defer func() { r.ReadCloser = nil }()

	if r.ReadCloser == nil {
		return nil
	}
	if err := r.ReadCloser.Close(); err != nil {
		pef("Problem closing FTP reader: %s", err)
		return ef("Problem closing FTP reader: %s", err)
	}
	// if err := r.conn.Quit(); err != nil {
	// pef("Error disconnecting from FTP server: %s", err)
	// return ef("Problem quitting: %s", err)
	// }

	// We aren't quitting the conn here becasue we're sharing one global
	// FTP connection. For whatever reason, I couldn't get multiple
	// connections to work right---even after closing them, they seem to
	// remain open. (The IMDb mirrors block more than 5 concurrent connections.)
	//
	// The result here is that the FTP connection won't be closed until the
	// program quits. I'm fine with that for now.
	return nil
}

var (
	ftpConn *ftp.ServerConn = nil
)

func (ff ftpFetcher) list(name string) io.ReadCloser {
	if ftpConn == nil {
		var err error
		ftpConn, err = ftp.Connect(ff.Host)
		if err != nil {
			fatalf("Could not connect to '%s': %s", ff.Host, err)
		}

		pass, _ := ff.User.Password()
		if err := ftpConn.Login(ff.User.Username(), pass); err != nil {
			fatalf("Authentication failed for '%s': %s", ff.Host, err)
		}
	}

	namePath := sf("%s/%s.list.gz", ff.Path, name)
	r, err := ftpConn.Retr(namePath)
	if err != nil {
		fatalf("Could not retrieve '%s' from '%s': %s", namePath, ff.Host, err)
	}
	return &ftpRetrCloser{r, ftpConn}
}

// bufCloser makes a bytes.Buffer satisfy the io.ReadCloser interface.
type bufCloser struct {
	*bytes.Buffer
}

func (bc bufCloser) Close() error {
	return nil
}

// saver wraps any fetcher value and saves anything retrieved to the directory
// in `saveto`. If `saveto` has length 0, then the file isn't saved.
//
// saver also satisfies the fetcher interface itself.
type saver struct {
	fetcher
	saveto string
}

func (s saver) list(name string) io.ReadCloser {
	r := s.fetcher.list(name)
	if len(s.saveto) == 0 {
		return r
	}

	buf := new(bytes.Buffer)
	saveto := createFile(path.Join(s.saveto, sf("%s.list.gz", name)))
	tee := io.TeeReader(r, buf)
	if _, err := io.Copy(saveto, tee); err != nil {
		fatalf("Could not save list '%s' to disk: %s", name, err)
	}
	if err := r.Close(); err != nil {
		fatalf("Could not close reader for '%s': %s", name, err)
	}
	return bufCloser{buf}
}
