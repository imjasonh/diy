package pkg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/imdario/mergo"
)

func Resolve(ctx context.Context, cfg *Config, verbose bool) error {
	if cfg.Base != "" {
		br, err := name.ParseReference(cfg.Base)
		if err != nil {
			return fmt.Errorf("name.ParseReference(%q): %w", cfg.Base, err)
		}
		desc, err := remote.Head(br, remote.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("remote.Head(%q): %w", br, err)
		}
		cfg.Base = br.Context().Digest(desc.Digest.String()).String()

		if verbose {
			log.Printf("resolved base %s -> %s", br, cfg.Base)
		}
	}
	return nil
}

func Build(ctx context.Context, cfg Config, verbose bool) (v1.Image, error) {
	var img v1.Image = empty.Image
	if cfg.Base != "" {
		br, err := name.ParseReference(cfg.Base)
		if err != nil {
			return nil, fmt.Errorf("name.ParseReference(%q): %w", cfg.Base, err)
		}
		img, err = remote.Image(br, remote.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("remote.Image: %w", err)
		}
	}

	for _, l := range cfg.Layers {
		// sort for reproducibility.
		sort.Slice(l.Files, func(i, j int) bool { return l.Files[i].Name < l.Files[j].Name })
		filenames := map[string]struct{}{}
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf) // TODO stream this.
		for _, ff := range l.Files {
			fn := filepath.Clean(ff.Name)
			if fn == "" {
				return nil, errors.New("filename is required")
			}
			if _, found := filenames[fn]; found {
				return nil, fmt.Errorf("duplicate file path: %s", fn)
			}
			filenames[fn] = struct{}{}

			if ff.Contents != "" && ff.Data != "" {
				return nil, errors.New("cannot specify file contents and data")
			}
			size := len(ff.Contents)
			data := []byte(ff.Contents)
			var err error
			if ff.Data != "" {
				data, err = base64.StdEncoding.DecodeString(ff.Data)
				if err != nil {
					return nil, fmt.Errorf("base64.DecodeString: %w", err)
				}
				size = len(data)
			}

			if err := tw.WriteHeader(&tar.Header{
				Name: fn,
				Size: int64(size),
				Mode: ff.Mode,
			}); err != nil {
				return nil, fmt.Errorf("tw.WriteHeader(%q): %w", fn, err)
			}
			if _, err := io.Copy(tw, bytes.NewReader(data)); err != nil {
				return nil, fmt.Errorf("io.Copy(%q): %w", fn, err)
			}
			if verbose {
				log.Println("wrote:", fn)
			}
		}

		if l.Archive != nil {
			if l.Archive.URL == "" {
				return nil, errors.New(".archive.url is required if archive is specified")
			}
			if l.Archive.SHA256 == "" {
				return nil, errors.New(".archive.sha256 is required if archive is specified")
			}
			if l.Archive.Size == 0 {
				return nil, errors.New(".archive.size is required if archive is specified")
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.Archive.URL, nil)
			if err != nil {
				return nil, fmt.Errorf("http.NewRequest: %w", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, fmt.Errorf("http.Get: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				io.Copy(os.Stderr, resp.Body)
				return nil, errors.New(resp.Status)
			}
			if resp.ContentLength != l.Archive.Size {
				return nil, fmt.Errorf("size mismatch: got %d, want %d", resp.ContentLength, l.Archive.Size)
			}

			sha := sha256.New()
			gzr, err := gzip.NewReader(io.TeeReader(io.LimitReader(resp.Body, l.Archive.Size), sha))
			if err != nil {
				return nil, fmt.Errorf("gzip.NewReader: %w", err)
			}
			tr := tar.NewReader(gzr)
			for {
				th, err := tr.Next()
				if err == io.EOF {
					break
				} else if err != nil {
					return nil, fmt.Errorf("tr.Next: %w", err)
				}
				fn := filepath.Clean(th.Name)
				if _, found := filenames[fn]; found {
					if verbose {
						log.Println("skipping archive file:", th.Name)
					}
					continue
				}
				th.Name = fn
				if err := tw.WriteHeader(th); err != nil {
					return nil, fmt.Errorf("tw.WriterHeader(%q): %w", th.Name, err)
				}
				if _, err := io.Copy(tw, tr); err != nil {
					return nil, fmt.Errorf("io.Copy(%q): %w", th.Name, err)
				}
				if verbose {
					log.Println("wrote:", fn)
				}
			}

			got := fmt.Sprintf("%x", sha.Sum(nil))
			if got != l.Archive.SHA256 {
				return nil, fmt.Errorf("fetching %s: digest mismatch; got %q, want %q", l.Archive, got, l.Archive.SHA256)
			}
			if verbose {
				log.Println("sha256 matched!")
			}
		}

		if err := tw.Flush(); err != nil {
			return nil, fmt.Errorf("tw.Flush: %w", err)
		}
		if err := tw.Close(); err != nil {
			return nil, fmt.Errorf("tw.Close: %w", err)
		}

		layer, err := tarball.LayerFromReader(&buf, tarball.WithCompressionLevel(gzip.BestCompression))
		if err != nil {
			return nil, fmt.Errorf("tarball.LayerFromReader: %w", err)
		}
		img, err = mutate.AppendLayers(img, layer)
	}

	// Apply annotations.
	img = mutate.Annotations(img, cfg.Annotations).(v1.Image)

	// Merge YAML config into base image config.
	if cfg.Config != nil {
		icfg, err := img.ConfigFile()
		if err != nil {
			return nil, fmt.Errorf("img.ConfigFile: %w", err)
		}
		if err := mergo.Merge(&icfg.Config, cfg.Config, mergo.WithOverride); err != nil {
			return nil, fmt.Errorf("mergo.Merge: %w", err)
		}
		img, err = mutate.ConfigFile(img, icfg)
		if err != nil {
			return nil, fmt.Errorf("mutate.ConfigFile: %w", err)
		}
	}

	return img, nil
}

type Config struct {
	Name, Base  string
	Annotations map[string]string
	Layers      []layer
	Config      *v1.Config
}

type layer struct {
	Archive *archive
	Files   []file
}

type archive struct {
	URL    string
	SHA256 string
	Size   int64
}

type file struct {
	Name     string
	Contents string
	Mode     int64
	Data     string
}
