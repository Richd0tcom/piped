package filemanager

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"

	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	// "github.com/go-git/go-git/v5/plumbing"
	// "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/richd0tcom/piped/internal/models"
)

type FileManager struct {
	baseDir string
}

func New(baseDir string) (*FileManager, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}
	return &FileManager{baseDir: baseDir}, nil
}

func (f *FileManager) TempDir() (string, error) {
	return os.MkdirTemp(f.baseDir, "build-*")
}

func (f *FileManager) CloneRepo(ctx context.Context, repoURL, srcDir string ) error {

	//TODO: change this path to be srcDir
	repoPath := fmt.Sprintf("%s/repo-%d", models.TempDir, time.Now().Unix())
	fmt.Println(repoPath)

	cloneOpts:= git.CloneOptions{
		URL: repoURL,
		Progress: nil, 
		Depth: 1, 
	}

	// if opts.AuthToken != "" {
	// 	cloneOpts.Auth = &http.BasicAuth{
	// 		Username: "token", // Can be anything for token auth
	// 		Password: opts.AuthToken,
	// 	}
	// }

	_, err := git.PlainCloneContext(ctx, repoPath, false, &cloneOpts)
	if err != nil {
		return err
	}


	// w, _ := r.Worktree()

	// err = w.Checkout(&git.CheckoutOptions{
	// 	Branch: plumbing.NewBranchReferenceName("richys-branch"),
	// })

	return nil
}

// ExtractArchive handles .zip and .tar.gz
func (f *FileManager) ExtractArchive(src, destDir string) error {
	switch {
	case strings.HasSuffix(src, ".zip"):
		return extractZip(src, destDir)
	case strings.HasSuffix(src, ".tar.gz") || strings.HasSuffix(src, ".tgz"):
		return extractTarGz(src, destDir)
	default:
		return fmt.Errorf("unsupported archive format: %s", src)
	}
}

func (f *FileManager) Cleanup(dir string) error {
	return os.RemoveAll(dir)
}

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		path := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("zip slip detected: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
			continue
		}
		err := writeFile(path, f.Open, f.Mode())
		if  err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		path := filepath.Join(dest, hdr.Name)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("tar slip detected: %s", hdr.Name)
		}
		if hdr.FileInfo().IsDir() {
			os.MkdirAll(path, hdr.FileInfo().Mode())
			continue
		}
		 err = writeFile(path, func() (io.ReadCloser, error) { 
			return io.NopCloser(tr), nil 

			}, hdr.FileInfo().Mode()); 
			
			if err != nil {
				return err
			}
	}
	return nil
}

func writeFile(path string, open func() (io.ReadCloser, error), mode os.FileMode) error {
	os.MkdirAll(filepath.Dir(path), 0755)
	rc, err := open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}
