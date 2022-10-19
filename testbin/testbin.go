package testbin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"text/template"

	"emperror.dev/errors"

	"github.com/kralicky/spellbook/internal/deps"
	"github.com/mholt/archiver/v4"
)

var testbinDeps = deps.New()

var (
	Deps          = testbinDeps.Deps
	SerialDeps    = testbinDeps.SerialDeps
	CtxDeps       = testbinDeps.CtxDeps
	SerialCtxDeps = testbinDeps.SerialCtxDeps
)

type GetVersionFunc func(bin string) string

type Binary struct {
	Name       string
	Version    string
	URL        string
	GetVersion GetVersionFunc
	// Optional: only specify if the binary is not located at <top-level-dir>/name
	// or <top-level-dir>/bin/name within the archive.
	PathInArchive string
}

var Config = struct {
	Binaries []Binary
	Dir      string
}{
	Dir: "testbin/bin",
}

func fallbackUntar(binaryName, dst string, r io.Reader) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()

		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		case header == nil:
			continue
		}
		if !strings.HasPrefix(header.Name, binaryName) {
			continue
		}
		target := filepath.Join(dst, binaryName)

		switch header.Typeflag {
		case tar.TypeDir:
			// ignore
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return err
			}
			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
		}
	}
}

func Testbin() error {
	testbinDeps.Resolve()
	if _, err := os.Stat(Config.Dir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(Config.Dir, 0755); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	wg := sync.WaitGroup{}
	errs := []error{}
	mu := sync.Mutex{}
	for _, binary := range Config.Binaries {
		wg.Add(1)
		go func(binary Binary) {
			defer wg.Done()
			needsDownload := false
			exists := false
			if _, err := os.Stat(filepath.Join(Config.Dir, binary.Name)); err == nil {
				exists = true
			}
			if !exists {
				fmt.Printf("%s binary missing\n", binary.Name)
				needsDownload = true
			} else {
				curVersion := binary.GetVersion(filepath.Join(Config.Dir, binary.Name))
				if curVersion != binary.Version {
					fmt.Printf("%s binary version mismatch (have %s, want %s)\n", binary.Name, curVersion, binary.Version)
					needsDownload = true
				} else {
					fmt.Printf("%s binary up to date\n", binary.Name)
				}
			}
			if needsDownload {
				if err := downloadBinary(binary); err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
					return
				}
			}
		}(binary)
	}
	wg.Wait()
	return errors.Combine(errs...)
}

type binaryTemplateData struct {
	Version string
	GOOS    string
	GOARCH  string
}

func downloadBinary(binary Binary) error {
	fmt.Printf("downloading %s version %s...\n", binary.Name, binary.Version)
	tempDir, err := os.MkdirTemp("", "testbin-download-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	tmpl, err := template.New("download").Parse(binary.URL)
	if err != nil {
		return err
	}
	var url bytes.Buffer
	if err := tmpl.Execute(&url, binaryTemplateData{
		Version: binary.Version,
		GOOS:    runtime.GOOS,
		GOARCH:  runtime.GOARCH,
	}); err != nil {
		return err
	}

	resp, err := http.Get(url.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	archiveFile := filepath.Join(tempDir, binary.Name+"-archive")
	f, err := os.Create(archiveFile)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	f.Close()

	fsys, err := archiver.FileSystem(archiveFile)
	if err != nil {
		return err
	}

	var fsFile fs.File
	if bin, err := fsys.Open(binary.Name); err == nil {
		fsFile = bin
	} else if bin, err := fsys.Open(filepath.Base(archiveFile)); err == nil {
		fsFile = bin
	} else if bin, err := fsys.Open(binary.PathInArchive); binary.PathInArchive != "" && err == nil {
		fsFile = bin
	} else {
		var topLevelDir string
		entries, err := fs.ReadDir(fsys, ".")
		if err != nil {
			return err
		}
		if len(entries) == 0 { // something is wrong, try fallback
			archiveReader, err := os.Open(archiveFile)
			if err != nil {
				archiveReader.Close()
				return err
			}
			fallbackUntar(binary.Name, Config.Dir, archiveReader)
			archiveReader.Close()
			return nil
		} else if len(entries) == 1 {
			topLevelDir = entries[0].Name()
		} else {
			return errors.New(fmt.Sprintf("unexpected number of top-level directories in archive (%d)", len(entries)))
		}
		if bin, err := fsys.Open(filepath.Join(topLevelDir, binary.Name)); err == nil {
			fsFile = bin
		} else if bin, err := fsys.Open(filepath.Join(topLevelDir, "bin", binary.Name)); err == nil {
			fsFile = bin
		} else if bin, err := fsys.Open(filepath.Join(topLevelDir, binary.PathInArchive)); binary.PathInArchive != "" && err == nil {
			fsFile = bin
		} else {
			return errors.New("could not auto-detect binary in archive")
		}
	}

	bin, err := os.OpenFile(filepath.Join(Config.Dir, binary.Name), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer bin.Close()
	if _, err := io.Copy(bin, fsFile); err != nil {
		return err
	}
	return nil
}
