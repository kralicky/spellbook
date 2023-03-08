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
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"text/template"

	"emperror.dev/errors"

	"github.com/kralicky/spellbook/internal/deps"
	"github.com/magefile/mage/sh"
	"github.com/mholt/archiver/v4"
)

var testbinDeps = deps.New()

var (
	Deps          = testbinDeps.Deps
	SerialDeps    = testbinDeps.SerialDeps
	CtxDeps       = testbinDeps.CtxDeps
	SerialCtxDeps = testbinDeps.SerialCtxDeps
)

const (
	testBinTypeCompressed = "compressed"
	testBinTypeExecutable = "executable"
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

func fallbackUntar(binaryName, dst string, r io.Reader) (fs.File, error) {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()

		switch {
		case err == io.EOF:
			return nil, err
		case err != nil:
			return nil, err
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
			if strings.HasSuffix(header.Name, binaryName) {
				f, err := os.OpenFile(target+"-extracted", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
				if err != nil {
					return nil, err
				}
				// copy over contents
				if _, err := io.Copy(f, tr); err != nil {
					return nil, err
				}
				//seek to the beginning of the file, because it is copied later
				if _, err := f.Seek(0, io.SeekStart); err != nil {
					return nil, err
				}
				return f, nil
			}

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

func getFileType(filepath string) (string, error) {
	// we default to the more common case of treating release binaries as compressed
	unkownType := testBinTypeCompressed

	buf := make([]byte, 512) // max length required for sniff algorithm of net/http
	f, err := os.Open(filepath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = f.Read(buf)
	if err != nil {
		return "", err
	}
	// the api we get the test binary from can sometimes encode hints about the content type
	encodedFiletype := http.DetectContentType(buf)
	switch encodedFiletype {
	case "application/x-gzip", "application/gzip", "application/zip":
		return testBinTypeCompressed, nil
	case "application/octet-stream":
		// apis like github /download frequently return this type despite having compressed contents
		fallthrough
	default:
		// no-op
	}

	// the api didn't have enough information to evaluate file type, fallback
	// to gnu/gpl file utility

	_, err = exec.LookPath("file")
	if err != nil {
		// this will always fail for windows users that have not manually installed gnu file
		// so this is treated as a recoverable error
		return unkownType, nil
	}

	discoveredProperties, err := sh.Output("file", filepath)
	if err != nil {
		return unkownType, nil
	}

	if strings.Contains("executable", discoveredProperties) {
		return testBinTypeExecutable, nil
	}
	if strings.Contains("compressed", discoveredProperties) {
		return testBinTypeCompressed, nil
	}
	return unkownType, nil
}

func extract(fsys fs.FS, binary Binary, archiveFile string) (fs.File, error) {
	if bin, err := fsys.Open(binary.Name); err == nil {
		return bin, err
	} else if bin, err := fsys.Open(filepath.Base(archiveFile)); err == nil {
		return bin, nil
	} else if bin, err := fsys.Open(binary.PathInArchive); binary.PathInArchive != "" && err == nil {
		return bin, err
	} else {
		var topLevelDir string
		entries, err := fs.ReadDir(fsys, ".")
		if err != nil {
			return nil, err
		}
		if len(entries) == 0 { // something is wrong, try fallback
			archiveReader, err := os.Open(archiveFile)
			if err != nil {
				return nil, err
			}
			defer archiveReader.Close()
			return fallbackUntar(binary.Name, path.Dir(archiveFile), archiveReader)
		} else if len(entries) == 1 {
			topLevelDir = entries[0].Name()
		} else {
			return nil, errors.New(fmt.Sprintf("unexpected number of top-level directories in archive (%d)", len(entries)))
		}
		if bin, err := fsys.Open(filepath.Join(topLevelDir, binary.Name)); err == nil {
			return bin, nil
		} else if bin, err := fsys.Open(filepath.Join(topLevelDir, "bin", binary.Name)); err == nil {
			return bin, nil
		} else if bin, err := fsys.Open(filepath.Join(topLevelDir, binary.PathInArchive)); binary.PathInArchive != "" && err == nil {
			return bin, nil
		} else {
			return nil, errors.New("could not auto-detect binary in archive")
		}
	}
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

	filetype, err := getFileType(archiveFile)
	if err != nil {
		return err
	}
	// assumption here is the fs.File is opened but not closed
	var fsFile fs.File
	defer func() {
		if fsFile != nil {
			fsFile.Close()
		}
	}()
	var fTypeErr error
	switch filetype {
	case testBinTypeCompressed:
		fsFile, fTypeErr = extract(fsys, binary, archiveFile)
	case testBinTypeExecutable:
		fsFile, fTypeErr = f, nil
	default:
		fTypeErr = fmt.Errorf("unknown detected file type, cannot be handled by testbin target: %s", filetype)
	}
	if fTypeErr != nil {
		return fTypeErr
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
